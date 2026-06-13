package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"agent-testbench/internal/domain/composefile"
)

func environmentRestoreContainerNames(compose map[string]any, workspace string) []string {
	byService := environmentRestoreContainerNameByService(compose, workspace)
	names := make([]string, 0, len(byService))
	for _, name := range byService {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func environmentRestoreContainerNameByService(compose map[string]any, workspace string) map[string]string {
	out := map[string]string{}
	addContent := func(content string) {
		for service, container := range composefile.ParseContainerNames(content) {
			out[service] = container
		}
	}
	for _, content := range generatedFileContentMapFromAny(compose["generatedFiles"]) {
		addContent(content)
	}
	for _, file := range environmentRestoreComposeFiles(compose) {
		path := restoreWorkspacePath(workspace, file)
		raw, err := os.ReadFile(path)
		if err == nil {
			addContent(string(raw))
		}
	}
	return out
}

func environmentRestoreComposeBindMountSources(compose map[string]any, workspace string) map[string][]string {
	out := map[string][]string{}
	for _, content := range environmentRestoreComposeFileContents(compose, workspace) {
		for service, sources := range composefile.ParseBindMountSources(content) {
			out[service] = append(out[service], sources...)
		}
	}
	return out
}

func environmentRestoreComposeImageReferences(compose map[string]any, workspace string) map[string]string {
	out := map[string]string{}
	for _, content := range environmentRestoreComposeFileContents(compose, workspace) {
		for service, image := range composefile.ParseImageReferences(content) {
			out[service] = image
		}
	}
	return out
}

func environmentRestoreComposeFileContents(compose map[string]any, workspace string) []string {
	contents := []string{}
	generated := generatedFileContentMapFromAny(compose["generatedFiles"])
	for _, file := range environmentRestoreComposeFiles(compose) {
		content := generated[filepath.Clean(file)]
		if content == "" {
			content = generated[file]
		}
		if content == "" {
			raw, err := os.ReadFile(restoreWorkspacePath(workspace, file))
			if err == nil {
				content = string(raw)
			}
		}
		if strings.TrimSpace(content) != "" {
			contents = append(contents, content)
		}
	}
	return contents
}

func parseComposeImageReferences(content string) map[string]string {
	return composefile.ParseImageReferences(content)
}

func cleanComposeScalar(value string) string {
	return composefile.CleanScalar(value)
}
