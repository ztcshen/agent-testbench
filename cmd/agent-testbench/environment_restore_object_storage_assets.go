package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

const (
	objectStorageCapability = "object-storage"
	actionSeedObjectStorage = "seed-object-storage"
)

func environmentRestoreApplyObjectStorageEdgeAsset(ctx context.Context, dep store.ComponentDependency, asset store.ComponentConfigAsset, provider store.EnvironmentComponent, content string, contentErr error, workspace string, execute bool, options environmentRestoreApplyAssetOptions, item environmentRestoreAppliedAsset) environmentRestoreAppliedAsset {
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
		if !options.UseExistingContainers && environmentRestoreObjectStorageSeedSatisfiedByCompose(item.TargetComposeService, options.CompletedServices) {
			item.Action = "object-storage-seed-satisfied-by-compose"
			item.Status = "compose-service-completed"
			return item
		}
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

func environmentRestoreObjectStorageSeedSatisfiedByCompose(service string, completedServices map[string]bool) bool {
	service = strings.TrimSpace(service)
	return service != "" && completedServices[service]
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
		case "s3", "bucket", "fixture", environmentRestoreAssetTokenSeed:
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
		case "s3", "bucket", "fixture", environmentRestoreAssetTokenSeed:
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
