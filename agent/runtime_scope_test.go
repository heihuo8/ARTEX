package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopedRuntimeEnvUsesScopeDirectory(t *testing.T) {
	scope := filepath.Join(t.TempDir(), "task-51")
	env := scopedRuntimeEnv([]string{"HTTP_PROXY=http://proxy"}, scope)
	joined := strings.Join(env, "\n")
	for _, want := range []string{
		"HTTP_PROXY=http://proxy",
		"HOME=" + filepath.Join(scope, ".runtime", "home"),
		"TMPDIR=" + filepath.Join(scope, ".runtime", "tmp"),
		"XDG_CACHE_HOME=" + filepath.Join(scope, ".runtime", "cache"),
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("runtime env missing %q: %s", want, joined)
		}
	}
	for _, dir := range []string{"home", "tmp", "cache", "config", "data"} {
		info, err := os.Stat(filepath.Join(scope, ".runtime", dir))
		if err != nil || !info.IsDir() {
			t.Fatalf("runtime directory %s was not created: %v", dir, err)
		}
	}
}

func TestRuntimeEvidenceSpecDistinguishesRemoteTargetFiles(t *testing.T) {
	got := runtimeEvidenceSpec("/app/data/tasks/51")
	for _, want := range []string{"其他任务遗留", "不得写成当前目标的 fact/finding", "远程 /root"} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime evidence boundary missing %q: %s", want, got)
		}
	}
}
