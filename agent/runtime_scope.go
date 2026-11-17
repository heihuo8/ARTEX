package agent

import (
	"os"
	"path/filepath"
)

// scopedRuntimeEnv gives commands a scope-local temporary directory without
// replacing HOME or XDG paths. Keeping the real user environment is important:
// many installed tools load credentials, configuration and global packages
// from it. This is temp-state separation, not a filesystem sandbox.
func scopedRuntimeEnv(base []string, scopeDir string) []string {
	tmpDir := filepath.Join(scopeDir, ".runtime", "tmp")
	_ = os.MkdirAll(tmpDir, 0o700)

	env := append([]string{}, base...)
	return append(env,
		"TMPDIR="+tmpDir,
		"TMP="+tmpDir,
		"TEMP="+tmpDir,
	)
}

// runtimeEvidenceSpec separates target evidence from ARTEX's own host/container
// state without removing the deliberate ability to inspect either one.
func runtimeEvidenceSpec(workDir string) string {
	return "\n\n**本机证据边界（代码固定约束）**：当前任务材料以工作目录 " + workDir + " 及任务明确附加的文件为准。即使其他提示要求搜索本机，ARTEX 进程/容器自身的 /root、全局 /tmp、其他任务目录、进程环境和工具缓存仍属于平台运行环境，可能含其他任务遗留；不要主动扫描并把它们当作当前目标线索。用户明确要求检查平台环境时仍可读取，但必须标为平台数据，且没有来自当前目标请求、远程会话或本任务产物的来源证明时，不得写成当前目标的 fact/finding。通过当前目标的 SSH、WebShell、文件读取等能力取得的远程 /root、/tmp、进程环境仍属于有效目标证据。"
}
