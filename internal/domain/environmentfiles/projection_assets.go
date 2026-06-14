package environmentfiles

import (
	"strings"

	"agent-testbench/internal/domain/environmentsource"
)

func projectionFilesFromAssets(assets []ProjectionAsset) []ProjectionFile {
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

func projectionAssetContentByPath(assets []ProjectionAsset) map[string]string {
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

func projectionFileFromAsset(asset ProjectionAsset) ProjectionFile {
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
