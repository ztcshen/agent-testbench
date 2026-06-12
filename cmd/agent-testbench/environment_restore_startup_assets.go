package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"agent-testbench/internal/domain/environmentfiles"
	"agent-testbench/internal/domain/environmentsource"
)

type environmentRestoreStartupAsset struct {
	Path        string `json:"path"`
	Source      string `json:"source,omitempty"`
	ComposeFile string `json:"composeFile,omitempty"`
	Kind        string `json:"kind"`
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
}

type environmentRestoreStartupAssetCandidate struct {
	path        string
	source      string
	composeFile string
	kind        string
}

func environmentRestoreStartupAssets(compose map[string]any, specs []environmentRestoreRepoSpec, workspace string) []environmentRestoreStartupAsset {
	generated := stringMapFromAny(compose["generatedFiles"])
	generatedPaths := map[string]bool{}
	for path := range generated {
		generatedPaths[filepath.Clean(path)] = true
	}
	repoCheckouts := map[string]bool{}
	for _, spec := range specs {
		if spec.Checkout == "" {
			continue
		}
		if checkout := environmentRestoreStartupWorkspacePath(spec.Checkout, workspace); checkout != "" {
			repoCheckouts[checkout] = true
		}
	}
	candidates := []environmentRestoreStartupAssetCandidate{}
	for _, composeFile := range environmentRestoreComposeFiles(compose) {
		cleanCompose := filepath.Clean(composeFile)
		content := generated[cleanCompose]
		if content == "" {
			if raw, err := os.ReadFile(restoreWorkspacePath(workspace, composeFile)); err == nil {
				content = string(raw)
			}
		}
		if content == "" {
			continue
		}
		composeDir := filepath.Dir(cleanCompose)
		candidates = append(candidates, environmentRestoreStartupAssetCandidates(content, cleanCompose, composeDir, compose, workspace)...)
	}
	seen := map[string]bool{}
	out := []environmentRestoreStartupAsset{}
	for _, item := range candidates {
		clean := filepath.Clean(item.path)
		if clean == "." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			continue
		}
		if environmentRestoreStartupAssetCoveredByRepo(clean, repoCheckouts) {
			continue
		}
		key := clean + "\x00" + item.source + "\x00" + item.composeFile
		if seen[key] {
			continue
		}
		seen[key] = true
		asset := environmentRestoreStartupAsset{
			Path:        clean,
			Source:      item.source,
			ComposeFile: item.composeFile,
			Kind:        item.kind,
			OK:          true,
		}
		if !environmentRestoreStartupAssetAvailable(clean, workspace, generatedPaths) {
			asset.OK = false
			asset.Error = "startup asset must exist in the restore workspace or be provided through Store generatedFiles"
		}
		out = append(out, asset)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].Source < out[j].Source
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func environmentRestoreIsRemoteGitURL(rawURL string) bool {
	return environmentsource.IsRemoteGitURL(rawURL)
}

func environmentRestoreStartupAssetCandidates(content string, composeFile string, composeDir string, compose map[string]any, workspace string) []environmentRestoreStartupAssetCandidate {
	out := []environmentRestoreStartupAssetCandidate{}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, "/sandbox/compose/") {
			for _, path := range extractSandboxComposePaths(trimmed) {
				out = append(out, environmentRestoreStartupAssetCandidate{path: path, source: trimmed, composeFile: composeFile, kind: "container-command"})
			}
		}
		volume := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
		if volume == trimmed {
			continue
		}
		source, target, ok := parseComposeShortVolume(volume)
		if !ok || !strings.HasPrefix(target, "/") {
			continue
		}
		assetPath, assetOK := environmentRestoreStartupAssetPath(source, composeDir, compose, workspace)
		if !assetOK {
			continue
		}
		out = append(out, environmentRestoreStartupAssetCandidate{path: assetPath, source: source, composeFile: composeFile, kind: "bind-source"})
	}
	for _, envFile := range stringSliceFromAny(compose["envFiles"]) {
		if envFile == "" {
			continue
		}
		out = append(out, environmentRestoreStartupAssetCandidate{path: filepath.Clean(envFile), source: envFile, composeFile: composeFile, kind: "compose-env-file"})
	}
	return out
}

func parseComposeShortVolume(value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	if strings.HasPrefix(value, "[") || strings.Contains(value, "source:") || strings.Contains(value, "target:") {
		return "", "", false
	}
	source, target, ok := splitComposeShortVolume(value)
	if !ok {
		return "", "", false
	}
	source = strings.Trim(source, `"' `)
	target = strings.Trim(target, `"' `)
	if source == "" || target == "" {
		return "", "", false
	}
	if !composeHostSourceLooksLikePath(source) {
		return "", "", false
	}
	return source, target, true
}

func environmentRestoreStartupAssetPath(source string, composeDir string, compose map[string]any, workspace string) (string, bool) {
	expanded := expandEnvironmentRestoreComposeSource(source, compose, workspace)
	if expanded == "" {
		return "", false
	}
	if strings.HasPrefix(expanded, "../.runtime") || strings.Contains(expanded, string(os.PathSeparator)+".runtime"+string(os.PathSeparator)) {
		return "", false
	}
	if strings.HasPrefix(expanded, "~") || strings.HasPrefix(expanded, "$HOME") || strings.HasPrefix(expanded, "${HOME}") {
		return "", false
	}
	if filepath.IsAbs(expanded) {
		if rel, err := filepath.Rel(workspace, expanded); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".." {
			return filepath.Clean(rel), true
		}
		return "", false
	}
	if strings.HasPrefix(expanded, "./") || strings.HasPrefix(expanded, "../") {
		return filepath.Clean(filepath.Join(composeDir, expanded)), true
	}
	return "", false
}

func expandEnvironmentRestoreComposeSource(source string, compose map[string]any, workspace string) string {
	values := environmentRestoreComposeEnvValues(compose, workspace)
	expanded := strings.TrimSpace(source)
	if strings.Contains(expanded, "$") {
		if resolved, ok := environmentfiles.InterpolateComposeText(expanded, values); ok {
			expanded = resolved
		}
	}
	return expanded
}

func environmentRestoreComposeEnvValues(compose map[string]any, workspace string) map[string]string {
	values := stringMapFromAny(compose["env"])
	for key, value := range values {
		values[key] = strings.ReplaceAll(strings.ReplaceAll(value, "$AGENT_TESTBENCH_WORKSPACE", workspace), "${AGENT_TESTBENCH_WORKSPACE}", workspace)
	}
	values["AGENT_TESTBENCH_WORKSPACE"] = workspace
	return values
}

func splitComposeShortVolume(value string) (string, string, bool) {
	depth := 0
	for i := 0; i < len(value); i++ {
		switch {
		case value[i] == '$' && i+1 < len(value) && value[i+1] == '{':
			depth++
			i++
		case value[i] == '}' && depth > 0:
			depth--
		case value[i] == ':' && depth == 0:
			return value[:i], value[i+1:], true
		}
	}
	return "", "", false
}

func environmentRestoreStartupWorkspacePath(path string, workspace string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." {
		return ""
	}
	if filepath.IsAbs(path) {
		rel, err := filepath.Rel(workspace, path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return ""
		}
		return filepath.Clean(rel)
	}
	return path
}

func environmentRestoreStartupAssetCoveredByRepo(path string, repoCheckouts map[string]bool) bool {
	for checkout := range repoCheckouts {
		if path == checkout || strings.HasPrefix(path, checkout+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func environmentRestoreStartupAssetAvailable(path string, workspace string, generatedPaths map[string]bool) bool {
	if generatedPaths[filepath.Clean(path)] {
		return true
	}
	prefix := filepath.Clean(path) + string(os.PathSeparator)
	for generated := range generatedPaths {
		if strings.HasPrefix(generated, prefix) {
			return true
		}
	}
	if _, err := os.Stat(restoreWorkspacePath(workspace, path)); err == nil {
		return true
	}
	return false
}

func restoreWorkspacePath(workspace string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(workspace, value)
}
