// Package environmentfiles derives Store-backed file projection readiness for
// Environment Catalog compose, env, generated file, and component asset data.
package environmentfiles

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
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

type ProjectionAsset struct {
	OwnerComponentID  string
	AssetID           string
	AssetKind         string
	TargetComponentID string
	TargetPath        string
	ContentInline     string
	RemoteRefJSON     string
}

type ProjectionSource struct {
	Path   string
	Source string
}

func FromJSON(composeJSON string, summaryJSON string, assets []ProjectionAsset) ProjectionReport {
	return FromCompose(jsonObject(composeJSON), jsonObject(summaryJSON), assets)
}

func FromCompose(compose map[string]any, summary map[string]any, assets []ProjectionAsset) ProjectionReport {
	return FromComposeWithSources(compose, summary, assets, nil)
}

func FromComposeWithSources(compose map[string]any, summary map[string]any, assets []ProjectionAsset, sources []ProjectionSource) ProjectionReport {
	assetFiles := projectionFilesFromAssets(assets)
	builder := projectionBuilder{
		compose:       compose,
		generated:     stringMap(compose["generatedFiles"]),
		generatedMode: stringMap(compose["generatedFileModes"]),
		sourceByPath:  projectionSourceByPath(sources),
		startupFiles:  startupFileSet(summary),
		packageSource: strings.TrimSpace(valueString(jsonObjectFromAny(compose["package"])["url"])) != "",
		assetByPath:   projectionFilesByPath(assetFiles),
		assetContent:  projectionAssetContentByPath(assets),
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
	sourceByPath  map[string]string
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
		Error:       "referenced file is not backed by environment_files, component asset, legacy generatedFiles, or environment package metadata",
	}
	if b.startupFiles[path] {
		file.Source = "summary.startupFiles"
		file.Error = "startup file summary exists but Store-backed file content is missing"
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

func (b *projectionBuilder) generatedFile(path string, kind string, required bool) ProjectionFile {
	source := strings.TrimSpace(b.sourceByPath[cleanPath(path)])
	if source == "" {
		source = "compose.generatedFiles"
	}
	file := ProjectionFile{
		Path:           path,
		Kind:           kind,
		Source:         source,
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
			Target:        "environment_files",
			Missing:       dedupeSortedProjectionTargets(summaryStartup),
			Action:        "store the referenced startup file content in environment_files; summary.startupFiles is only a repair hint",
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
			Action:        "store every referenced Compose env/config/secret/include/extends file in environment_files, component assets, or environment package projection",
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

func projectionSourceByPath(sources []ProjectionSource) map[string]string {
	out := map[string]string{}
	for _, item := range sources {
		path := cleanPath(item.Path)
		source := strings.TrimSpace(item.Source)
		if path == "" || source == "" {
			continue
		}
		out[path] = source
	}
	return out
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
