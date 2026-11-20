package traffic

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ArchiveSnapshot is the portable traffic subset embedded in one task archive.
// Large content-addressed bodies are copied alongside this manifest rather than
// base64-encoded, preserving deduplication and allowing the tar writer to stream.
type ArchiveSnapshot struct {
	Version   int               `json:"version"`
	Exchanges []ArchiveExchange `json:"exchanges"`
	Blobs     []string          `json:"blobs"`
}

type ArchiveExchange struct {
	ID          string `json:"id"`
	TS          int64  `json:"ts"`
	Host        string `json:"host"`
	Method      string `json:"method"`
	URLTemplate string `json:"url_template"`
	URL         string `json:"url"`
	Status      int    `json:"status"`
	ContentType string `json:"content_type"`
	ReqLen      int    `json:"req_len"`
	RespLen     int    `json:"resp_len"`
	ReqHead     string `json:"req_head"`
	ReqBody     []byte `json:"req_body,omitempty"`
	ReqBlob     string `json:"req_blob,omitempty"`
	RespHead    string `json:"resp_head"`
	RespBody    []byte `json:"resp_body,omitempty"`
	RespBlob    string `json:"resp_blob,omitempty"`
}

// ExportHosts writes an exact-host snapshot to dir. It holds the traffic writer
// lock while reading SQLite and blobs, so each body and its index row come from
// one consistent point in time.
func (t *Traffic) ExportHosts(hosts []string, dir string) (int64, error) {
	return t.ExportHostsWithProgress(hosts, dir, nil)
}

// ExportHostsWithProgress streams the traffic manifest instead of retaining all
// exchanges and inline bodies in memory. The callback receives exported and
// total exchange counts while the traffic writer lock is held.
func (t *Traffic) ExportHostsWithProgress(hosts []string, dir string, onProgress func(exported, total int64)) (int64, error) {
	if t == nil || len(hosts) == 0 {
		return 0, nil
	}
	if err := os.MkdirAll(filepath.Join(dir, "blobs"), 0o700); err != nil {
		return 0, err
	}
	t.wmu.Lock()
	defer t.wmu.Unlock()
	unique := uniqueArchiveHosts(hosts)
	placeholders := make([]string, len(unique))
	args := make([]any, len(unique))
	for i, host := range unique {
		placeholders[i] = "?"
		args[i] = host
	}
	where := ` WHERE e.host IN (` + strings.Join(placeholders, ",") + `)`
	var total int64
	if err := t.db.QueryRow(`SELECT count(*) FROM exchanges e`+where, args...).Scan(&total); err != nil {
		return 0, err
	}
	rows, err := t.db.Query(`SELECT e.id,e.ts,e.host,e.method,e.url_template,e.url,e.status,e.content_type,e.req_len,e.resp_len,e.path,
 b.req_head,b.req_body,b.req_blob,b.resp_head,b.resp_body,b.resp_blob
FROM exchanges e LEFT JOIN exchange_bodies b ON b.id=e.id`+where+` ORDER BY e.ts,e.id`, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	manifestPath := filepath.Join(dir, "traffic.json")
	temporary := manifestPath + ".partial"
	_ = os.Remove(temporary)
	manifest, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	committed := false
	defer func() {
		_ = manifest.Close()
		if !committed {
			_ = os.Remove(temporary)
		}
	}()
	buffer := bufio.NewWriterSize(manifest, 256<<10)
	if _, err := buffer.WriteString("{\"version\":1,\"exchanges\":["); err != nil {
		return 0, err
	}
	blobs := map[string]struct{}{}
	var exported int64
	for rows.Next() {
		var item ArchiveExchange
		var legacyPath string
		var reqHead, reqBlob, respHead, respBlob sql.NullString
		if err := rows.Scan(&item.ID, &item.TS, &item.Host, &item.Method, &item.URLTemplate,
			&item.URL, &item.Status, &item.ContentType, &item.ReqLen, &item.RespLen, &legacyPath,
			&reqHead, &item.ReqBody, &reqBlob, &respHead, &item.RespBody, &respBlob); err != nil {
			return 0, err
		}
		item.ReqHead, item.RespHead = reqHead.String, respHead.String
		item.ReqBlob, item.RespBlob = reqBlob.String, respBlob.String
		if !reqHead.Valid && !respHead.Valid && strings.TrimSpace(legacyPath) != "" {
			req, readErr := os.ReadFile(filepath.Join(t.dir, legacyPath, "request.http"))
			if readErr != nil && !os.IsNotExist(readErr) {
				return 0, readErr
			}
			resp, readErr := os.ReadFile(filepath.Join(t.dir, legacyPath, "response.http"))
			if readErr != nil && !os.IsNotExist(readErr) {
				return 0, readErr
			}
			item.ReqHead, item.RespHead = string(req), string(resp)
		}
		for _, hash := range []string{item.ReqBlob, item.RespBlob} {
			if hash != "" {
				blobs[hash] = struct{}{}
			}
		}
		raw, err := json.Marshal(item)
		if err != nil {
			return 0, err
		}
		if exported > 0 {
			if err := buffer.WriteByte(','); err != nil {
				return 0, err
			}
		}
		if _, err := buffer.Write(raw); err != nil {
			return 0, err
		}
		exported++
		if onProgress != nil {
			onProgress(exported, total)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if _, err := buffer.WriteString("],\"blobs\":["); err != nil {
		return 0, err
	}
	blobList := make([]string, 0, len(blobs))
	for hash := range blobs {
		blobList = append(blobList, hash)
	}
	sort.Strings(blobList)
	for i, hash := range blobList {
		source, err := t.blobPath(hash)
		if err != nil {
			return 0, err
		}
		if err := copyArchiveBlob(source, filepath.Join(dir, "blobs", hash+".bin")); err != nil {
			return 0, err
		}
		raw, err := json.Marshal(hash)
		if err != nil {
			return 0, err
		}
		if i > 0 {
			if err := buffer.WriteByte(','); err != nil {
				return 0, err
			}
		}
		if _, err := buffer.Write(raw); err != nil {
			return 0, err
		}
	}
	if _, err := buffer.WriteString("]}\n"); err != nil {
		return 0, err
	}
	if err := buffer.Flush(); err != nil {
		return 0, err
	}
	if err := manifest.Sync(); err != nil {
		return 0, err
	}
	if err := manifest.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(temporary, manifestPath); err != nil {
		return 0, err
	}
	committed = true
	return exported, nil
}

func copyArchiveBlob(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	var syncErr error
	if copyErr == nil {
		syncErr = output.Sync()
	}
	closeErr := output.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		_ = os.Remove(destination)
		return err
	}
	return nil
}

// ImportArchive imports only missing exchange IDs. Current hot rows always win,
// and repeated restore attempts are safe after a partial external failure.
func (t *Traffic) ImportArchive(dir string) (int64, error) {
	if t == nil {
		return 0, nil
	}
	raw, err := os.ReadFile(filepath.Join(dir, "traffic.json"))
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var snapshot ArchiveSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return 0, err
	}
	if snapshot.Version != 1 {
		return 0, fmt.Errorf("unsupported traffic archive version %d", snapshot.Version)
	}
	t.wmu.Lock()
	defer t.wmu.Unlock()
	for _, hash := range snapshot.Blobs {
		if !blobHashRe.MatchString(hash) {
			return 0, fmt.Errorf("invalid archived traffic blob %q", hash)
		}
		data, err := os.ReadFile(filepath.Join(dir, "blobs", hash+".bin"))
		if err != nil {
			return 0, err
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != hash {
			return 0, fmt.Errorf("traffic blob checksum mismatch: %s", hash)
		}
		destination := filepath.Join(t.dir, "_blobs", "sha256", hash[:2], hash+".bin")
		if _, err := os.Stat(destination); os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				return 0, err
			}
			if err := os.WriteFile(destination, data, 0o644); err != nil {
				return 0, err
			}
		}
	}
	tx, err := t.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck
	var imported int64
	for _, item := range snapshot.Exchanges {
		res, err := tx.Exec(`INSERT OR IGNORE INTO exchanges(id,ts,host,method,url_template,url,status,content_type,req_len,resp_len,path)
VALUES(?,?,?,?,?,?,?,?,?,?,'')`, item.ID, item.TS, item.Host, item.Method, item.URLTemplate,
			item.URL, item.Status, item.ContentType, item.ReqLen, item.RespLen)
		if err != nil {
			return imported, err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO exchange_bodies(id,req_head,req_body,req_blob,resp_head,resp_body,resp_blob)
VALUES(?,?,?,?,?,?,?)`, item.ID, item.ReqHead, item.ReqBody, nullIfEmpty(item.ReqBlob),
			item.RespHead, item.RespBody, nullIfEmpty(item.RespBlob)); err != nil {
			return imported, err
		}
		for _, hash := range []string{item.ReqBlob, item.RespBlob} {
			if hash != "" {
				if _, err := tx.Exec(`INSERT OR IGNORE INTO blob_refs(hash,exchange_id) VALUES(?,?)`, hash, item.ID); err != nil {
					return imported, err
				}
			}
		}
		if t.fts {
			var rowID int64
			if err := tx.QueryRow(`SELECT rowid FROM exchanges WHERE id=?`, item.ID).Scan(&rowID); err != nil {
				return imported, err
			}
			content := strings.Join([]string{item.URL, item.ReqHead, string(item.ReqBody), item.RespHead, string(item.RespBody)}, "\n")
			if _, err := tx.Exec(`INSERT INTO ex_fts(rowid,content) VALUES(?,?)`, rowID, content); err != nil {
				return imported, err
			}
		}
		imported++
	}
	if err := tx.Commit(); err != nil {
		return imported, err
	}
	return imported, nil
}

func uniqueArchiveHosts(hosts []string) []string {
	seen := make(map[string]struct{}, len(hosts))
	out := make([]string, 0, len(hosts))
	for _, host := range hosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host == "" {
			continue
		}
		if _, exists := seen[host]; exists {
			continue
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	sort.Strings(out)
	return out
}
