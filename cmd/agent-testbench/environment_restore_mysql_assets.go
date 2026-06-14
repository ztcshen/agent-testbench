package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-testbench/internal/store"

	"gopkg.in/yaml.v3"
)

const (
	environmentRestoreActionApplyMySQLSQL      = "apply-mysql-sql"
	environmentRestoreActionProjectMySQLInitDB = "project-mysql-initdb"
)

func environmentRestoreApplyMySQLSQLEdgeAsset(ctx context.Context, content string, contentErr error, compose map[string]any, workspace string, execute bool, composeBaseArgs []string, options environmentRestoreApplyAssetOptions, item environmentRestoreAppliedAsset) environmentRestoreAppliedAsset {
	item.Action = environmentRestoreActionProjectMySQLInitDB
	item.Command = nil
	if len(composeBaseArgs) == 0 || item.TargetComposeService == "" {
		item.OK = false
		item.Error = "mysql edge asset requires a Docker Compose target service"
		return item
	}
	if execute && options.UseExistingContainers {
		item.Action = "skip-mysql-sql-use-existing-containers"
		item.Command = nil
		item.Status = "skipped"
		item.Error = "plain MySQL SQL bootstrap asset was not re-applied to existing containers; convert it to an environment migration asset or rerun restore with a clean Docker state when it must be applied"
		return item
	}
	targetPathOK := false
	if strings.TrimSpace(item.TargetPath) != "" {
		if ok, errText := environmentRestoreGeneratedFileTargetOK(item.TargetPath, workspace); !ok {
			item.OK = false
			item.Error = errText
			return item
		}
		targetPathOK = true
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
	mounted := targetPathOK && environmentRestoreMySQLInitDBMountsTarget(compose, workspace, item.TargetComposeService, item.TargetPath)
	if !mounted {
		item.Action = environmentRestoreActionApplyMySQLSQL
		item.Command = environmentRestoreMySQLApplyCommand(composeBaseArgs, item.TargetComposeService)
		if execute {
			attempts, errText := runRestoreMySQLCommandWithInputRetry(ctx, workspace, item.Command, content)
			item.Attempts = attempts
			if errText != "" {
				item.OK = false
				item.Error = errText
			} else {
				item.Status = "applied"
			}
		}
		return item
	}
	if execute {
		projected, err := os.ReadFile(restoreWorkspacePath(workspace, filepath.Clean(item.TargetPath)))
		if err != nil {
			item.OK = false
			item.Error = err.Error()
			return item
		}
		if string(projected) != content {
			item.OK = false
			item.Error = "projected mysql initdb SQL does not match Store asset content"
		}
	}
	return item
}

func environmentRestoreMySQLInitDBMountsTarget(compose map[string]any, workspace string, service string, targetPath string) bool {
	service = strings.TrimSpace(service)
	cleanTarget := filepath.Clean(strings.TrimSpace(targetPath))
	if service == "" || cleanTarget == "." || cleanTarget == "" {
		return false
	}
	for _, composeFile := range environmentRestoreComposeFiles(compose) {
		cleanCompose := filepath.Clean(composeFile)
		content := generatedFileContentMapFromAny(compose["generatedFiles"])[cleanCompose]
		if content == "" {
			raw, err := os.ReadFile(restoreWorkspacePath(workspace, cleanCompose))
			if err == nil {
				content = string(raw)
			}
		}
		if content == "" {
			continue
		}
		if environmentRestoreComposeContentMountsMySQLInitDB(content, filepath.Dir(cleanCompose), compose, workspace, service, cleanTarget) {
			return true
		}
	}
	return false
}

func environmentRestoreComposeContentMountsMySQLInitDB(content string, composeDir string, compose map[string]any, workspace string, service string, targetPath string) bool {
	if ok, parsed := environmentRestoreComposeYAMLMountsMySQLInitDB(content, composeDir, compose, workspace, service, targetPath); parsed {
		return ok
	}
	scanner := environmentRestoreComposeMountScanner{
		composeDir: composeDir,
		compose:    compose,
		workspace:  workspace,
		service:    service,
		targetPath: targetPath,
	}
	for _, line := range strings.Split(content, "\n") {
		if scanner.processLine(line) {
			return true
		}
	}
	return scanner.flushVolume()
}

func environmentRestoreComposeYAMLMountsMySQLInitDB(content string, composeDir string, compose map[string]any, workspace string, service string, targetPath string) (bool, bool) {
	var doc struct {
		Services map[string]struct {
			Volumes []any `yaml:"volumes"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return false, false
	}
	serviceDef, ok := doc.Services[service]
	if !ok {
		return false, true
	}
	for _, volume := range serviceDef.Volumes {
		if environmentRestoreComposeVolumeMountsMySQLInitDB(volume, composeDir, compose, workspace, targetPath) {
			return true, true
		}
	}
	return false, true
}

func environmentRestoreComposeVolumeMountsMySQLInitDB(volume any, composeDir string, compose map[string]any, workspace string, targetPath string) bool {
	switch value := volume.(type) {
	case string:
		source, target, ok := parseComposeShortVolume(value)
		return ok && environmentRestoreVolumeSourceMountsMySQLInitDB(source, target, composeDir, compose, workspace, targetPath)
	case map[string]any:
		source := valueString(firstNonNil(value["source"], value["src"]))
		target := valueString(firstNonNil(value["target"], value["dst"], value["destination"]))
		return environmentRestoreVolumeSourceMountsMySQLInitDB(source, target, composeDir, compose, workspace, targetPath)
	default:
		return false
	}
}

func environmentRestoreVolumeSourceMountsMySQLInitDB(source string, target string, composeDir string, compose map[string]any, workspace string, targetPath string) bool {
	if !environmentRestoreIsMySQLInitDBTarget(target) {
		return false
	}
	sourcePath, sourceOK := environmentRestoreStartupAssetPath(source, composeDir, compose, workspace)
	return sourceOK && environmentRestoreMountSourceCoversTarget(sourcePath, targetPath)
}

type environmentRestoreComposeMountScanner struct {
	composeDir     string
	compose        map[string]any
	workspace      string
	service        string
	targetPath     string
	currentService string
	inServices     bool
	volume         environmentRestoreComposeVolumeCandidate
}

func (scanner *environmentRestoreComposeMountScanner) processLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent == 0 {
		return scanner.processRootLine(trimmed)
	}
	if scanner.isServiceHeader(indent, trimmed) {
		return scanner.processServiceHeader(trimmed)
	}
	if !scanner.inTargetService() {
		return false
	}
	return scanner.processTargetServiceLine(indent, trimmed)
}

func (scanner *environmentRestoreComposeMountScanner) processRootLine(trimmed string) bool {
	if scanner.flushVolume() {
		return true
	}
	scanner.inServices = trimmed == composeServicesHeader
	scanner.currentService = ""
	return false
}

func (scanner *environmentRestoreComposeMountScanner) isServiceHeader(indent int, trimmed string) bool {
	return scanner.inServices && indent == 2 && strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "-")
}

func (scanner *environmentRestoreComposeMountScanner) processServiceHeader(trimmed string) bool {
	if scanner.flushVolume() {
		return true
	}
	scanner.currentService = strings.TrimSuffix(trimmed, ":")
	return false
}

func (scanner *environmentRestoreComposeMountScanner) inTargetService() bool {
	return scanner.inServices && scanner.currentService == scanner.service
}

func (scanner *environmentRestoreComposeMountScanner) processTargetServiceLine(indent int, trimmed string) bool {
	if scanner.volume.active && indent <= scanner.volume.indent && !strings.HasPrefix(trimmed, "-") && scanner.flushVolume() {
		return true
	}
	if !strings.HasPrefix(trimmed, "-") {
		if scanner.volume.active && indent > scanner.volume.indent {
			scanner.volume.applyKeyValue(trimmed)
		}
		return false
	}
	if scanner.flushVolume() {
		return true
	}
	return scanner.processVolumeItem(indent, strings.TrimSpace(strings.TrimPrefix(trimmed, "-")))
}

func (scanner *environmentRestoreComposeMountScanner) processVolumeItem(indent int, entry string) bool {
	source, target, ok := parseComposeShortVolume(entry)
	if !ok || !environmentRestoreIsMySQLInitDBTarget(target) {
		scanner.volume = newEnvironmentRestoreComposeVolumeCandidate(indent, entry)
		return false
	}
	sourcePath, sourceOK := environmentRestoreStartupAssetPath(source, scanner.composeDir, scanner.compose, scanner.workspace)
	return sourceOK && environmentRestoreMountSourceCoversTarget(sourcePath, scanner.targetPath)
}

func (scanner *environmentRestoreComposeMountScanner) flushVolume() bool {
	defer func() {
		scanner.volume = environmentRestoreComposeVolumeCandidate{}
	}()
	if !environmentRestoreIsMySQLInitDBTarget(scanner.volume.target) {
		return false
	}
	sourcePath, sourceOK := environmentRestoreStartupAssetPath(scanner.volume.source, scanner.composeDir, scanner.compose, scanner.workspace)
	return sourceOK && environmentRestoreMountSourceCoversTarget(sourcePath, scanner.targetPath)
}

type environmentRestoreComposeVolumeCandidate struct {
	active bool
	indent int
	source string
	target string
}

func newEnvironmentRestoreComposeVolumeCandidate(indent int, entry string) environmentRestoreComposeVolumeCandidate {
	volume := environmentRestoreComposeVolumeCandidate{active: true, indent: indent}
	volume.applyKeyValue(entry)
	return volume
}

func (volume *environmentRestoreComposeVolumeCandidate) applyKeyValue(line string) {
	key, value, ok := strings.Cut(line, ":")
	if !ok {
		return
	}
	value = strings.Trim(strings.TrimSpace(value), `"'`)
	switch strings.TrimSpace(key) {
	case "source":
		volume.source = value
	case "target":
		volume.target = value
	}
}

func environmentRestoreIsMySQLInitDBTarget(target string) bool {
	target = filepath.ToSlash(strings.TrimSpace(target))
	return target == "/docker-entrypoint-initdb.d" || strings.HasPrefix(target, "/docker-entrypoint-initdb.d/")
}

func environmentRestoreMountSourceCoversTarget(source string, targetPath string) bool {
	source = filepath.Clean(strings.TrimSpace(source))
	targetPath = filepath.Clean(strings.TrimSpace(targetPath))
	return source == targetPath || strings.HasPrefix(targetPath, source+string(filepath.Separator))
}

func environmentRestoreProjectDockerNativeAssets(report *environmentRestoreDockerReport, graph store.EnvironmentComponentGraph, compose map[string]any, workspace string, execute bool) bool {
	if !execute {
		return true
	}
	failures := environmentRestoreProjectMySQLInitDBAssets(graph, compose, generatedFileContentMapFromAny(compose["generatedFiles"]), workspace)
	for _, item := range failures {
		report.AppliedAssets = append(report.AppliedAssets, item)
		if !item.OK {
			report.OK = false
			report.Action = environmentRestoreActionProjectMySQLInitDB
			report.Error = item.Error
			return false
		}
	}
	return true
}

func environmentRestoreProjectMySQLInitDBAssets(graph store.EnvironmentComponentGraph, compose map[string]any, generated map[string]string, workspace string) []environmentRestoreAppliedAsset {
	if len(graph.Dependencies) == 0 || len(graph.Assets) == 0 {
		return nil
	}
	componentByID := environmentRestoreComponentMap(graph.Components)
	out := []environmentRestoreAppliedAsset{}
	targetAssetIDs := map[string]string{}
	for _, ref := range environmentRestoreDependencyAssetRefs(graph) {
		if !ref.Found || environmentMigrationIsAsset(ref.Asset) || !environmentRestoreIsMySQLSQLAsset(ref.Asset, ref.Dependency) {
			continue
		}
		item := environmentRestoreMySQLInitDBProjectionItem(ref.Dependency, ref.Asset, componentByID[ref.TargetComponentID], ref.TargetComponentID)
		cleanTarget := filepath.Clean(strings.TrimSpace(item.TargetPath))
		if previous := targetAssetIDs[cleanTarget]; previous != "" && previous != item.AssetID {
			item.OK = false
			item.Error = "mysql initdb target path is shared by multiple Store assets: " + cleanTarget + " (" + previous + ", " + item.AssetID + ")"
			out = append(out, item)
			continue
		}
		targetAssetIDs[cleanTarget] = item.AssetID
		if !environmentRestoreMySQLInitDBMountsTarget(compose, workspace, item.TargetComposeService, cleanTarget) {
			item.OK = false
			item.Error = "mysql initdb target path is not mounted into service " + item.TargetComposeService + ": " + cleanTarget
			out = append(out, item)
			continue
		}
		if existing, ok := generated[cleanTarget]; ok {
			content, contentErr := environmentRestoreEdgeAssetContent(ref.Asset, workspace)
			if contentErr == nil && strings.TrimSpace(existing) != strings.TrimSpace(content) {
				item.OK = false
				item.Error = "mysql initdb target path conflicts with generated Store file: " + cleanTarget
				out = append(out, item)
				continue
			}
		}
		if !environmentRestoreProjectMySQLInitDBAsset(ref.Asset, workspace, &item) {
			out = append(out, item)
		}
	}
	return out
}

func environmentRestoreMySQLInitDBProjectionItem(dep store.ComponentDependency, asset store.ComponentConfigAsset, target store.EnvironmentComponent, targetComponentID string) environmentRestoreAppliedAsset {
	return environmentRestoreAppliedAsset{
		AssetID:              strings.TrimSpace(asset.AssetID),
		OwnerComponentID:     strings.TrimSpace(asset.OwnerComponentID),
		TargetComponentID:    targetComponentID,
		TargetComposeService: environmentRestoreComponentComposeService(target, targetComponentID),
		DependencyConsumer:   strings.TrimSpace(dep.ConsumerComponentID),
		DependencyProvider:   strings.TrimSpace(dep.ProviderComponentID),
		TargetPath:           strings.TrimSpace(asset.TargetPath),
		ApplyOrder:           asset.ApplyOrder,
		Action:               environmentRestoreActionProjectMySQLInitDB,
		OK:                   true,
	}
}

func environmentRestoreProjectMySQLInitDBAsset(asset store.ComponentConfigAsset, workspace string, item *environmentRestoreAppliedAsset) bool {
	if ok, errText := environmentRestoreGeneratedFileTargetOK(item.TargetPath, workspace); !ok {
		item.OK = false
		item.Error = errText
		return false
	}
	content, contentErr := environmentRestoreEdgeAssetContent(asset, workspace)
	item.Bytes = len(content)
	if contentErr != nil {
		item.OK = false
		item.Error = contentErr.Error()
		return false
	}
	if strings.TrimSpace(content) == "" {
		item.OK = false
		item.Error = "mysql edge asset requires SQL content"
		return false
	}
	target := restoreWorkspacePath(workspace, filepath.Clean(item.TargetPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		item.OK = false
		item.Error = err.Error()
		return false
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		item.OK = false
		item.Error = err.Error()
		return false
	}
	return true
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
		case "ddl", "schema", environmentRestoreAssetTokenSeed, "sql":
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
