// Package environmentfiles derives Store-backed file projection readiness for
// Environment Catalog compose, env, generated file, and component asset data.
package environmentfiles

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"agent-testbench/internal/domain/environmentsource"
	"agent-testbench/internal/store"

	"gopkg.in/yaml.v3"
)

const (
	KindComposeFile       = "compose-file"
	KindEnvFile           = "env-file"
	KindComposeConfigFile = "compose-config-file"
	KindComposeSecretFile = "compose-secret-file"
	KindGeneratedEnv      = "generated-compose-env"
	KindGenerated         = "generated-file"
	KindAsset             = "component-asset"
)

const (
	assetKindComposeConfig = "compose-config"
	assetKindComposeSecret = "compose-secret"
)

type ProjectionReport struct {
	OK         bool                   `json:"ok"`
	Files      []ProjectionFile       `json:"files"`
	Missing    []ProjectionFile       `json:"missing,omitempty"`
	RepairPlan []ProjectionRepairItem `json:"repairPlan,omitempty"`
	Counts     ProjectionCounts       `json:"counts"`
}

type ProjectionCounts struct {
	Referenced  int `json:"referenced"`
	StoreBacked int `json:"storeBacked"`
	Missing     int `json:"missing"`
	RepairItems int `json:"repairItems,omitempty"`
}

type ProjectionRepairItem struct {
	Name          string   `json:"name"`
	Target        string   `json:"target"`
	Missing       []string `json:"missing,omitempty"`
	Action        string   `json:"action"`
	CommandHint   string   `json:"commandHint"`
	StoreBacked   bool     `json:"storeBacked"`
	BlocksRestore bool     `json:"blocksRestore"`
}

type ProjectionFile struct {
	Path              string `json:"path"`
	Kind              string `json:"kind"`
	Source            string `json:"source"`
	ProjectionRule    string `json:"projectionRule,omitempty"`
	Required          bool   `json:"required,omitempty"`
	StoreBacked       bool   `json:"storeBacked"`
	OK                bool   `json:"ok"`
	AssetID           string `json:"assetId,omitempty"`
	OwnerComponentID  string `json:"ownerComponentId,omitempty"`
	TargetComponentID string `json:"targetComponentId,omitempty"`
	Mode              string `json:"mode,omitempty"`
	Error             string `json:"error,omitempty"`
}

func FromEnvironment(env store.Environment, graph store.EnvironmentComponentGraph) ProjectionReport {
	return FromCompose(jsonObject(env.ComposeJSON), jsonObject(env.SummaryJSON), graph)
}

func FromCompose(compose map[string]any, summary map[string]any, graph store.EnvironmentComponentGraph) ProjectionReport {
	assetFiles := projectionFilesFromAssets(graph.Assets)
	builder := projectionBuilder{
		compose:       compose,
		generated:     stringMap(compose["generatedFiles"]),
		generatedMode: stringMap(compose["generatedFileModes"]),
		startupFiles:  startupFileSet(summary),
		packageSource: strings.TrimSpace(valueString(jsonObjectFromAny(compose["package"])["url"])) != "",
		assetByPath:   projectionFilesByPath(assetFiles),
		assetContent:  projectionAssetContentByPath(graph.Assets),
	}
	builder.env = projectionComposeEnv(compose, builder.generated, builder.assetContent)
	builder.addReferencedFiles(KindComposeFile, composeFiles(compose))
	builder.addReferencedFiles(KindEnvFile, stringSlice(compose["envFiles"]))
	builder.addComposeContentReferences(composeFiles(compose))
	if len(stringMap(compose["env"])) > 0 {
		builder.add(ProjectionFile{
			Path:           filepath.ToSlash(filepath.Join(".agent-testbench", "restore.env")),
			Kind:           KindGeneratedEnv,
			Source:         "compose.env",
			ProjectionRule: "compose-env-file",
			Required:       true,
			StoreBacked:    true,
			OK:             true,
		})
	}
	for path := range builder.generated {
		if builder.seen[projectionKey(KindComposeFile, path)] || builder.seen[projectionKey(KindEnvFile, path)] {
			continue
		}
		builder.add(builder.generatedFile(path, KindGenerated, false))
	}
	for _, file := range assetFiles {
		builder.add(file)
	}
	return builder.report()
}

type projectionBuilder struct {
	compose       map[string]any
	generated     map[string]string
	generatedMode map[string]string
	env           map[string]string
	startupFiles  map[string]bool
	packageSource bool
	assetByPath   map[string]ProjectionFile
	assetContent  map[string]string
	files         []ProjectionFile
	seen          map[string]bool
}

func (b *projectionBuilder) addReferencedFiles(kind string, paths []string) {
	for _, path := range paths {
		path = cleanPath(path)
		if path == "" {
			continue
		}
		b.add(b.referencedFile(path, kind))
	}
}

func (b *projectionBuilder) addComposeContentReferences(paths []string) {
	scanned := map[string]bool{}
	queue := make([]composeContentScanTarget, 0, len(paths))
	for _, path := range paths {
		queue = append(queue, composeContentScanTarget{path: path})
	}
	for len(queue) > 0 {
		target := queue[0]
		queue = queue[1:]
		cleanCompose := cleanPath(target.path)
		scanKey := cleanCompose + "\x00" + strings.TrimSpace(target.service)
		if cleanCompose == "" || scanned[scanKey] {
			continue
		}
		scanned[scanKey] = true
		content := b.generated[cleanCompose]
		if content == "" {
			content = b.assetContent[cleanCompose]
		}
		if content == "" {
			continue
		}
		for _, ref := range composeContentFileReferences(content, cleanCompose, target.service, b.env) {
			if ref.err != "" {
				b.add(b.unresolvedReferenceFile(ref.path, ref.kind, ref.source, ref.err))
				continue
			}
			b.add(b.referencedFile(ref.path, ref.kind))
			if ref.kind == KindComposeFile {
				queue = append(queue, composeContentScanTarget{path: ref.path, service: ref.service})
			}
		}
	}
}

func (b *projectionBuilder) referencedFile(path string, kind string) ProjectionFile {
	if _, ok := b.generated[path]; ok {
		return b.generatedFile(path, kind, true)
	}
	if asset, ok := b.assetByPath[path]; ok {
		asset.Kind = kind
		asset.Required = true
		return asset
	}
	if b.packageSource {
		return ProjectionFile{
			Path:           path,
			Kind:           kind,
			Source:         "environment-package",
			ProjectionRule: "package-checkout",
			Required:       true,
			StoreBacked:    true,
			OK:             true,
		}
	}
	file := ProjectionFile{
		Path:        path,
		Kind:        kind,
		Source:      "workspace-file",
		Required:    true,
		StoreBacked: false,
		OK:          false,
		Error:       "referenced file is not backed by compose.generatedFiles, component asset, or environment package metadata",
	}
	if b.startupFiles[path] {
		file.Source = "summary.startupFiles"
		file.Error = "startup file summary exists but compose.generatedFiles content is missing"
	}
	return file
}

func (b *projectionBuilder) unresolvedReferenceFile(path string, kind string, source string, errorText string) ProjectionFile {
	if source == "" {
		source = "compose.interpolation"
	}
	return ProjectionFile{
		Path:        path,
		Kind:        kind,
		Source:      source,
		Required:    true,
		StoreBacked: false,
		OK:          false,
		Error:       errorText,
	}
}

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

// InterpolateComposeText resolves Compose-style variable expressions from env.
func InterpolateComposeText(value string, env map[string]string) (string, bool) {
	return interpolateComposeText(value, env, 0)
}

func interpolateComposeText(value string, env map[string]string, depth int) (string, bool) {
	if depth > 8 {
		return value, false
	}
	var out strings.Builder
	for i := 0; i < len(value); {
		if value[i] != '$' {
			out.WriteByte(value[i])
			i++
			continue
		}
		if i+1 >= len(value) {
			out.WriteByte(value[i])
			i++
			continue
		}
		if value[i+1] == '$' {
			out.WriteByte('$')
			i += 2
			continue
		}
		replacement, next, ok := composeInterpolationReplacement(value, i, env, depth)
		if !ok {
			return value, false
		}
		if next == i {
			out.WriteByte(value[i])
			i++
			continue
		}
		out.WriteString(replacement)
		i = next
	}
	return out.String(), true
}

func composeInterpolationReplacement(value string, start int, env map[string]string, depth int) (string, int, bool) {
	if value[start+1] == '{' {
		return composeBracedInterpolationReplacement(value, start, env, depth)
	}
	nameEnd := start + 1
	for nameEnd < len(value) && composeVariableNameByte(value[nameEnd]) {
		nameEnd++
	}
	if nameEnd == start+1 {
		return "", start, true
	}
	replacement, ok := env[value[start+1:nameEnd]]
	return replacement, nameEnd, ok
}

func composeBracedInterpolationReplacement(value string, start int, env map[string]string, depth int) (string, int, bool) {
	end := composeExpressionEnd(value, start+2)
	if end < 0 {
		return "", start, false
	}
	replacement, ok := resolveComposeVariableExpression(value[start+2:end], env)
	if !ok {
		return "", start, false
	}
	if strings.Contains(replacement, "$") {
		replacement, ok = interpolateComposeText(replacement, env, depth+1)
		if !ok {
			return "", start, false
		}
	}
	return replacement, end + 1, true
}

func composeExpressionEnd(value string, start int) int {
	depth := 0
	for i := start; i < len(value); i++ {
		if value[i] == '$' && i+1 < len(value) && value[i+1] == '{' {
			depth++
			i++
			continue
		}
		if value[i] != '}' {
			continue
		}
		if depth == 0 {
			return i
		}
		depth--
	}
	return -1
}

func resolveComposeVariableExpression(expr string, env map[string]string) (string, bool) {
	nameEnd := 0
	for nameEnd < len(expr) && composeVariableNameByte(expr[nameEnd]) {
		nameEnd++
	}
	if nameEnd == 0 {
		return "", false
	}
	name := expr[:nameEnd]
	opArg := expr[nameEnd:]
	value, exists := env[name]
	switch {
	case opArg == "":
		return value, exists
	case strings.HasPrefix(opArg, ":-"):
		if !exists || value == "" {
			return opArg[2:], true
		}
		return value, true
	case strings.HasPrefix(opArg, "-"):
		if !exists {
			return opArg[1:], true
		}
		return value, true
	case strings.HasPrefix(opArg, ":?"):
		return value, exists && value != ""
	case strings.HasPrefix(opArg, "?"):
		return value, exists
	case strings.HasPrefix(opArg, ":+"):
		if exists && value != "" {
			return opArg[2:], true
		}
		return "", true
	case strings.HasPrefix(opArg, "+"):
		if exists {
			return opArg[1:], true
		}
		return "", true
	default:
		return "", false
	}
}

func composeVariableNameByte(value byte) bool {
	return value == '_' || value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func projectionComposeEnv(compose map[string]any, generated map[string]string, assetContent map[string]string) map[string]string {
	env := stringMap(compose["env"])
	for _, envFile := range stringSlice(compose["envFiles"]) {
		content := generated[cleanPath(envFile)]
		if content == "" {
			content = assetContent[cleanPath(envFile)]
		}
		if content == "" {
			continue
		}
		for key, value := range parseComposeEnvFile(content) {
			env[key] = value
		}
	}
	return env
}

func parseComposeEnvFile(content string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		key, value, ok := parseComposeEnvLine(line)
		if ok {
			out[key] = value
		}
	}
	return out
}

func parseComposeEnvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	sep := strings.IndexByte(line, '=')
	if sep <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:sep])
	if !composeEnvKey(key) {
		return "", "", false
	}
	return key, cleanComposeEnvValue(line[sep+1:]), true
}

func composeEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		if !composeVariableNameByte(key[i]) {
			return false
		}
	}
	return true
}

func cleanComposeEnvValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		return strings.Trim(value[1:len(value)-1], "\r")
	}
	if hash := strings.Index(value, " #"); hash >= 0 {
		value = strings.TrimSpace(value[:hash])
	}
	return strings.Trim(value, "\r")
}

func (b *projectionBuilder) generatedFile(path string, kind string, required bool) ProjectionFile {
	file := ProjectionFile{
		Path:           path,
		Kind:           kind,
		Source:         "compose.generatedFiles",
		ProjectionRule: "store-inline-file",
		Required:       required,
		StoreBacked:    true,
		OK:             true,
		Mode:           strings.TrimSpace(b.generatedMode[path]),
	}
	if !safeRelativePath(path) {
		file.OK = false
		file.Error = "generated file target must be relative to the restore workspace"
	}
	return file
}

func (b *projectionBuilder) add(file ProjectionFile) {
	if b.seen == nil {
		b.seen = map[string]bool{}
	}
	file.Path = cleanPath(file.Path)
	if file.Path == "" {
		return
	}
	key := projectionKey(file.Kind, file.Path)
	if b.seen[key] {
		return
	}
	b.seen[key] = true
	b.files = append(b.files, file)
}

func (b *projectionBuilder) report() ProjectionReport {
	sort.SliceStable(b.files, func(i, j int) bool {
		if b.files[i].Required != b.files[j].Required {
			return b.files[i].Required
		}
		if b.files[i].Kind != b.files[j].Kind {
			return b.files[i].Kind < b.files[j].Kind
		}
		return b.files[i].Path < b.files[j].Path
	})
	report := ProjectionReport{OK: true, Files: b.files}
	for _, file := range b.files {
		if file.Required {
			report.Counts.Referenced++
		}
		if file.StoreBacked {
			report.Counts.StoreBacked++
		}
		if !file.OK {
			report.OK = false
			report.Counts.Missing++
			report.Missing = append(report.Missing, file)
		}
	}
	if !report.OK {
		report.RepairPlan = projectionRepairPlan(report.Missing)
		report.Counts.RepairItems = len(report.RepairPlan)
	}
	return report
}

func projectionRepairPlan(missing []ProjectionFile) []ProjectionRepairItem {
	summaryStartup := []string{}
	unresolvedVariables := []string{}
	unprojectedFiles := []string{}
	for _, file := range missing {
		target := projectionRepairTarget(file)
		switch file.Source {
		case "summary.startupFiles":
			summaryStartup = append(summaryStartup, target)
		case "compose.interpolation":
			unresolvedVariables = append(unresolvedVariables, target)
		default:
			unprojectedFiles = append(unprojectedFiles, target)
		}
	}
	items := []ProjectionRepairItem{}
	if len(summaryStartup) > 0 {
		items = append(items, ProjectionRepairItem{
			Name:          "startup-file-content",
			Target:        "compose.generatedFiles",
			Missing:       dedupeSortedProjectionTargets(summaryStartup),
			Action:        "store the referenced startup file content in compose.generatedFiles; summary.startupFiles is only a repair hint",
			CommandHint:   "environment startup-file put ENV_ID --file PATH=LOCAL_FILE",
			StoreBacked:   true,
			BlocksRestore: true,
		})
	}
	if len(unresolvedVariables) > 0 {
		items = append(items, ProjectionRepairItem{
			Name:          "compose-env-variable",
			Target:        "compose.env",
			Missing:       dedupeSortedProjectionTargets(unresolvedVariables),
			Action:        "record required Compose path variables in Store-backed compose.env or compose.envFiles so file references resolve reproducibly",
			CommandHint:   "environment register --id ENV_ID --compose-env KEY=VALUE --verification-workflow WORKFLOW_ID",
			StoreBacked:   true,
			BlocksRestore: true,
		})
	}
	if len(unprojectedFiles) > 0 {
		items = append(items, ProjectionRepairItem{
			Name:          "compose-file-projection",
			Target:        "fileProjection.missing",
			Missing:       dedupeSortedProjectionTargets(unprojectedFiles),
			Action:        "store every referenced Compose env/config/secret/include/extends file as a generated file, component asset, or environment package projection",
			CommandHint:   "environment startup-file put ENV_ID --file PATH=LOCAL_FILE",
			StoreBacked:   true,
			BlocksRestore: true,
		})
	}
	return items
}

func projectionRepairTarget(file ProjectionFile) string {
	kind := strings.TrimSpace(file.Kind)
	if kind == "" {
		kind = "file"
	}
	return kind + ":" + filepath.ToSlash(cleanPath(file.Path))
}

func dedupeSortedProjectionTargets(values []string) []string {
	out := dedupeProjectionTargets(values)
	sort.Strings(out)
	return out
}

func dedupeProjectionTargets(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func projectionFilesFromAssets(assets []store.ComponentConfigAsset) []ProjectionFile {
	files := []ProjectionFile{}
	for _, asset := range assets {
		if file := projectionFileFromAsset(asset); strings.TrimSpace(file.Path) != "" {
			files = append(files, file)
		}
	}
	return files
}

func projectionFilesByPath(files []ProjectionFile) map[string]ProjectionFile {
	out := map[string]ProjectionFile{}
	for _, file := range files {
		if strings.TrimSpace(file.Path) == "" {
			continue
		}
		if existing, ok := out[file.Path]; ok && existing.OK {
			continue
		}
		out[file.Path] = file
	}
	return out
}

func projectionAssetContentByPath(assets []store.ComponentConfigAsset) map[string]string {
	out := map[string]string{}
	for _, asset := range assets {
		path := cleanPath(asset.TargetPath)
		content := strings.TrimSpace(asset.ContentInline)
		if path == "" || content == "" {
			continue
		}
		if _, exists := out[path]; exists {
			continue
		}
		out[path] = asset.ContentInline
	}
	return out
}

func projectionFileFromAsset(asset store.ComponentConfigAsset) ProjectionFile {
	path := cleanPath(asset.TargetPath)
	file := ProjectionFile{
		Path:              path,
		Kind:              KindAsset,
		Source:            "component_config_assets",
		ProjectionRule:    assetProjectionRule(asset.AssetKind),
		StoreBacked:       true,
		OK:                true,
		AssetID:           strings.TrimSpace(asset.AssetID),
		OwnerComponentID:  strings.TrimSpace(asset.OwnerComponentID),
		TargetComponentID: strings.TrimSpace(asset.TargetComponentID),
	}
	if !safeRelativePath(path) {
		file.OK = false
		file.Error = "component asset target must be relative to the restore workspace"
	}
	if strings.TrimSpace(asset.ContentInline) == "" && !environmentsource.ComponentAssetRemoteRefOK(asset.TargetPath, asset.RemoteRefJSON) {
		file.OK = false
		file.Error = "component asset must provide inline content or a valid remote ref"
	}
	return file
}

func assetProjectionRule(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case assetKindComposeConfig, "docker-config":
		return assetKindComposeConfig
	case assetKindComposeSecret, "docker-secret":
		return assetKindComposeSecret
	case "env-file", "compose-env-file":
		return "compose-env-file"
	case "mysql-sql", "mysql-initdb":
		return "mysql-initdb"
	case "mysql-migration":
		return "mysql-migration"
	default:
		return "generated-file"
	}
}

func composeFiles(compose map[string]any) []string {
	files := stringSlice(compose["composeFiles"])
	if len(files) == 0 {
		if file := strings.TrimSpace(valueString(compose["composeFile"])); file != "" {
			files = []string{file}
		}
	}
	return files
}

func startupFileSet(summary map[string]any) map[string]bool {
	out := map[string]bool{}
	startup := jsonObjectFromAny(summary["startupFiles"])
	for _, raw := range sliceAny(startup["files"]) {
		item := jsonObjectFromAny(raw)
		if path := cleanPath(valueString(firstNonNil(item["path"], item["target"]))); path != "" {
			out[path] = true
		}
	}
	return out
}

func cleanPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func safeRelativePath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || path == "." || filepath.IsAbs(path) {
		return false
	}
	clean := filepath.Clean(path)
	return clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator)) && !strings.HasPrefix(filepath.ToSlash(clean), "../")
}

func projectionKey(kind string, path string) string {
	return strings.TrimSpace(kind) + "\x00" + cleanPath(path)
}

func jsonObject(raw string) map[string]any {
	out := map[string]any{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

func jsonObjectFromAny(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = value
		}
		return out
	default:
		return map[string]any{}
	}
}

func stringMap(value any) map[string]string {
	out := map[string]string{}
	for key, raw := range jsonObjectFromAny(value) {
		if key = strings.TrimSpace(key); key != "" {
			out[cleanPath(key)] = strings.TrimSpace(valueString(raw))
		}
	}
	return out
}

func stringSlice(value any) []string {
	out := []string{}
	for _, raw := range sliceAny(value) {
		if item := strings.TrimSpace(valueString(raw)); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func sliceAny(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func valueString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
