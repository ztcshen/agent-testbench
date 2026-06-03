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

const (
	objectStorageCapability = "object-storage"
	actionSeedObjectStorage = "seed-object-storage"
)

type environmentRestoreComponentAsset struct {
	AssetID          string   `json:"assetId"`
	OwnerComponentID string   `json:"ownerComponentId,omitempty"`
	SourceURL        string   `json:"sourceUrl,omitempty"`
	SourcePath       string   `json:"sourcePath,omitempty"`
	Checkout         string   `json:"checkout,omitempty"`
	TargetPath       string   `json:"targetPath"`
	Bytes            int64    `json:"bytes,omitempty"`
	ApplyOrder       int      `json:"applyOrder,omitempty"`
	Action           string   `json:"action"`
	RepoAction       string   `json:"repoAction,omitempty"`
	Command          []string `json:"command,omitempty"`
	OK               bool     `json:"ok"`
	Error            string   `json:"error,omitempty"`
}

type environmentRestoreAppliedAsset struct {
	AssetID              string   `json:"assetId"`
	OwnerComponentID     string   `json:"ownerComponentId,omitempty"`
	TargetComponentID    string   `json:"targetComponentId,omitempty"`
	TargetComposeService string   `json:"targetComposeService,omitempty"`
	DependencyConsumer   string   `json:"dependencyConsumer,omitempty"`
	DependencyProvider   string   `json:"dependencyProvider,omitempty"`
	TargetPath           string   `json:"targetPath,omitempty"`
	Bytes                int      `json:"bytes,omitempty"`
	ApplyOrder           int      `json:"applyOrder,omitempty"`
	Action               string   `json:"action"`
	Command              []string `json:"command,omitempty"`
	Attempts             int      `json:"attempts,omitempty"`
	Status               string   `json:"status,omitempty"`
	OK                   bool     `json:"ok"`
	Error                string   `json:"error,omitempty"`
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
	if environmentMigrationIsAsset(asset) {
		return environmentRestoreApplyMigrationEdgeAsset(ctx, dep, asset, content, contentErr, workspace, execute, composeBaseArgs, item)
	}
	if environmentRestoreIsObjectStorageAsset(asset, dep) {
		return environmentRestoreApplyObjectStorageEdgeAsset(ctx, dep, asset, components[targetComponentID], content, contentErr, workspace, execute, item)
	}
	if environmentRestoreIsMySQLSQLAsset(asset, dep) {
		return environmentRestoreApplyMySQLSQLEdgeAsset(ctx, content, contentErr, workspace, execute, composeBaseArgs, item)
	}
	return environmentRestoreApplyGeneratedEdgeAsset(asset, generated, workspace, execute, item)
}

func environmentRestoreApplyObjectStorageEdgeAsset(ctx context.Context, dep store.ComponentDependency, asset store.ComponentConfigAsset, provider store.EnvironmentComponent, content string, contentErr error, workspace string, execute bool, item environmentRestoreAppliedAsset) environmentRestoreAppliedAsset {
	bucket, key := environmentRestoreObjectStorageAssetLocation(asset)
	content, contentErr = environmentRestoreObjectStorageAssetContent(asset, content, contentErr)
	item.TargetPath = environmentRestoreObjectStorageTargetPath(bucket, key)
	item.Action = "plan-seed-object-storage"
	if bucket == "" || key == "" {
		item.OK = false
		item.Error = "object storage asset requires bucket and key metadata"
		return item
	}
	if contentErr != nil {
		item.OK = false
		item.Error = contentErr.Error()
		return item
	}
	command := environmentRestoreObjectStorageSeedCommand(provider, map[string]string{
		"assetId":         item.AssetID,
		"bucket":          bucket,
		"key":             key,
		"targetPath":      item.TargetPath,
		"consumer":        dep.ConsumerComponentID,
		"provider":        dep.ProviderComponentID,
		"composeService":  item.TargetComposeService,
		"targetComponent": item.TargetComponentID,
	})
	item.Command = command
	if len(command) == 0 {
		item.OK = false
		item.Error = "object storage asset requires provider objectStorage.seedCommand metadata"
		return item
	}
	if execute {
		item.Action = actionSeedObjectStorage
		attempts, errText := runRestoreObjectStorageSeedCommandWithInputRetry(ctx, workspace, command, content)
		item.Attempts = attempts
		if errText != "" {
			item.OK = false
			item.Error = errText
		}
	}
	return item
}

func environmentRestoreObjectStorageAssetContent(asset store.ComponentConfigAsset, content string, contentErr error) (string, error) {
	if contentErr != nil {
		if environmentRestoreAllowsEmptyObjectStorageContent(asset) {
			return "", nil
		}
		return "", contentErr
	}
	return content, nil
}

func environmentRestoreAllowsEmptyObjectStorageContent(asset store.ComponentConfigAsset) bool {
	if asset.SizeBytes != 0 || strings.TrimSpace(asset.ContentInline) != "" {
		return false
	}
	for _, raw := range []string{asset.SummaryJSON, asset.RemoteRefJSON} {
		meta := jsonObjectString(raw)
		if boolFromReportAny(firstNonNil(meta["empty"], meta["emptyObject"], meta["zeroByte"], meta["zeroBytes"])) {
			return true
		}
		for _, key := range []string{"sizeBytes", "bytes", "size"} {
			if value, ok := meta[key]; ok && environmentRestoreIsNumericZero(value) {
				return true
			}
		}
	}
	return false
}

func environmentRestoreIsNumericZero(value any) bool {
	switch typed := value.(type) {
	case int:
		return typed == 0
	case int64:
		return typed == 0
	case float64:
		return typed == 0
	case json.Number:
		return typed.String() == "0" || typed.String() == "0.0"
	default:
		return false
	}
}

func environmentRestoreObjectStorageAssetLocation(asset store.ComponentConfigAsset) (string, string) {
	if bucket, key := environmentRestoreObjectStorageMetadataLocation(asset.SummaryJSON, true); bucket != "" && key != "" {
		return bucket, key
	}
	if bucket, key := environmentRestoreObjectStorageMetadataLocation(asset.RemoteRefJSON, false); bucket != "" && key != "" {
		return bucket, key
	}
	return environmentRestoreObjectStorageTargetPathLocation(asset.TargetPath)
}

func environmentRestoreObjectStorageMetadataLocation(raw string, allowPathKey bool) (string, string) {
	meta := jsonObjectString(raw)
	bucket := strings.TrimSpace(valueString(firstNonNil(meta["bucket"], meta["bucketName"])))
	keyValues := []any{meta["key"], meta["objectKey"]}
	if allowPathKey {
		keyValues = append(keyValues, meta["path"])
	}
	key := strings.TrimSpace(valueString(firstNonNil(keyValues...)))
	return bucket, key
}

func environmentRestoreObjectStorageTargetPath(bucket string, key string) string {
	if bucket == "" || key == "" {
		return ""
	}
	return strings.Trim(strings.TrimSpace(bucket)+"/"+strings.Trim(strings.TrimSpace(key), "/"), "/")
}

func environmentRestoreObjectStorageSeedCommand(provider store.EnvironmentComponent, values map[string]string) []string {
	for _, raw := range []string{provider.RuntimeJSON, provider.SummaryJSON} {
		config := mapFromReportAny(jsonObjectString(raw)["objectStorage"])
		command := stringSliceFromAny(config["seedCommand"])
		if len(command) == 0 {
			commandText := strings.TrimSpace(valueString(config["seedCommand"]))
			if commandText != "" {
				command = []string{"sh", "-lc", commandText}
			}
		}
		if len(command) > 0 {
			return environmentRestoreReplaceObjectStoragePlaceholders(command, values)
		}
	}
	return nil
}

func environmentRestoreReplaceObjectStoragePlaceholders(command []string, values map[string]string) []string {
	out := make([]string, 0, len(command))
	for _, arg := range command {
		next := arg
		for key, value := range values {
			next = strings.ReplaceAll(next, "{"+key+"}", value)
		}
		out = append(out, next)
	}
	return out
}

func runRestoreObjectStorageSeedCommandWithInputRetry(ctx context.Context, workdir string, command []string, input string) (int, string) {
	const maxAttempts = 60
	const delay = 250 * time.Millisecond
	var lastErr string
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		_, errText := runRestoreCommandWithInput(ctx, workdir, command, input)
		if errText == "" {
			return attempt, ""
		}
		lastErr = errText
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

func environmentRestoreApplyMigrationEdgeAsset(ctx context.Context, dep store.ComponentDependency, asset store.ComponentConfigAsset, content string, contentErr error, workspace string, execute bool, composeBaseArgs []string, item environmentRestoreAppliedAsset) environmentRestoreAppliedAsset {
	item.Action = environmentMigrationActionPlanApplyMySQL
	item.Command = environmentRestoreMySQLApplyCommand(composeBaseArgs, item.TargetComposeService)
	if len(composeBaseArgs) == 0 || item.TargetComposeService == "" {
		item.OK = false
		item.Error = "mysql migration asset requires a Docker Compose target service"
		return item
	}
	migration, ok := environmentMigrationItemFromAsset(asset, item.TargetComponentID)
	if !ok {
		item.OK = false
		item.Error = "mysql migration asset requires migration version and database metadata"
		return item
	}
	migration.EnvironmentID = firstNonEmpty(asset.EnvID, dep.EnvID)
	if contentErr != nil {
		item.OK = false
		item.Error = contentErr.Error()
		return item
	}
	if strings.TrimSpace(content) == "" {
		item.OK = false
		item.Error = "mysql migration asset requires SQL content"
		return item
	}
	migration.Content = content
	if execute {
		item.Action = environmentMigrationActionApplyMySQL
		edge := environmentMigrationEdge{Owner: dep.ConsumerComponentID, Provider: dep.ProviderComponentID}
		attempts, status, errText := runEnvironmentMigrationWithHistory(ctx, workspace, item.Command, edge, migration, environmentMigrationApplySQL(edge, migration), false)
		item.Attempts = attempts
		if errText != "" {
			item.OK = false
			item.Error = errText
		} else {
			item.Status = status
		}
	}
	return item
}

func environmentRestoreApplyMySQLSQLEdgeAsset(ctx context.Context, content string, contentErr error, workspace string, execute bool, composeBaseArgs []string, item environmentRestoreAppliedAsset) environmentRestoreAppliedAsset {
	item.Action = "plan-apply-mysql-sql"
	item.Command = environmentRestoreMySQLApplyCommand(composeBaseArgs, item.TargetComposeService)
	if len(composeBaseArgs) == 0 || item.TargetComposeService == "" {
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

func environmentRestoreApplyGeneratedEdgeAsset(asset store.ComponentConfigAsset, generated map[string]string, workspace string, execute bool, item environmentRestoreAppliedAsset) environmentRestoreAppliedAsset {
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

func environmentRestoreIsObjectStorageAsset(asset store.ComponentConfigAsset, dep store.ComponentDependency) bool {
	kind := strings.ToLower(strings.TrimSpace(asset.AssetKind))
	capability := strings.ToLower(strings.TrimSpace(dep.Capability))
	if environmentRestoreObjectStorageKindIsProviderConfig(kind) {
		return false
	}
	if environmentRestoreObjectStorageKindSignal(kind) {
		return true
	}
	if environmentRestoreIsObjectStorageCapability(capability) {
		return environmentRestoreHasObjectStorageLocationMetadata(asset) || environmentRestoreHasObjectStorageTargetPathLocation(asset)
	}
	return false
}

func environmentRestoreObjectStorageKindSignal(kind string) bool {
	tokens := environmentRestoreObjectStorageKindTokens(kind)
	objectCount := 0
	hasStorage := false
	hasObjectFixture := false
	for _, token := range tokens {
		switch token {
		case "object":
			objectCount++
		case "storage":
			hasStorage = true
		case "s3", "bucket", "fixture", "seed":
			hasObjectFixture = true
		}
	}
	if hasObjectFixture && (objectCount > 0 || hasStorage) {
		return true
	}
	if hasStorage && objectCount >= 2 {
		return true
	}
	return false
}

func environmentRestoreObjectStorageKindIsProviderConfig(kind string) bool {
	hasConfig := false
	hasObjectFixture := false
	objectCount := 0
	for _, token := range environmentRestoreObjectStorageKindTokens(kind) {
		switch token {
		case "config", "credential", "credentials", "env", "secret", "secrets", "setting", "settings", "policy":
			hasConfig = true
		case "s3", "bucket", "fixture", "seed":
			hasObjectFixture = true
		case "object":
			objectCount++
		}
	}
	return hasConfig && !hasObjectFixture && objectCount < 2
}

func environmentRestoreObjectStorageKindTokens(kind string) []string {
	return strings.FieldsFunc(kind, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
}

func environmentRestoreIsObjectStorageCapability(capability string) bool {
	return capability == objectStorageCapability || capability == "object_storage" || capability == "s3"
}

func environmentRestoreHasObjectStorageLocationMetadata(asset store.ComponentConfigAsset) bool {
	if bucket, key := environmentRestoreObjectStorageMetadataLocation(asset.SummaryJSON, true); bucket != "" && key != "" {
		return true
	}
	if bucket, key := environmentRestoreObjectStorageMetadataLocation(asset.RemoteRefJSON, false); bucket != "" && key != "" {
		return true
	}
	return false
}

func environmentRestoreHasObjectStorageTargetPathLocation(asset store.ComponentConfigAsset) bool {
	bucket, key := environmentRestoreObjectStorageTargetPathLocation(asset.TargetPath)
	return bucket != "" && key != ""
}

func environmentRestoreObjectStorageTargetPathLocation(targetPath string) (string, string) {
	targetPath = filepath.Clean(strings.TrimSpace(targetPath))
	if targetPath == "." || targetPath == "" || targetPath == ".." || filepath.IsAbs(targetPath) || strings.HasPrefix(targetPath, ".."+string(os.PathSeparator)) {
		return "", ""
	}
	parts := strings.SplitN(filepath.ToSlash(targetPath), "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
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
