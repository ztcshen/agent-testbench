package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
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
		for service, container := range parseComposeContainerNames(content) {
			out[service] = container
		}
	}
	for _, content := range stringMapFromAny(compose["generatedFiles"]) {
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

func parseComposeContainerNames(content string) map[string]string {
	out := map[string]string{}
	inServices := false
	currentService := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == 0 {
			inServices = trimmed == composeServicesHeader
			currentService = ""
			continue
		}
		if !inServices {
			continue
		}
		if indent == 2 && strings.HasSuffix(trimmed, ":") {
			currentService = strings.TrimSuffix(trimmed, ":")
			continue
		}
		if currentService == "" || !strings.HasPrefix(trimmed, "container_name:") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, "container_name:"))
		name = strings.Trim(name, `"'`)
		if name != "" {
			out[currentService] = name
		}
	}
	return out
}

func environmentRestoreComposeBindMountSources(compose map[string]any, workspace string) map[string][]string {
	out := map[string][]string{}
	for _, content := range environmentRestoreComposeFileContents(compose, workspace) {
		for service, sources := range parseComposeBindMountSources(content) {
			out[service] = append(out[service], sources...)
		}
	}
	return out
}

func environmentRestoreComposeImageReferences(compose map[string]any, workspace string) map[string]string {
	out := map[string]string{}
	for _, content := range environmentRestoreComposeFileContents(compose, workspace) {
		for service, image := range parseComposeImageReferences(content) {
			out[service] = image
		}
	}
	return out
}

func environmentRestoreComposeFileContents(compose map[string]any, workspace string) []string {
	contents := []string{}
	generated := stringMapFromAny(compose["generatedFiles"])
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
	out := map[string]string{}
	var doc struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(content), &doc); err == nil {
		for service, config := range doc.Services {
			image := cleanComposeScalar(config.Image)
			if service != "" && image != "" {
				out[service] = image
			}
		}
		return out
	}
	walkComposeServiceLines(content, func(service string, trimmed string) {
		if strings.HasPrefix(trimmed, "image:") {
			image := cleanComposeScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "image:")))
			if image != "" {
				out[service] = image
			}
		}
	})
	return out
}

func parseComposeBindMountSources(content string) map[string][]string {
	out := map[string][]string{}
	state := composeBindMountParseState{}
	for _, line := range strings.Split(content, "\n") {
		service, source := state.bindSource(line)
		if source != "" {
			out[service] = append(out[service], source)
		}
	}
	return out
}

type composeBindMountParseState struct {
	inServices     bool
	servicesIndent int
	serviceIndent  int
	currentService string
	inVolumes      bool
	volumesIndent  int
}

func (state *composeBindMountParseState) bindSource(line string) (string, string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", ""
	}
	indent := leadingSpaceCount(line)
	if !state.inServices {
		state.enterServices(trimmed, indent)
		return "", ""
	}
	if indent <= state.servicesIndent {
		state.reset()
		return "", ""
	}
	if state.enterService(trimmed, indent) {
		return "", ""
	}
	if state.currentService == "" || indent <= state.serviceIndent {
		state.inVolumes = false
		return "", ""
	}
	if state.enterVolumes(trimmed, indent) {
		return "", ""
	}
	if state.inVolumes && indent <= state.volumesIndent {
		state.inVolumes = false
	}
	if !state.inVolumes {
		return "", ""
	}
	return state.currentService, composeVolumeSource(trimmed)
}

func (state *composeBindMountParseState) enterServices(trimmed string, indent int) {
	if trimmed != composeServicesHeader {
		return
	}
	state.inServices = true
	state.servicesIndent = indent
	state.serviceIndent = -1
}

func (state *composeBindMountParseState) reset() {
	state.inServices = false
	state.currentService = ""
	state.inVolumes = false
	state.serviceIndent = -1
}

func (state *composeBindMountParseState) enterService(trimmed string, indent int) bool {
	if strings.HasPrefix(trimmed, "-") || !strings.HasSuffix(trimmed, ":") {
		return false
	}
	if state.serviceIndent >= 0 && indent != state.serviceIndent {
		return false
	}
	state.serviceIndent = indent
	state.currentService = strings.TrimSpace(strings.TrimSuffix(trimmed, ":"))
	state.inVolumes = false
	return true
}

func (state *composeBindMountParseState) enterVolumes(trimmed string, indent int) bool {
	if strings.TrimSuffix(trimmed, ":") != "volumes" {
		return false
	}
	state.inVolumes = true
	state.volumesIndent = indent
	return true
}

func composeVolumeSource(trimmed string) string {
	if strings.HasPrefix(trimmed, "- ") {
		return environmentRestoreShortVolumeSource(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
	}
	if strings.HasPrefix(trimmed, "source:") {
		return cleanComposeScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "source:")))
	}
	return ""
}

func walkComposeServiceLines(content string, visit func(service string, trimmed string)) {
	inServices := false
	servicesIndent := -1
	serviceIndent := -1
	currentService := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaceCount(line)
		if !inServices {
			if trimmed == composeServicesHeader {
				inServices = true
				servicesIndent = indent
			}
			continue
		}
		if indent <= servicesIndent {
			break
		}
		if strings.HasPrefix(trimmed, "-") {
			continue
		}
		if strings.HasSuffix(trimmed, ":") && (serviceIndent < 0 || indent == serviceIndent) {
			serviceIndent = indent
			currentService = strings.TrimSpace(strings.TrimSuffix(trimmed, ":"))
			continue
		}
		if currentService != "" && indent > serviceIndent {
			visit(currentService, trimmed)
		}
	}
}

func environmentRestoreShortVolumeSource(entry string) string {
	entry = cleanComposeScalar(entry)
	if entry == "" || strings.HasPrefix(entry, "type:") {
		return ""
	}
	if !strings.HasPrefix(entry, "/") && !strings.HasPrefix(entry, "$") {
		return ""
	}
	source, _, ok := strings.Cut(entry, ":")
	if !ok {
		return ""
	}
	return cleanComposeScalar(source)
}

func cleanComposeScalar(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	if cut, _, ok := strings.Cut(value, " #"); ok {
		value = strings.TrimSpace(cut)
	}
	return strings.Trim(value, `"'`)
}
