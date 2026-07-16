package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	environmentRestoreGeneratedFileActionPlanWrite = "plan-write"
	environmentRestoreGeneratedFileActionWrite     = "write"
)

type environmentRestoreGeneratedFile struct {
	Path   string `json:"path"`
	Bytes  int    `json:"bytes"`
	Action string `json:"action"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

func prepareEnvironmentRestoreGeneratedFiles(compose map[string]any, workspace string, execute bool) []environmentRestoreGeneratedFile {
	files := generatedFileContentMapFromAny(compose["generatedFiles"])
	if len(files) == 0 {
		return nil
	}
	modes := environmentRestoreGeneratedFileModes(compose)
	paths := environmentRestoreGeneratedFilePaths(compose, files)
	out := make([]environmentRestoreGeneratedFile, 0, len(paths))
	for _, path := range paths {
		content := files[path]
		mode := modes[path]
		if mode == 0 {
			mode = 0o644
		}
		report := environmentRestoreGeneratedFile{
			Path:   restoreWorkspacePath(workspace, path),
			Bytes:  len(content),
			Action: environmentRestoreGeneratedFileActionPlanWrite,
			OK:     true,
		}
		if ok, errText := environmentRestoreGeneratedFileTargetOK(path, workspace); !ok {
			report.OK = false
			report.Error = errText
			out = append(out, report)
			continue
		}
		if execute {
			report.Action = environmentRestoreGeneratedFileActionWrite
			if err := writeEnvironmentRestoreWorkspaceFile(workspace, path, []byte(content), mode); err != nil {
				report.OK = false
				report.Error = err.Error()
			}
		}
		out = append(out, report)
	}
	return out
}

func environmentRestoreGeneratedFileModes(compose map[string]any) map[string]os.FileMode {
	raw := stringMapFromAny(compose["generatedFileModes"])
	out := map[string]os.FileMode{}
	for path, value := range raw {
		clean := filepath.Clean(strings.TrimSpace(path))
		if clean == "." || clean == "" {
			continue
		}
		modeText := strings.TrimSpace(value)
		switch modeText {
		case "0600", "600":
			out[clean] = 0o600
		case "0644", "644":
			out[clean] = 0o644
		}
	}
	return out
}

func environmentRestoreGeneratedFilePaths(compose map[string]any, files map[string]string) []string {
	paths := make([]string, 0, len(files))
	seen := map[string]bool{}
	for _, path := range stringSliceFromAny(compose["generatedFileOrder"]) {
		clean := filepath.Clean(strings.TrimSpace(path))
		if clean == "." || clean == "" || seen[clean] {
			continue
		}
		if _, exists := files[clean]; !exists {
			continue
		}
		paths = append(paths, clean)
		seen[clean] = true
	}
	remaining := make([]string, 0, len(files)-len(paths))
	for path := range files {
		clean := filepath.Clean(strings.TrimSpace(path))
		if clean == "." || clean == "" || seen[clean] {
			continue
		}
		remaining = append(remaining, clean)
	}
	sort.Strings(remaining)
	paths = append(paths, remaining...)
	return paths
}

func environmentRestoreGeneratedFileTargetOK(path string, workspace string) (bool, string) {
	raw := strings.TrimSpace(path)
	if raw == "" {
		return false, "generated file path is empty"
	}
	if filepath.IsAbs(raw) {
		return false, "generated file path must be relative to the restore workspace: " + raw
	}
	clean := filepath.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return false, "generated file path must stay inside the restore workspace: " + raw
	}
	target := restoreWorkspacePath(workspace, clean)
	rel, err := filepath.Rel(workspace, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false, "generated file path must stay inside the restore workspace: " + raw
	}
	return true, ""
}

func environmentRestoreGeneratedEnvFilePath(workspace string) string {
	return filepath.Join(workspace, ".agent-testbench", "restore.env")
}

func writeEnvironmentRestoreGeneratedEnvFile(workspace string, compose map[string]any) (string, error) {
	content := environmentRestoreGeneratedEnvFileContent(workspace, compose)
	if content == "" {
		return "", nil
	}
	workspace = filepath.Clean(strings.TrimSpace(workspace))
	path := environmentRestoreGeneratedEnvFilePath(workspace)
	if err := writeEnvironmentRestoreWorkspaceFile(workspace, filepath.Join(".agent-testbench", "restore.env"), []byte(content), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func environmentRestoreGeneratedEnvFileContent(workspace string, compose map[string]any) string {
	values := stringMapFromAny(compose["env"])
	if strings.TrimSpace(workspace) != "" {
		values["AGENT_TESTBENCH_WORKSPACE"] = workspace
	}
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		value := strings.ReplaceAll(strings.ReplaceAll(values[key], "$AGENT_TESTBENCH_WORKSPACE", workspace), "${AGENT_TESTBENCH_WORKSPACE}", workspace)
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(value)
		b.WriteString("\n")
	}
	return b.String()
}
