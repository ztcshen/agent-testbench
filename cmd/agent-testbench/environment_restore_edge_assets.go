package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"agent-testbench/internal/store"
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

type environmentRestoreApplyAssetOptions struct {
	UseExistingContainers bool
	CompletedServices     map[string]bool
}

func environmentRestoreApplyEdgeAssets(ctx context.Context, graph store.EnvironmentComponentGraph, compose map[string]any, workspace string, execute bool, composeBaseArgs []string) []environmentRestoreAppliedAsset {
	return environmentRestoreApplyEdgeAssetsWithOptions(ctx, graph, compose, workspace, execute, composeBaseArgs, environmentRestoreApplyAssetOptions{
		CompletedServices: environmentRestoreCompletedDependencyServices(compose, workspace),
	})
}

func environmentRestoreApplyEdgeAssetsWithOptions(ctx context.Context, graph store.EnvironmentComponentGraph, compose map[string]any, workspace string, execute bool, composeBaseArgs []string, options environmentRestoreApplyAssetOptions) []environmentRestoreAppliedAsset {
	if len(graph.Dependencies) == 0 {
		return nil
	}
	componentByID := environmentRestoreComponentMap(graph.Components)
	generated := generatedFileContentMapFromAny(compose["generatedFiles"])
	out := []environmentRestoreAppliedAsset{}
	for _, ref := range environmentRestoreDependencyAssetRefs(graph) {
		if !ref.Found {
			out = append(out, environmentRestoreAppliedAsset{
				AssetID:            ref.AssetID,
				DependencyConsumer: ref.Dependency.ConsumerComponentID,
				DependencyProvider: ref.Dependency.ProviderComponentID,
				Action:             "missing-edge-asset",
				OK:                 false,
				Error:              "component dependency references missing config asset: " + ref.AssetID,
			})
			continue
		}
		item := environmentRestoreApplyEdgeAsset(ctx, ref.Dependency, ref.Asset, componentByID, compose, generated, workspace, execute, composeBaseArgs, options)
		out = append(out, item)
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

type environmentRestoreDependencyAssetRef struct {
	Dependency        store.ComponentDependency
	AssetID           string
	Asset             store.ComponentConfigAsset
	Found             bool
	TargetComponentID string
}

func environmentRestoreDependencyAssetRefs(graph store.EnvironmentComponentGraph) []environmentRestoreDependencyAssetRef {
	assetsByID := environmentRestoreAssetMap(graph.Assets)
	out := []environmentRestoreDependencyAssetRef{}
	appliedAssetTargets := map[string]bool{}
	for _, dep := range graph.Dependencies {
		for _, assetID := range environmentRestoreDependencyAssetIDs(dep) {
			asset, ok := assetsByID[assetID]
			if !ok {
				out = append(out, environmentRestoreDependencyAssetRef{
					Dependency: dep,
					AssetID:    assetID,
					Found:      false,
				})
				continue
			}
			targetComponentID := firstNonEmpty(strings.TrimSpace(asset.TargetComponentID), strings.TrimSpace(dep.ProviderComponentID))
			dedupeKey := assetID + "\x00" + targetComponentID
			if appliedAssetTargets[dedupeKey] {
				continue
			}
			appliedAssetTargets[dedupeKey] = true
			out = append(out, environmentRestoreDependencyAssetRef{
				Dependency:        dep,
				AssetID:           assetID,
				Asset:             asset,
				Found:             true,
				TargetComponentID: targetComponentID,
			})
		}
	}
	return out
}

func environmentRestoreAssetMap(assets []store.ComponentConfigAsset) map[string]store.ComponentConfigAsset {
	out := map[string]store.ComponentConfigAsset{}
	for _, asset := range assets {
		if id := strings.TrimSpace(asset.AssetID); id != "" {
			out[id] = asset
		}
	}
	return out
}

func environmentRestoreComponentMap(components []store.EnvironmentComponent) map[string]store.EnvironmentComponent {
	out := map[string]store.EnvironmentComponent{}
	for _, component := range components {
		if id := strings.TrimSpace(component.ComponentID); id != "" {
			out[id] = component
		}
	}
	return out
}

func environmentRestoreApplyEdgeAsset(ctx context.Context, dep store.ComponentDependency, asset store.ComponentConfigAsset, components map[string]store.EnvironmentComponent, compose map[string]any, generated map[string]string, workspace string, execute bool, composeBaseArgs []string, options environmentRestoreApplyAssetOptions) environmentRestoreAppliedAsset {
	targetComponentID := firstNonEmpty(strings.TrimSpace(asset.TargetComponentID), strings.TrimSpace(dep.ProviderComponentID))
	targetService := environmentRestoreComponentComposeService(components[targetComponentID], targetComponentID)
	item := environmentRestoreAppliedAsset{
		AssetID:              strings.TrimSpace(asset.AssetID),
		OwnerComponentID:     strings.TrimSpace(asset.OwnerComponentID),
		TargetComponentID:    targetComponentID,
		TargetComposeService: targetService,
		DependencyConsumer:   strings.TrimSpace(dep.ConsumerComponentID),
		DependencyProvider:   strings.TrimSpace(dep.ProviderComponentID),
		TargetPath:           strings.TrimSpace(asset.TargetPath),
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
		content, contentErr := environmentRestoreEdgeAssetContent(asset, workspace)
		item.Bytes = len(content)
		return environmentRestoreApplyMigrationEdgeAsset(ctx, dep, asset, content, contentErr, workspace, execute, composeBaseArgs, item)
	}
	if environmentRestoreIsObjectStorageAsset(asset, dep) {
		content, contentErr := environmentRestoreEdgeAssetContent(asset, workspace)
		item.Bytes = len(content)
		return environmentRestoreApplyObjectStorageEdgeAsset(ctx, dep, asset, components[targetComponentID], content, contentErr, workspace, execute, options, item)
	}
	if environmentRestoreIsMySQLSQLAsset(asset, dep) {
		if execute && options.UseExistingContainers {
			return environmentRestoreApplyMySQLSQLEdgeAsset(ctx, "", nil, compose, workspace, execute, composeBaseArgs, options, item)
		}
		content, contentErr := environmentRestoreEdgeAssetContent(asset, workspace)
		item.Bytes = len(content)
		return environmentRestoreApplyMySQLSQLEdgeAsset(ctx, content, contentErr, compose, workspace, execute, composeBaseArgs, options, item)
	}
	return environmentRestoreApplyGeneratedEdgeAsset(asset, generated, workspace, execute, item)
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

func environmentRestoreApplyGeneratedEdgeAsset(asset store.ComponentConfigAsset, generated map[string]string, workspace string, execute bool, item environmentRestoreAppliedAsset) environmentRestoreAppliedAsset {
	targetPath := filepath.Clean(strings.TrimSpace(asset.TargetPath))
	if targetPath == "." || targetPath == "" {
		item.OK = false
		item.Error = "edge asset target path is required"
		return item
	}
	if _, ok := generated[targetPath]; ok {
		item.Action = environmentRestoreGeneratedEdgeAssetAction(asset)
		if execute {
			if item.Action == "project-generated-file" {
				item.Action = "verify-generated-file"
			}
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

func environmentRestoreGeneratedEdgeAssetAction(asset store.ComponentConfigAsset) string {
	switch strings.ToLower(strings.TrimSpace(asset.AssetKind)) {
	case "compose-config", "docker-config":
		return "project-compose-config"
	case environmentRestoreAssetKindComposeSecret, environmentRestoreAssetKindDockerSecret:
		return "project-compose-secret"
	case "env-file", "compose-env-file":
		return "project-env-file"
	default:
		return "project-generated-file"
	}
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

func environmentRestoreComponentComposeService(component store.EnvironmentComponent, defaultID string) string {
	if service := strings.TrimSpace(component.ComposeService); service != "" {
		return service
	}
	return strings.TrimSpace(defaultID)
}
