package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"agent-testbench/internal/domain/environmentsource"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

const (
	environmentRestoreAssetKindComposeSecret = "compose-secret"
	environmentRestoreAssetKindDockerSecret  = "docker-secret"
)

func environmentRestoreOrderedComponentAssets(envID string, g store.EnvironmentComponentGraph) []store.ComponentConfigAsset {
	out := append([]store.ComponentConfigAsset{}, g.Assets...)
	if len(out) == 0 {
		return out
	}
	componentOrder := controlplane.EnvironmentComponentGraphReadinessReport(envID, g).BlockingOrder
	ownerIndex := map[string]int{}
	for i, id := range componentOrder {
		ownerIndex[id] = i
	}
	defaultRank := len(componentOrder) + len(g.Components) + 1
	sort.SliceStable(out, func(i, j int) bool {
		left := out[i]
		right := out[j]
		leftOwner := strings.TrimSpace(left.OwnerComponentID)
		rightOwner := strings.TrimSpace(right.OwnerComponentID)
		leftRank, leftOK := ownerIndex[leftOwner]
		if !leftOK {
			leftRank = defaultRank
		}
		rightRank, rightOK := ownerIndex[rightOwner]
		if !rightOK {
			rightRank = defaultRank
		}
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if leftOwner != rightOwner {
			return leftOwner < rightOwner
		}
		if left.ApplyOrder != right.ApplyOrder {
			return left.ApplyOrder < right.ApplyOrder
		}
		if left.AssetID != right.AssetID {
			return left.AssetID < right.AssetID
		}
		return left.TargetPath < right.TargetPath
	})
	return out
}

func environmentRestoreComponentAssetRemoteRefOK(asset store.ComponentConfigAsset) bool {
	return environmentsource.ComponentAssetRemoteRefOK(asset.TargetPath, asset.RemoteRefJSON)
}

func environmentRestoreComposeWithComponentAssets(envID string, compose map[string]any, graph store.EnvironmentComponentGraph) map[string]any {
	if len(graph.Assets) == 0 {
		return compose
	}
	out := map[string]any{}
	for key, value := range compose {
		out[key] = value
	}
	generated := generatedFileContentMapFromAny(out["generatedFiles"])
	generatedModes := stringMapFromAny(out["generatedFileModes"])
	generatedOrder := stringSliceFromAny(out["generatedFileOrder"])
	if len(generatedOrder) == 0 && len(generated) > 0 {
		for target := range generated {
			generatedOrder = append(generatedOrder, target)
		}
		sort.Strings(generatedOrder)
	}
	for _, asset := range environmentRestoreOrderedComponentAssets(envID, graph) {
		target := filepath.Clean(strings.TrimSpace(asset.TargetPath))
		if target == "." || target == "" || strings.HasPrefix(target, ".."+string(os.PathSeparator)) || filepath.IsAbs(target) {
			continue
		}
		if strings.TrimSpace(asset.ContentInline) == "" {
			continue
		}
		if _, exists := generated[target]; exists {
			continue
		}
		generated[target] = asset.ContentInline
		if environmentRestoreComponentAssetFileMode(asset) == 0o600 {
			generatedModes[target] = "0600"
		}
		generatedOrder = append(generatedOrder, target)
	}
	if len(generated) > 0 {
		out["generatedFiles"] = generated
	}
	if len(generatedModes) > 0 {
		out["generatedFileModes"] = generatedModes
	}
	if len(generatedOrder) > 0 {
		out["generatedFileOrder"] = dedupeStrings(generatedOrder)
	}
	return out
}

func environmentRestoreComponentAssetFileMode(asset store.ComponentConfigAsset) os.FileMode {
	kind := strings.ToLower(strings.TrimSpace(asset.AssetKind))
	if asset.Sensitive || kind == environmentRestoreAssetKindComposeSecret || kind == environmentRestoreAssetKindDockerSecret {
		return 0o600
	}
	if mode := environmentRestoreComponentAssetSummaryFileMode(asset); mode != 0 {
		return mode
	}
	return 0o644
}

func environmentRestoreComponentAssetSummaryFileMode(asset store.ComponentConfigAsset) os.FileMode {
	summary := jsonObjectString(asset.SummaryJSON)
	candidates := make([]any, 0, 9)
	candidates = append(candidates,
		summary["fileMode"],
		summary["mode"],
		summary["permissions"],
	)
	for _, key := range []string{"dockerNative", "projection"} {
		nested := jsonObjectFromAny(summary[key])
		candidates = append(candidates, nested["fileMode"], nested["mode"], nested["permissions"])
	}
	for _, candidate := range candidates {
		switch strings.TrimSpace(valueString(candidate)) {
		case "0600", "600":
			return 0o600
		case "0644", "644":
			return 0o644
		}
	}
	return 0
}

func environmentRestoreRemoteComponentAssets(ctx context.Context, envID string, graph store.EnvironmentComponentGraph, workspace string, execute bool, pull bool) []environmentRestoreComponentAsset {
	out := []environmentRestoreComponentAsset{}
	for _, asset := range environmentRestoreOrderedComponentAssets(envID, graph) {
		if strings.TrimSpace(asset.ContentInline) != "" || strings.TrimSpace(asset.RemoteRefJSON) == "" {
			continue
		}
		ref := jsonObjectString(asset.RemoteRefJSON)
		sourceURL := strings.TrimSpace(valueString(ref["url"]))
		sourcePath := strings.TrimSpace(valueString(ref["path"]))
		if sourcePath == "" {
			sourcePath = strings.TrimSpace(asset.TargetPath)
		}
		checkout := strings.TrimSpace(valueString(ref["checkout"]))
		if checkout == "" {
			checkout = filepath.Join(workspace, ".agent-testbench", "component-assets", safeReportID(sourceURL))
		} else if !filepath.IsAbs(checkout) {
			checkout = filepath.Join(workspace, checkout)
		}
		report := environmentRestoreComponentAsset{
			AssetID:          asset.AssetID,
			OwnerComponentID: asset.OwnerComponentID,
			SourceURL:        sourceURL,
			SourcePath:       sourcePath,
			Checkout:         checkout,
			TargetPath:       restoreWorkspacePath(workspace, asset.TargetPath),
			Bytes:            asset.SizeBytes,
			ApplyOrder:       asset.ApplyOrder,
			Action:           "plan-materialize",
			OK:               true,
		}
		if !environmentRestoreComponentAssetRemoteRefOK(asset) {
			report.OK = false
			report.Error = "remote component asset requires remote Git URL plus relative source path"
			out = append(out, report)
			continue
		}
		if ok, errText := environmentRestoreGeneratedFileTargetOK(asset.TargetPath, workspace); !ok {
			report.OK = false
			report.Error = errText
			out = append(out, report)
			continue
		}
		spec := environmentRestoreRepoSpec{
			ServiceID: "component-asset-" + safeReportID(asset.AssetID),
			URL:       sourceURL,
			Branch:    strings.TrimSpace(valueString(ref["branch"])),
			Ref:       strings.TrimSpace(valueString(ref["ref"])),
			Checkout:  checkout,
		}
		repo := environmentRestoreRepo(ctx, spec, execute, pull)
		report.RepoAction = repo.Action
		report.Command = repo.Command
		if !repo.OK {
			report.OK = false
			report.Error = repo.Error
			out = append(out, report)
			continue
		}
		if !execute {
			out = append(out, report)
			continue
		}
		report.Action = "materialize"
		sourceFile := filepath.Join(checkout, filepath.Clean(sourcePath))
		raw, err := os.ReadFile(sourceFile)
		if err != nil {
			report.OK = false
			report.Error = err.Error()
			out = append(out, report)
			continue
		}
		report.Bytes = int64(len(raw))
		if err := os.MkdirAll(filepath.Dir(report.TargetPath), 0o755); err != nil {
			report.OK = false
			report.Error = err.Error()
			out = append(out, report)
			continue
		}
		mode := environmentRestoreComponentAssetFileMode(asset)
		if err := os.WriteFile(report.TargetPath, raw, mode); err != nil {
			report.OK = false
			report.Error = err.Error()
		} else if err := os.Chmod(report.TargetPath, mode); err != nil {
			report.OK = false
			report.Error = err.Error()
		}
		out = append(out, report)
	}
	return out
}
