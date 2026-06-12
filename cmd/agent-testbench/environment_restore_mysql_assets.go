package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

const environmentRestoreActionProjectMySQLInitDB = "project-mysql-initdb"

func environmentRestoreApplyMySQLSQLEdgeAsset(content string, contentErr error, compose map[string]any, workspace string, execute bool, composeBaseArgs []string, options environmentRestoreApplyAssetOptions, item environmentRestoreAppliedAsset) environmentRestoreAppliedAsset {
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
	if ok, errText := environmentRestoreGeneratedFileTargetOK(item.TargetPath, workspace); !ok {
		item.OK = false
		item.Error = errText
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
	mounted := environmentRestoreMySQLInitDBMountsTarget(compose, workspace, item.TargetComposeService, item.TargetPath)
	if !mounted {
		item.Action = "apply-mysql-sql"
		item.Command = environmentRestoreMySQLApplyCommand(composeBaseArgs, item.TargetComposeService)
		if execute {
			attempts, errText := runRestoreMySQLCommandWithInputRetry(context.Background(), workspace, item.Command, content)
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
		content := stringMapFromAny(compose["generatedFiles"])[cleanCompose]
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
	currentService := ""
	inServices := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == 0 {
			inServices = trimmed == "services:"
			currentService = ""
			continue
		}
		if inServices && indent == 2 && strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "-") {
			currentService = strings.TrimSuffix(trimmed, ":")
			continue
		}
		if !inServices || currentService != service || !strings.HasPrefix(trimmed, "-") {
			continue
		}
		source, target, ok := parseComposeShortVolume(strings.TrimSpace(strings.TrimPrefix(trimmed, "-")))
		if !ok || !environmentRestoreIsMySQLInitDBTarget(target) {
			continue
		}
		sourcePath, sourceOK := environmentRestoreStartupAssetPath(source, composeDir, compose, workspace)
		if sourceOK && environmentRestoreMountSourceCoversTarget(sourcePath, targetPath) {
			return true
		}
	}
	return false
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
	failures := environmentRestoreProjectMySQLInitDBAssets(graph, compose, stringMapFromAny(compose["generatedFiles"]), workspace)
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
