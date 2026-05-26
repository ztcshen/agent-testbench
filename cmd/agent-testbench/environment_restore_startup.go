package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func environmentRestoreEffectiveHealthChecks(checks []any, compose map[string]any, graph store.EnvironmentComponentGraph) []any {
	set := environmentRestoreHealthCheckSet{
		covered: map[string]bool{},
		seen:    map[string]bool{},
	}
	startedServices := environmentRestoreStartedServices(compose)
	hasServiceAllowList := len(startedServices) > 0
	for _, raw := range checks {
		set.add(raw)
	}
	for _, component := range graph.Components {
		if !environmentRestoreShouldAddComponentHealth(component, startedServices, hasServiceAllowList) {
			continue
		}
		item, errText := environmentRestoreNormalizeComponentHealthCheck(component)
		if errText == "" {
			set.add(item)
		}
	}
	for _, service := range stringSliceFromAny(compose["services"]) {
		if set.covered[service] {
			continue
		}
		set.out = append(set.out, map[string]any{
			"id":      "compose-service-" + safeReportID(service),
			"kind":    "compose-service",
			"service": service,
		})
		set.covered[service] = true
	}
	return set.out
}

type environmentRestoreHealthCheckSet struct {
	out     []any
	covered map[string]bool
	seen    map[string]bool
}

func (s *environmentRestoreHealthCheckSet) add(raw any) {
	item, ok := raw.(map[string]any)
	if !ok {
		s.out = append(s.out, raw)
		return
	}
	if signature := environmentRestoreHealthCheckSignature(item); signature != "" {
		if s.seen[signature] {
			return
		}
		s.seen[signature] = true
	}
	if environmentRestoreHealthCheckCoversService(item) {
		if service := strings.TrimSpace(valueString(item["service"])); service != "" {
			s.covered[service] = true
		}
	}
	s.out = append(s.out, raw)
}

func environmentRestoreStartedServices(compose map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, service := range stringSliceFromAny(compose["services"]) {
		if service = strings.TrimSpace(service); service != "" {
			out[service] = true
		}
	}
	return out
}

func environmentRestoreShouldAddComponentHealth(component store.EnvironmentComponent, startedServices map[string]bool, hasServiceAllowList bool) bool {
	service := strings.TrimSpace(component.ComposeService)
	return !hasServiceAllowList || service == "" || startedServices[service]
}

func environmentRestoreHealthCheckCoversService(item map[string]any) bool {
	kind := strings.TrimSpace(valueString(item["kind"]))
	if kind == "" {
		kind = strings.TrimSpace(valueString(item["type"]))
	}
	return kind == "compose-service" || kind == "url"
}

func environmentRestoreNormalizeComponentHealthCheck(component store.EnvironmentComponent) (map[string]any, string) {
	raw := strings.TrimSpace(component.HealthCheckJSON)
	normalized, errText := environmentRestoreDecodeHealthCheck(raw)
	if errText != "" {
		return nil, errText
	}
	environmentRestoreApplyComponentHealthDefaults(normalized, component)
	kind := environmentRestoreHealthCheckKind(normalized)
	normalized["kind"] = kind
	if environmentRestoreComponentRequiresURLHealth(component) && kind != "url" {
		return nil, strings.TrimSpace(component.Role) + " health check requires url"
	}
	if errText := environmentRestoreValidateHealthCheckKind(normalized, kind, component); errText != "" {
		return nil, errText
	}
	return normalized, ""
}

func environmentRestoreDecodeHealthCheck(raw string) (map[string]any, string) {
	if raw == "" || raw == "{}" {
		return nil, "missing health check"
	}
	var item map[string]any
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		return nil, "invalid health check JSON: " + err.Error()
	}
	if len(item) == 0 {
		return nil, "missing health check"
	}
	normalized := map[string]any{}
	for key, value := range item {
		normalized[key] = value
	}
	return normalized, ""
}

func environmentRestoreApplyComponentHealthDefaults(normalized map[string]any, component store.EnvironmentComponent) {
	componentID := strings.TrimSpace(component.ComponentID)
	if strings.TrimSpace(valueString(normalized["id"])) == "" && componentID != "" {
		normalized["id"] = "component-" + safeReportID(componentID)
	}
	if componentID != "" {
		normalized["componentId"] = componentID
	}
	if strings.TrimSpace(valueString(normalized["service"])) == "" && strings.TrimSpace(component.ComposeService) != "" {
		normalized["service"] = strings.TrimSpace(component.ComposeService)
	}
}

func environmentRestoreHealthCheckKind(normalized map[string]any) string {
	kind := strings.TrimSpace(valueString(normalized["kind"]))
	if kind == "" {
		kind = strings.TrimSpace(valueString(normalized["type"]))
	}
	if kind == "" && strings.TrimSpace(valueString(normalized["url"])) != "" {
		return "url"
	}
	return kind
}

func environmentRestoreValidateHealthCheckKind(normalized map[string]any, kind string, component store.EnvironmentComponent) string {
	switch kind {
	case "url":
		if strings.TrimSpace(valueString(normalized["url"])) == "" {
			return "url health check requires url"
		}
	case "tcp":
		if strings.TrimSpace(valueString(normalized["address"])) == "" {
			return "tcp health check requires address"
		}
	case "command":
		if strings.TrimSpace(valueString(normalized["command"])) == "" {
			return "command health check requires command"
		}
	case "compose-service":
		if strings.TrimSpace(valueString(normalized["service"])) == "" {
			normalized["service"] = strings.TrimSpace(component.ComposeService)
		}
		if strings.TrimSpace(valueString(normalized["service"])) == "" {
			return "compose-service health check requires service"
		}
	case "container":
		if strings.TrimSpace(valueString(normalized["container"])) == "" {
			return "container health check requires container"
		}
	default:
		if kind == "" {
			return "health check requires kind"
		}
		return "unsupported health check kind: " + kind
	}
	return ""
}

func environmentRestoreComponentRequiresURLHealth(component store.EnvironmentComponent) bool {
	role := strings.TrimSpace(strings.ToLower(component.Role))
	kind := strings.TrimSpace(strings.ToLower(component.Kind))
	return role == "business-service" || kind == "app"
}

func environmentRestoreHealthCheckSignature(item map[string]any) string {
	kind := strings.TrimSpace(valueString(item["kind"]))
	if kind == "" {
		kind = strings.TrimSpace(valueString(item["type"]))
	}
	switch kind {
	case "url":
		return "url:" + strings.TrimSpace(valueString(item["url"]))
	case "tcp":
		return "tcp:" + strings.TrimSpace(valueString(item["address"]))
	case "command":
		return "command:" + strings.TrimSpace(valueString(item["command"]))
	case "compose-service":
		return "compose-service:" + strings.TrimSpace(valueString(item["service"]))
	case "container":
		return "container:" + strings.TrimSpace(valueString(item["container"]))
	default:
		return ""
	}
}

func environmentRestoreApplyEdgeAssets(ctx context.Context, graph store.EnvironmentComponentGraph, compose map[string]any, workspace string, execute bool, composeBaseArgs []string) []environmentRestoreAppliedAsset {
	if len(graph.Dependencies) == 0 || len(graph.Assets) == 0 {
		return nil
	}
	assetsByID := map[string]store.ComponentConfigAsset{}
	for _, asset := range graph.Assets {
		if id := strings.TrimSpace(asset.AssetID); id != "" {
			assetsByID[id] = asset
		}
	}
	componentByID := map[string]store.EnvironmentComponent{}
	for _, component := range graph.Components {
		if id := strings.TrimSpace(component.ComponentID); id != "" {
			componentByID[id] = component
		}
	}
	generated := stringMapFromAny(compose["generatedFiles"])
	out := []environmentRestoreAppliedAsset{}
	appliedAssetTargets := map[string]bool{}
	for _, dep := range graph.Dependencies {
		for _, assetID := range environmentRestoreDependencyAssetIDs(dep) {
			asset, ok := assetsByID[assetID]
			if !ok {
				out = append(out, environmentRestoreAppliedAsset{
					AssetID:            assetID,
					DependencyConsumer: dep.ConsumerComponentID,
					DependencyProvider: dep.ProviderComponentID,
					Action:             "missing-edge-asset",
					OK:                 false,
					Error:              "component dependency references missing config asset: " + assetID,
				})
				continue
			}
			targetComponentID := firstNonEmpty(strings.TrimSpace(asset.TargetComponentID), strings.TrimSpace(dep.ProviderComponentID))
			dedupeKey := assetID + "\x00" + targetComponentID
			if appliedAssetTargets[dedupeKey] {
				continue
			}
			appliedAssetTargets[dedupeKey] = true
			item := environmentRestoreApplyEdgeAsset(ctx, dep, asset, componentByID, generated, workspace, execute, composeBaseArgs)
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DependencyProvider != out[j].DependencyProvider {
			return out[i].DependencyProvider < out[j].DependencyProvider
		}
		if out[i].DependencyConsumer != out[j].DependencyConsumer {
			return out[i].DependencyConsumer < out[j].DependencyConsumer
		}
		if out[i].ApplyOrder != out[j].ApplyOrder {
			return out[i].ApplyOrder < out[j].ApplyOrder
		}
		return out[i].AssetID < out[j].AssetID
	})
	return out
}

func environmentRestoreApplyEdgeAsset(ctx context.Context, dep store.ComponentDependency, asset store.ComponentConfigAsset, components map[string]store.EnvironmentComponent, generated map[string]string, workspace string, execute bool, composeBaseArgs []string) environmentRestoreAppliedAsset {
	targetComponentID := firstNonEmpty(strings.TrimSpace(asset.TargetComponentID), strings.TrimSpace(dep.ProviderComponentID))
	targetService := environmentRestoreComponentComposeService(components[targetComponentID], targetComponentID)
	content, contentErr := environmentRestoreEdgeAssetContent(asset, workspace)
	item := environmentRestoreAppliedAsset{
		AssetID:              strings.TrimSpace(asset.AssetID),
		OwnerComponentID:     strings.TrimSpace(asset.OwnerComponentID),
		TargetComponentID:    targetComponentID,
		TargetComposeService: targetService,
		DependencyConsumer:   strings.TrimSpace(dep.ConsumerComponentID),
		DependencyProvider:   strings.TrimSpace(dep.ProviderComponentID),
		TargetPath:           strings.TrimSpace(asset.TargetPath),
		Bytes:                len(content),
		ApplyOrder:           asset.ApplyOrder,
		Action:               "plan-apply-edge-asset",
		OK:                   true,
	}
	if targetComponentID == "" {
		item.OK = false
		item.Error = "edge asset target component is required"
		return item
	}
	if environmentRestoreIsMySQLSQLAsset(asset, dep) {
		item.Action = "plan-apply-mysql-sql"
		item.Command = environmentRestoreMySQLApplyCommand(composeBaseArgs, targetService)
		if len(composeBaseArgs) == 0 || targetService == "" {
			item.OK = false
			item.Error = "mysql edge asset requires a Docker Compose target service"
			return item
		}
		if contentErr != nil {
			item.OK = false
			item.Error = contentErr.Error()
			return item
		}
		if strings.TrimSpace(content) == "" {
			item.OK = false
			item.Error = "mysql edge asset requires SQL content"
			return item
		}
		if execute {
			item.Action = "apply-mysql-sql"
			attempts, errText := runRestoreMySQLCommandWithInputRetry(ctx, workspace, item.Command, content)
			item.Attempts = attempts
			if errText != "" {
				item.OK = false
				item.Error = errText
			}
		}
		return item
	}
	targetPath := filepath.Clean(strings.TrimSpace(asset.TargetPath))
	if targetPath == "." || targetPath == "" {
		item.OK = false
		item.Error = "edge asset target path is required"
		return item
	}
	if _, ok := generated[targetPath]; ok {
		item.Action = "project-generated-file"
		if execute {
			item.Action = "verify-generated-file"
			if _, err := os.Stat(restoreWorkspacePath(workspace, targetPath)); err != nil {
				item.OK = false
				item.Error = err.Error()
			}
		}
		return item
	}
	item.OK = false
	item.Error = "edge asset must be generated from Store before target startup: " + targetPath
	return item
}

func environmentRestoreEdgeAssetContent(asset store.ComponentConfigAsset, workspace string) (string, error) {
	if strings.TrimSpace(asset.ContentInline) != "" {
		return asset.ContentInline, nil
	}
	targetPath := filepath.Clean(strings.TrimSpace(asset.TargetPath))
	if targetPath == "." || targetPath == "" || targetPath == ".." || filepath.IsAbs(targetPath) || strings.HasPrefix(targetPath, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("edge asset target path is required")
	}
	raw, err := os.ReadFile(restoreWorkspacePath(workspace, targetPath))
	if err != nil {
		return "", fmt.Errorf("read edge asset content from %s: %w", targetPath, err)
	}
	return string(raw), nil
}

func environmentRestoreDependencyAssetIDs(dep store.ComponentDependency) []string {
	profile := jsonObjectString(dep.ProfileJSON)
	ids := []string{}
	for _, key := range []string{"assetIds", "configAssetIds", "startupAssetIds", "applyAssetIds"} {
		ids = append(ids, stringSliceFromAny(profile[key])...)
	}
	if value := strings.TrimSpace(valueString(profile["assetId"])); value != "" {
		ids = append(ids, value)
	}
	return dedupeStrings(ids)
}

func environmentRestoreIsMySQLSQLAsset(asset store.ComponentConfigAsset, dep store.ComponentDependency) bool {
	kind := strings.ToLower(strings.TrimSpace(asset.AssetKind))
	capability := strings.ToLower(strings.TrimSpace(dep.Capability))
	if kind == "" {
		return false
	}
	tokens := strings.FieldsFunc(kind, func(r rune) bool {
		return r < 'a' || r > 'z'
	})
	hasSQLToken := false
	hasMySQLToken := false
	for _, token := range tokens {
		switch token {
		case "ddl", "schema", "seed", "sql":
			hasSQLToken = true
		case "mysql":
			hasMySQLToken = true
		}
	}
	if !hasSQLToken {
		return false
	}
	if hasMySQLToken {
		return true
	}
	return capability == "sql" && (environmentRestoreHasMySQLComponentSignal(asset.TargetComponentID) || environmentRestoreHasMySQLComponentSignal(dep.ProviderComponentID))
}

func environmentRestoreHasMySQLComponentSignal(componentID string) bool {
	tokens := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(componentID)), func(r rune) bool {
		return r < 'a' || r > 'z'
	})
	for _, token := range tokens {
		if token == "mysql" {
			return true
		}
	}
	return false
}

func environmentRestoreComponentComposeService(component store.EnvironmentComponent, defaultID string) string {
	if service := strings.TrimSpace(component.ComposeService); service != "" {
		return service
	}
	return strings.TrimSpace(defaultID)
}

func environmentRestoreMySQLApplyCommand(composeBaseArgs []string, service string) []string {
	command := append([]string{"docker", "compose"}, composeBaseArgs...)
	command = append(command, "exec", "-T", service, "sh", "-lc", environmentRestoreMySQLClientScript())
	return command
}

func environmentRestoreMySQLClientScript() string {
	return `user="${MYSQL_USER:-root}"
password="${MYSQL_PASSWORD:-${MYSQL_ROOT_PASSWORD:-}}"
database="${AGENT_TESTBENCH_MYSQL_APPLY_DATABASE:-}"
set --
if [ -n "$user" ]; then
  set -- "$@" "-u${user}"
fi
if [ -n "$password" ]; then
  set -- "$@" "-p${password}"
fi
if [ -n "$database" ]; then
  set -- "$@" "${database}"
fi
exec mysql "$@"`
}

func runRestoreMySQLCommandWithInputRetry(ctx context.Context, workdir string, command []string, input string) (int, string) {
	const maxAttempts = 60
	const delay = time.Second
	var lastErr string
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		_, errText := runRestoreCommandWithInput(ctx, workdir, command, input)
		if errText == "" {
			return attempt, ""
		}
		lastErr = errText
		if !environmentRestoreMySQLApplyErrCanRetry(errText) {
			return attempt, errText
		}
		if attempt == maxAttempts {
			break
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return attempt, ctx.Err().Error()
		case <-timer.C:
		}
	}
	return maxAttempts, lastErr
}

func environmentRestoreMySQLApplyErrCanRetry(errText string) bool {
	lower := strings.ToLower(errText)
	retryable := []string{
		"access denied for user 'root'@'localhost'",
		"can't connect to local mysql server",
		"can't connect to mysql server",
		"lost connection to mysql server",
		"server has gone away",
		"error 1045",
		"error 2002",
		"error 2003",
		"error 2013",
	}
	for _, item := range retryable {
		if strings.Contains(lower, item) {
			return true
		}
	}
	return false
}

func environmentRestoreIsRemoteGitURL(rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	lower := strings.ToLower(rawURL)
	for _, prefix := range []string{"https://", "http://", "ssh://", "git://"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	at := strings.Index(rawURL, "@")
	colon := strings.Index(rawURL, ":")
	return at > 0 && colon > at+1
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
		repoCheckouts[filepath.Clean(spec.Checkout)] = true
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
	parts := strings.Split(value, ":")
	if len(parts) < 2 {
		return "", "", false
	}
	source := strings.Trim(parts[0], `"' `)
	target := strings.Trim(parts[1], `"' `)
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
	values := stringMapFromAny(compose["env"])
	expanded := strings.TrimSpace(source)
	for key, value := range values {
		value = strings.ReplaceAll(value, "$AGENT_TESTBENCH_WORKSPACE", workspace)
		expanded = strings.ReplaceAll(expanded, "${"+key+"}", value)
		expanded = strings.ReplaceAll(expanded, "$"+key, value)
		for {
			start := strings.Index(expanded, "${"+key+":-")
			if start < 0 {
				break
			}
			end := strings.Index(expanded[start:], "}")
			if end < 0 {
				break
			}
			end += start
			expanded = expanded[:start] + value + expanded[end+1:]
		}
	}
	expanded = strings.ReplaceAll(expanded, "$AGENT_TESTBENCH_WORKSPACE", workspace)
	expanded = strings.ReplaceAll(expanded, "${AGENT_TESTBENCH_WORKSPACE}", workspace)
	return expanded
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
