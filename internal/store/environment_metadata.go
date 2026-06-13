package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

func EnvironmentComposeJSONWithoutGeneratedFiles(compose map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range compose {
		if key == "generatedFiles" {
			continue
		}
		out[key] = value
	}
	return out
}

func EnvironmentFilesFromComposeJSON(compose map[string]any, source string) []EnvironmentFile {
	generated := stringMapFromJSONAny(compose["generatedFiles"])
	seen := map[string]bool{}
	files := make([]EnvironmentFile, 0, len(generated))
	add := func(kind string, path string, order int) {
		path = cleanEnvironmentFilePath(path)
		if path == "" {
			return
		}
		key := kind + "\x00" + path
		if seen[key] {
			return
		}
		seen[key] = true
		files = append(files, EnvironmentFile{
			Path:          path,
			Kind:          kind,
			ContentInline: generated[path],
			Required:      true,
			ApplyOrder:    order,
			SummaryJSON:   sourceSummaryJSON(source),
		})
	}
	for index, path := range stringSliceFromJSONAny(compose["composeFiles"]) {
		add(EnvironmentFileKindComposeFile, path, 10+index)
	}
	for index, path := range stringSliceFromJSONAny(compose["envFiles"]) {
		add(EnvironmentFileKindComposeEnvFile, path, 100+index)
	}
	order := 200
	for path := range generated {
		if seen[EnvironmentFileKindComposeFile+"\x00"+cleanEnvironmentFilePath(path)] || seen[EnvironmentFileKindComposeEnvFile+"\x00"+cleanEnvironmentFilePath(path)] {
			continue
		}
		add(EnvironmentFileKindStartupFile, path, order)
		order++
	}
	return NormalizeEnvironmentFiles(files)
}

func EnvironmentFilesForGeneratedUpdates(compose map[string]any, generated map[string]string, source string) []EnvironmentFile {
	composeFiles := map[string]bool{}
	for _, path := range stringSliceFromJSONAny(compose["composeFiles"]) {
		if clean := cleanEnvironmentFilePath(path); clean != "" {
			composeFiles[clean] = true
		}
	}
	envFiles := map[string]bool{}
	for _, path := range stringSliceFromJSONAny(compose["envFiles"]) {
		if clean := cleanEnvironmentFilePath(path); clean != "" {
			envFiles[clean] = true
		}
	}
	out := make([]EnvironmentFile, 0, len(generated))
	order := 200
	for path, content := range generated {
		cleanPath := cleanEnvironmentFilePath(path)
		if cleanPath == "" {
			continue
		}
		kind := EnvironmentFileKindStartupFile
		applyOrder := order
		if composeFiles[cleanPath] {
			kind = EnvironmentFileKindComposeFile
			applyOrder = 10
		} else if envFiles[cleanPath] {
			kind = EnvironmentFileKindComposeEnvFile
			applyOrder = 100
		}
		out = append(out, EnvironmentFile{
			Path:          cleanPath,
			Kind:          kind,
			ContentInline: content,
			Required:      true,
			ApplyOrder:    applyOrder,
			SummaryJSON:   sourceSummaryJSON(source),
		})
		order++
	}
	return NormalizeEnvironmentFiles(out)
}

func EnvironmentServicesFromJSON(services []any, repos map[string]any, source string) []EnvironmentService {
	byID := map[string]EnvironmentService{}
	for _, raw := range services {
		item := jsonObjectFromAny(raw)
		id := strings.TrimSpace(valueStringFromAny(item["id"]))
		if id == "" {
			continue
		}
		service := byID[id]
		service.ServiceID = id
		if value := strings.TrimSpace(valueStringFromAny(item["repo"])); value != "" {
			service.RepoURL = value
		}
		if value := strings.TrimSpace(valueStringFromAny(item["branch"])); value != "" {
			service.Branch = value
		}
		if value := strings.TrimSpace(valueStringFromAny(item["ref"])); value != "" {
			service.Ref = value
		}
		if value := strings.TrimSpace(valueStringFromAny(item["checkout"])); value != "" {
			service.Checkout = value
		}
		byID[id] = service
	}
	for id, raw := range repos {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		repo := jsonObjectFromAny(raw)
		service := byID[id]
		service.ServiceID = id
		if value := strings.TrimSpace(valueStringFromAny(repo["url"])); value != "" {
			service.RepoURL = value
		}
		if value := strings.TrimSpace(valueStringFromAny(repo["branch"])); value != "" {
			service.Branch = value
		}
		if value := strings.TrimSpace(valueStringFromAny(repo["ref"])); value != "" {
			service.Ref = value
		}
		if value := strings.TrimSpace(valueStringFromAny(repo["checkout"])); value != "" {
			service.Checkout = value
		}
		byID[id] = service
	}
	out := make([]EnvironmentService, 0, len(byID))
	for _, service := range byID {
		service.SummaryJSON = sourceSummaryJSON(source)
		out = append(out, service)
	}
	return NormalizeEnvironmentServices(out)
}

func EnvironmentHealthChecksFromJSON(checks []any, source string) []EnvironmentHealthCheck {
	out := make([]EnvironmentHealthCheck, 0, len(checks))
	for index, raw := range checks {
		item := jsonObjectFromAny(raw)
		id := strings.TrimSpace(valueStringFromAny(item["id"]))
		if id == "" {
			id = fmt.Sprintf("health-%02d", index+1)
		}
		kind := strings.TrimSpace(valueStringFromAny(item["kind"]))
		if kind == "" {
			kind = environmentHealthCheckKindFromLegacyJSON(item)
		}
		check := EnvironmentHealthCheck{
			CheckID:        id,
			Kind:           kind,
			URL:            strings.TrimSpace(valueStringFromAny(item["url"])),
			Address:        strings.TrimSpace(valueStringFromAny(item["address"])),
			Command:        strings.TrimSpace(valueStringFromAny(item["command"])),
			ComposeService: strings.TrimSpace(valueStringFromAny(item["service"])),
			Expect:         strings.TrimSpace(valueStringFromAny(item["expect"])),
			ApplyOrder:     index + 1,
			SummaryJSON:    sourceSummaryJSON(source),
		}
		out = append(out, check)
	}
	return NormalizeEnvironmentHealthChecks(out)
}

func environmentHealthCheckKindFromLegacyJSON(item map[string]any) string {
	switch {
	case strings.TrimSpace(valueStringFromAny(item["service"])) != "":
		return "compose-service"
	case strings.TrimSpace(valueStringFromAny(item["url"])) != "":
		return "url"
	case strings.TrimSpace(valueStringFromAny(item["address"])) != "":
		return "tcp"
	case strings.TrimSpace(valueStringFromAny(item["command"])) != "":
		return "command"
	default:
		return ""
	}
}

func stringSliceFromJSONAny(value any) []string {
	if raw, ok := value.([]string); ok {
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if text := strings.TrimSpace(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	}
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if text := strings.TrimSpace(valueStringFromAny(item)); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func jsonObjectFromAny(value any) map[string]any {
	if raw, ok := value.(map[string]any); ok {
		return raw
	}
	return map[string]any{}
}

func valueStringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func sourceSummaryJSON(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "{}"
	}
	raw, err := json.Marshal(map[string]string{"source": source})
	if err != nil {
		return "{}"
	}
	return string(raw)
}
