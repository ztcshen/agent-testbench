// Package composefile parses Docker Compose file fragments used by environment restore diagnostics.
package composefile

import (
	"strings"

	"gopkg.in/yaml.v3"
)

const servicesHeader = "services:"

func ParseContainerNames(content string) map[string]string {
	out := map[string]string{}
	inServices := false
	currentService := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaceCount(line)
		if indent == 0 {
			inServices = trimmed == servicesHeader
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

func ParseImageReferences(content string) map[string]string {
	out := map[string]string{}
	var doc struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(content), &doc); err == nil {
		for service, config := range doc.Services {
			image := CleanScalar(config.Image)
			if service != "" && image != "" {
				out[service] = image
			}
		}
		return out
	}
	walkServiceLines(content, func(service string, trimmed string) {
		if strings.HasPrefix(trimmed, "image:") {
			image := CleanScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "image:")))
			if image != "" {
				out[service] = image
			}
		}
	})
	return out
}

func ParseBindMountSources(content string) map[string][]string {
	out := map[string][]string{}
	state := bindMountParseState{}
	for _, line := range strings.Split(content, "\n") {
		service, source := state.bindSource(line)
		if source != "" {
			out[service] = append(out[service], source)
		}
	}
	return out
}

func RewriteBindMountSources(content string, replacement func(source string) string) (string, bool) {
	lines := strings.Split(content, "\n")
	state := bindMountParseState{}
	changed := false
	for index, line := range lines {
		_, source := state.bindSource(line)
		nextSource := replacement(source)
		if nextSource == "" || nextSource == source {
			continue
		}
		next := strings.Replace(line, source, nextSource, 1)
		if next != line {
			lines[index] = next
			changed = true
		}
	}
	if !changed {
		return content, false
	}
	return strings.Join(lines, "\n"), true
}

type bindMountParseState struct {
	inServices     bool
	servicesIndent int
	serviceIndent  int
	currentService string
	inVolumes      bool
	volumesIndent  int
}

func (state *bindMountParseState) bindSource(line string) (string, string) {
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
	return state.currentService, volumeSource(trimmed)
}

func (state *bindMountParseState) enterServices(trimmed string, indent int) {
	if trimmed != servicesHeader {
		return
	}
	state.inServices = true
	state.servicesIndent = indent
	state.serviceIndent = -1
}

func (state *bindMountParseState) reset() {
	state.inServices = false
	state.currentService = ""
	state.inVolumes = false
	state.serviceIndent = -1
}

func (state *bindMountParseState) enterService(trimmed string, indent int) bool {
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

func (state *bindMountParseState) enterVolumes(trimmed string, indent int) bool {
	if strings.TrimSuffix(trimmed, ":") != "volumes" {
		return false
	}
	state.inVolumes = true
	state.volumesIndent = indent
	return true
}

func volumeSource(trimmed string) string {
	if strings.HasPrefix(trimmed, "- ") {
		source, _, ok := ParseShortVolume(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		if ok {
			return source
		}
		return ""
	}
	if strings.HasPrefix(trimmed, "source:") {
		return CleanScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "source:")))
	}
	return ""
}

func ParseShortVolume(value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	if strings.HasPrefix(value, "[") ||
		strings.HasPrefix(value, "{") ||
		strings.HasPrefix(value, "type:") ||
		strings.HasPrefix(value, "source:") ||
		strings.HasPrefix(value, "target:") {
		return "", "", false
	}
	source, target, ok := splitShortVolume(value)
	if !ok {
		return "", "", false
	}
	source = strings.Trim(source, `"' `)
	target = strings.Trim(target, `"' `)
	if source == "" || target == "" {
		return "", "", false
	}
	if !HostSourceLooksLikePath(source) {
		return "", "", false
	}
	return source, target, true
}

func splitShortVolume(value string) (string, string, bool) {
	depth := 0
	firstColon := -1
	for i := 0; i < len(value); i++ {
		switch {
		case value[i] == '$' && i+1 < len(value) && value[i+1] == '{':
			depth++
			i++
		case value[i] == '}' && depth > 0:
			depth--
		case value[i] == ':' && depth == 0:
			if firstColon < 0 {
				firstColon = i
				continue
			}
			return value[:firstColon], value[firstColon+1 : i], true
		}
	}
	if firstColon < 0 {
		return "", "", false
	}
	return value[:firstColon], value[firstColon+1:], true
}

func HostSourceLooksLikePath(source string) bool {
	return strings.HasPrefix(source, ".") ||
		strings.HasPrefix(source, "/") ||
		strings.HasPrefix(source, "~") ||
		strings.HasPrefix(source, "$") ||
		strings.HasPrefix(source, "${")
}

func walkServiceLines(content string, visit func(service string, trimmed string)) {
	inServices := false
	servicesIndent := -1
	serviceIndent := -1
	fieldIndent := -1
	currentService := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaceCount(line)
		if !inServices {
			if trimmed == servicesHeader {
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
			fieldIndent = -1
			currentService = strings.TrimSpace(strings.TrimSuffix(trimmed, ":"))
			continue
		}
		if currentService != "" && indent > serviceIndent {
			if fieldIndent < 0 {
				fieldIndent = indent
			}
			if indent == fieldIndent {
				visit(currentService, trimmed)
			}
		}
	}
}

func CleanScalar(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	if cut, _, ok := strings.Cut(value, " #"); ok {
		value = strings.TrimSpace(cut)
	}
	return strings.Trim(value, `"'`)
}

func leadingSpaceCount(value string) int {
	count := 0
	for _, r := range value {
		if r != ' ' {
			break
		}
		count++
	}
	return count
}
