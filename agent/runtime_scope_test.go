package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopedRuntimeEnvUsesScopeDirectory(t *testing.T) {
	scope := filepath.Join(t.TempDir(), "task-51")
	env := scopedRuntimeEnv([]string{"HTTP_PROXY=http://proxy", "HOME=/real/home"}, scope)
	joined := strings.Join(env, "\n")
	for _, want := range []string{
		"HTTP_PROXY=http://proxy",
		"HOME=/real/home",
		"TMPDIR=" + filepath.Join(scope, ".runtime", "tmp"),
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("runtime env missing %q: %s", want, joined)
		}
	}
	for _, unexpected := range []string{"XDG_CACHE_HOME=", "XDG_CONFIG_HOME=", "XDG_DATA_HOME="} {
		if strings.Contains(joined, unexpected) {
			t.Fatalf("runtime env unexpectedly overrides user configuration with %q: %s", unexpected, joined)
		}
	}
	info, err := os.Stat(filepath.Join(scope, ".runtime", "tmp"))
	if err != nil || !info.IsDir() {
		t.Fatalf("runtime temp directory was not created: %v", err)
	}
}

func TestScopedRuntimeEnvDoesNotIntroduceHome(t *testing.T) {
	env := scopedRuntimeEnv(nil, t.TempDir())
	for _, item := range env {
		if strings.HasPrefix(item, "HOME=") || strings.HasPrefix(item, "XDG_") {
			t.Fatalf("runtime env must inherit user configuration, got %q", item)
		}
	}
}

func TestRuntimeEvidenceSpecDistinguishesRemoteTargetFiles(t *testing.T) {
	got := runtimeEvidenceSpec("/app/data/tasks/51")
	for _, want := range []string{"即使其他提示要求搜索本机", "其他任务遗留", "不得写成当前目标的 fact/finding", "远程 /root"} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime evidence boundary missing %q: %s", want, got)
		}
	}
}

func TestRuntimeEvidenceBoundaryFollowsGlobalPrompt(t *testing.T) {
	t.Cleanup(func() { GlobalSystemPrompt = nil })
	GlobalSystemPrompt = func() string { return "GLOBAL: scan local files" }

	got := workerSystem("", "", "/app/data", "/app/data/tasks/51/i9")
	globalAt := strings.Index(got, "GLOBAL: scan local files")
	boundaryAt := strings.Index(got, "本机证据边界（代码固定约束）")
	if globalAt < 0 || boundaryAt <= globalAt {
		t.Fatalf("code-owned evidence boundary must follow the global prompt: %s", got)
	}
}
