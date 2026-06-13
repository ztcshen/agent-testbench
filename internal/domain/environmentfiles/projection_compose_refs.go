package environmentfiles

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type composeContentScanTarget struct {
	path    string
	service string
}

type composeContentFileReference struct {
	kind    string
	path    string
	service string
	source  string
	err     string
}

func composeContentFileReferences(content string, composeFile string, service string, env map[string]string) []composeContentFileReference {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return nil
	}
	collector := composeReferenceCollector{composeDir: filepath.Dir(cleanPath(composeFile)), service: strings.TrimSpace(service), env: env}
	collector.collect(doc)
	return collector.refs
}

type composeReferenceCollector struct {
	composeDir string
	service    string
	env        map[string]string
	refs       []composeContentFileReference
}

func (c *composeReferenceCollector) collect(doc map[string]any) {
	if c.service != "" {
		c.collectServiceOnly(doc)
		return
	}
	c.collectInclude(doc["include"])
	c.collectComposeResourceFiles(KindComposeConfigFile, doc["configs"])
	c.collectComposeResourceFiles(KindComposeSecretFile, doc["secrets"])
	for _, service := range composeMap(doc["services"]) {
		serviceMap := composeMap(service)
		c.addReferences(KindEnvFile, serviceMap["env_file"])
		c.collectExtendsReference(serviceMap)
	}
}

func (c *composeReferenceCollector) collectServiceOnly(doc map[string]any) {
	serviceMap := composeMap(composeMap(doc["services"])[c.service])
	if len(serviceMap) == 0 {
		return
	}
	c.addReferences(KindEnvFile, serviceMap["env_file"])
	c.collectServiceResourceFiles(KindComposeConfigFile, serviceMap["configs"], doc["configs"])
	c.collectServiceResourceFiles(KindComposeSecretFile, serviceMap["secrets"], doc["secrets"])
	c.collectExtendsReference(serviceMap)
}

func (c *composeReferenceCollector) collectExtendsReference(serviceMap map[string]any) {
	extendsMap := composeMap(serviceMap["extends"])
	if len(extendsMap) == 0 {
		return
	}
	extendsFile, ok := extendsMap["file"]
	if !ok {
		return
	}
	service := strings.TrimSpace(composeString(extendsMap["service"]))
	c.addReferencesWithService(KindComposeFile, extendsFile, service)
}

func (c *composeReferenceCollector) collectInclude(value any) {
	for _, item := range composeList(value) {
		itemMap := composeMap(item)
		if len(itemMap) == 0 {
			c.addReferences(KindComposeFile, item)
			continue
		}
		c.addReferences(KindComposeFile, itemMap["path"])
		c.addReferences(KindEnvFile, itemMap["env_file"])
	}
}

func (c *composeReferenceCollector) collectComposeResourceFiles(kind string, value any) {
	for _, resource := range composeMap(value) {
		if file, ok := composeMap(resource)["file"]; ok {
			c.addReferences(kind, file)
		}
	}
}

func (c *composeReferenceCollector) collectServiceResourceFiles(kind string, serviceValue any, resources any) {
	resourceMap := composeMap(resources)
	for _, name := range composeServiceResourceNames(serviceValue) {
		if file, ok := composeMap(resourceMap[name])["file"]; ok {
			c.addReferences(kind, file)
		}
	}
}

func composeServiceResourceNames(value any) []string {
	out := []string{}
	if name := composeString(value); name != "" {
		return []string{name}
	}
	for _, item := range composeList(value) {
		itemMap := composeMap(item)
		if len(itemMap) == 0 {
			if name := composeString(item); name != "" {
				out = append(out, name)
			}
			continue
		}
		if name := composeString(firstNonNil(itemMap["source"], itemMap["config"], itemMap["secret"])); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func (c *composeReferenceCollector) addReferences(kind string, value any) {
	c.addReferencesWithService(kind, value, "")
}

func (c *composeReferenceCollector) addReferencesWithService(kind string, value any, service string) {
	if path := composeString(value); path != "" {
		c.addReference(kind, path, service)
		return
	}
	for _, item := range composeList(value) {
		itemMap := composeMap(item)
		if len(itemMap) == 0 {
			c.addReferencesWithService(kind, item, service)
			continue
		}
		if kind == KindEnvFile && !composeBool(itemMap["required"], true) {
			continue
		}
		if path, ok := itemMap["path"]; ok {
			c.addReferencesWithService(kind, path, service)
		}
	}
}

func (c *composeReferenceCollector) addReference(kind string, path string, service string) {
	resolvedPath, source, errText := cleanComposeReferencedPath(path, c.composeDir, c.env)
	path = resolvedPath
	if path == "" {
		return
	}
	c.refs = append(c.refs, composeContentFileReference{kind: kind, path: path, service: strings.TrimSpace(service), source: source, err: errText})
}

func composeMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			if keyString, ok := key.(string); ok {
				out[keyString] = value
			}
		}
		return out
	default:
		return nil
	}
}

func composeList(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case nil:
		return nil
	default:
		return []any{value}
	}
}

func composeString(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func composeBool(value any, defaultValue bool) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	default:
		return defaultValue
	}
}

func cleanComposeReferencedPath(path string, composeDir string, env map[string]string) (string, string, string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", ""
	}
	if strings.HasPrefix(path, "~") || filepath.IsAbs(path) {
		return cleanPath(path), "compose.path", "compose file reference must be relative and Store-projected, not a host-local absolute or home path"
	}
	if strings.Contains(path, "$") {
		resolved, ok := InterpolateComposeText(path, env)
		if !ok {
			return cleanComposePathWithDir(path, composeDir), "compose.interpolation", "compose file reference contains variables that are not resolved by Store-backed compose.env"
		}
		path = resolved
		if strings.HasPrefix(path, "~") || filepath.IsAbs(path) {
			return cleanPath(path), "compose.path", "resolved compose file reference must stay relative to the restore workspace"
		}
	}
	return cleanComposePathWithDir(path, composeDir), "", ""
}

func cleanComposePathWithDir(path string, composeDir string) string {
	if composeDir == "." || composeDir == "" {
		return cleanPath(path)
	}
	return cleanPath(filepath.Join(composeDir, path))
}
