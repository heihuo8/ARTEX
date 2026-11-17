package agent

import (
	"os"
	"path/filepath"
)

// scopedRuntimeEnv keeps command-line tool state inside the current task or
// conversation. ARTEX agents still retain normal filesystem access, but tools
// that honor HOME/TMPDIR/XDG no longer leave discoveries for another task.
func scopedRuntimeEnv(base []string, scopeDir string) []string {
	runtimeDir := filepath.Join(scopeDir, ".runtime")
	homeDir := filepath.Join(runtimeDir, "home")
	tmpDir := filepath.Join(runtimeDir, "tmp")
	cacheDir := filepath.Join(runtimeDir, "cache")
	configDir := filepath.Join(runtimeDir, "config")
	dataDir := filepath.Join(runtimeDir, "data")
	for _, dir := range []string{homeDir, tmpDir, cacheDir, configDir, dataDir} {
		_ = os.MkdirAll(dir, 0o700)
	}

	env := append([]string{}, base...)
	return append(env,
		"HOME="+homeDir,
		"TMPDIR="+tmpDir,
		"TMP="+tmpDir,
		"TEMP="+tmpDir,
		"XDG_CACHE_HOME="+cacheDir,
		"XDG_CONFIG_HOME="+configDir,
		"XDG_DATA_HOME="+dataDir,
	)
}

// runtimeEvidenceSpec separates target evidence from ARTEX's own host/container
// state without removing the deliberate ability to inspect either one.
func runtimeEvidenceSpec(workDir string) string {
	return "\n\n**本机证据边界**：当前任务材料以工作目录 " + workDir + " 及任务明确附加的文件为准。ARTEX 进程/容器自身的 /root、全局 /tmp、其他任务目录、进程环境和工具缓存属于平台运行环境，可能含其他任务遗留；不要主动扫描并把它们当作当前目标线索。用户明确要求检查平台环境时仍可读取，但必须标为平台数据，且没有来自当前目标请求、远程会话或本任务产物的来源证明时，不得写成当前目标的 fact/finding。通过当前目标的 SSH、WebShell、文件读取等能力取得的远程 /root、/tmp、进程环境仍属于有效目标证据。"
}
