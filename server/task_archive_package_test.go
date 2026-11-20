package server

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	pgdb "github.com/Autumn-27/artex/db"
	"github.com/klauspost/compress/zstd"
)

func TestTaskArchivePackageFilesRoundTrip(t *testing.T) {
	dataDir := t.TempDir()
	taskID := "42"
	explorationID := int64(73)
	taskFile := filepath.Join(dataDir, "tasks", taskID, "uploads", "evidence.txt")
	transcriptFile := filepath.Join(dataDir, "transcripts", "exp73-worker-1.jsonl")
	unrelatedTranscript := filepath.Join(dataDir, "transcripts", "exp74-worker-1.jsonl")
	for path, body := range map[string]string{
		taskFile:            "task evidence",
		transcriptFile:      "transcript",
		unrelatedTranscript: "leave me hot",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	stage, err := stageTaskArchiveFiles(dataDir, 1, taskID, explorationID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(taskFile); !os.IsNotExist(err) {
		t.Fatalf("task file still hot after staging: %v", err)
	}
	if _, err := os.Stat(transcriptFile); !os.IsNotExist(err) {
		t.Fatalf("task transcript still hot after staging: %v", err)
	}
	if _, err := os.Stat(unrelatedTranscript); err != nil {
		t.Fatalf("unrelated transcript was staged: %v", err)
	}

	archivePath := taskArchivePath(dataDir, 1, taskID)
	streamPath := filepath.Join(stage.payload, filepath.FromSlash(pgdb.TaskArchiveLLMRecordsPath))
	if err := os.MkdirAll(filepath.Dir(streamPath), archiveDirMode); err != nil {
		t.Fatal(err)
	}
	streamBody := []byte("{\"id\":1,\"raw_request\":\"large\"}\n")
	if err := os.WriteFile(streamPath, streamBody, archiveFileMode); err != nil {
		t.Fatal(err)
	}
	snapshot := &pgdb.TaskArchiveSnapshot{
		FormatVersion: pgdb.TaskArchiveFormatVersion, TaskID: 42, ExplorationID: explorationID,
		StreamedTables: map[string]string{"llm_records": pgdb.TaskArchiveLLMRecordsPath},
	}
	original, compressed, checksum, err := writeTaskArchivePackage(archivePath, stage.payload, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if original == 0 || compressed == 0 || checksum == "" {
		t.Fatalf("invalid package metrics original=%d compressed=%d checksum=%q", original, compressed, checksum)
	}
	if err := stage.commit(); err != nil {
		t.Fatal(err)
	}
	extracted := filepath.Join(dataDir, "restore")
	if err := extractTaskArchivePackage(archivePath, checksum, extracted); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(extracted, filepath.FromSlash(pgdb.TaskArchiveLLMRecordsPath))); err != nil || !bytes.Equal(got, streamBody) {
		t.Fatalf("streamed LLM archive payload=%q err=%v", got, err)
	}
	installed, err := installTaskArchiveFiles(dataDir, extracted, taskID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := installed.commit(); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{taskFile: "task evidence", transcriptFile: "transcript"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("restored %s = %q, %v; want %q", path, got, err, want)
		}
	}
	if err := extractTaskArchivePackage(archivePath, "deadbeef", filepath.Join(dataDir, "bad-checksum")); err == nil {
		t.Fatal("checksum mismatch was accepted")
	}
}

func TestTaskArchivePackageSkipsSymlink(t *testing.T) {
	dataDir := t.TempDir()
	payload := filepath.Join(dataDir, "payload")
	if err := os.MkdirAll(payload, archiveDirMode); err != nil {
		t.Fatal(err)
	}
	regular := filepath.Join(payload, "keep.txt")
	if err := os.WriteFile(regular, []byte("keep me"), archiveFileMode); err != nil {
		t.Fatal(err)
	}
	// 工作目录里出现的符号链接应被跳过，而不是让整个归档失败。
	if err := os.Symlink(regular, filepath.Join(payload, "link.txt")); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	archivePath := filepath.Join(dataDir, "archive.tar.zst")
	snapshot := &pgdb.TaskArchiveSnapshot{FormatVersion: pgdb.TaskArchiveFormatVersion, TaskID: 42}
	_, _, checksum, err := writeTaskArchivePackage(archivePath, payload, snapshot)
	if err != nil {
		t.Fatalf("archive should skip symlink, not fail: %v", err)
	}
	extracted := filepath.Join(dataDir, "restore")
	if err := extractTaskArchivePackage(archivePath, checksum, extracted); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(extracted, "keep.txt")); err != nil || string(got) != "keep me" {
		t.Fatalf("regular file not archived: got=%q err=%v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(extracted, "link.txt")); !os.IsNotExist(err) {
		t.Fatalf("symlink should have been skipped, but link.txt exists: %v", err)
	}
}

func TestTaskArchivePackageWithProgressRoundTrip(t *testing.T) {
	payload := t.TempDir()
	body := bytes.Repeat([]byte("archive progress\n"), 4096)
	if err := os.WriteFile(filepath.Join(payload, "payload.txt"), body, archiveFileMode); err != nil {
		t.Fatal(err)
	}
	snapshot := &pgdb.TaskArchiveSnapshot{FormatVersion: pgdb.TaskArchiveFormatVersion, TaskID: 42}
	archivePath := filepath.Join(t.TempDir(), "archive.tar.zst")
	var progressed int64
	original, compressed, checksum, err := writeTaskArchivePackageWithProgress(
		archivePath,
		payload,
		snapshot,
		func(processed int64) { progressed = processed },
	)
	if err != nil {
		t.Fatal(err)
	}
	if progressed < int64(len(body)) {
		t.Fatalf("progressed=%d, want at least %d", progressed, len(body))
	}
	if original != progressed || compressed <= 0 || checksum == "" {
		t.Fatalf("invalid package metrics original=%d compressed=%d checksum=%q", original, compressed, checksum)
	}
	extracted := filepath.Join(t.TempDir(), "restore")
	if err := extractTaskArchivePackage(archivePath, checksum, extracted); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(extracted, "payload.txt"))
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("restored payload mismatch: bytes=%d err=%v", len(got), err)
	}
}

func TestTaskArchiveFileModeSupported(t *testing.T) {
	tests := []struct {
		name string
		mode fs.FileMode
		want bool
	}{
		{name: "regular", mode: 0, want: true},
		{name: "directory", mode: fs.ModeDir, want: true},
		{name: "named pipe", mode: fs.ModeNamedPipe, want: false},
		{name: "socket", mode: fs.ModeSocket, want: false},
		{name: "device", mode: fs.ModeDevice, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := taskArchiveFileModeSupported(tt.mode); got != tt.want {
				t.Fatalf("taskArchiveFileModeSupported(%v)=%v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestTaskArchiveStageJournalRollsBackInterruptedMoves(t *testing.T) {
	dataDir := t.TempDir()
	taskFile := filepath.Join(dataDir, "tasks", "42", "evidence.txt")
	transcriptFile := filepath.Join(dataDir, "transcripts", "exp73-worker.jsonl")
	for _, path := range []string{taskFile, transcriptFile} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(path), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stage, err := stageTaskArchiveFiles(dataDir, 9, "42", 73)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(stage.root, "journal.json"))
	if err != nil {
		t.Fatal(err)
	}
	var journal archiveStageJournal
	if err := json.Unmarshal(raw, &journal); err != nil {
		t.Fatal(err)
	}
	if len(journal.Moves) != 2 {
		t.Fatalf("journal moves=%d, want 2", len(journal.Moves))
	}
	recovered := &taskArchiveFileStage{root: stage.root, payload: stage.payload, journal: journal}
	if err := recovered.rollback(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{taskFile, transcriptFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("interrupted archive did not restore %s: %v", path, err)
		}
	}
}

func TestTaskArchiveRestoreJournalRollsBackInterruptedInstall(t *testing.T) {
	dataDir := t.TempDir()
	extracted := filepath.Join(dataDir, "archives", "tasks", ".restore", "11-test")
	source := filepath.Join(extracted, "files", "tasks", "42", "evidence.txt")
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := installTaskArchiveFiles(dataDir, extracted, "42", 11)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dataDir, "tasks", "42", "evidence.txt")
	if _, err := os.Stat(destination); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(extracted, "restore-journal.json"))
	if err != nil {
		t.Fatal(err)
	}
	var journal archiveRestoreJournal
	if err := json.Unmarshal(raw, &journal); err != nil {
		t.Fatal(err)
	}
	if journal.ArchiveID != 11 || len(journal.Moves) != 1 {
		t.Fatalf("unexpected restore journal: %+v", journal)
	}
	recovered := &taskArchiveRestoreFiles{extracted: extracted, moves: journal.Moves}
	if err := recovered.rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("hot destination remains after restore rollback: %v", err)
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != "evidence" {
		t.Fatalf("restore source=%q err=%v", got, err)
	}
}

func TestTaskArchiveDeletePackageCanResumeFromStagedPath(t *testing.T) {
	dataDir := t.TempDir()
	archivePath := taskArchivePath(dataDir, 21, "42")
	if err := os.MkdirAll(filepath.Dir(archivePath), archiveDirMode); err != nil {
		t.Fatal(err)
	}
	staged := archivePath + ".deleting-21"
	if err := os.WriteFile(staged, []byte("archive"), archiveFileMode); err != nil {
		t.Fatal(err)
	}
	got, moved, err := stageTaskArchivePackageDelete(archivePath, 21)
	if err != nil || !moved || got != staged {
		t.Fatalf("resume staged package path=%q moved=%v err=%v", got, moved, err)
	}
}

func TestTaskArchivePackageRejectsTraversal(t *testing.T) {
	var compressed bytes.Buffer
	encoder, err := zstd.NewWriter(&compressed)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(encoder)
	content := []byte("escape")
	if err := tw.WriteHeader(&tar.Header{Name: "../escape.txt", Mode: 0o600, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "malicious.tar.zst")
	if err := os.WriteFile(path, compressed.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := extractTaskArchivePackage(path, "", root); err == nil {
		t.Fatal("path traversal archive was accepted")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("traversal wrote outside destination: %v", err)
	}
}
