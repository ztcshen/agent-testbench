// Package environmentprojection adapts Store environment records into
// domain-level file projection inputs.
package environmentprojection

import (
	"encoding/json"
	"strings"

	"agent-testbench/internal/domain/environmentfiles"
	"agent-testbench/internal/store"
)

func FromEnvironment(env store.Environment, graph store.EnvironmentComponentGraph) environmentfiles.ProjectionReport {
	return environmentfiles.FromJSON(env.ComposeJSON, env.SummaryJSON, AssetsFromGraph(graph))
}

func FromEnvironmentWithEnvironmentFiles(env store.Environment, graph store.EnvironmentComponentGraph, files []store.EnvironmentFile) environmentfiles.ProjectionReport {
	return environmentfiles.FromComposeWithSources(jsonObject(env.ComposeJSON), jsonObject(env.SummaryJSON), AssetsFromGraph(graph), SourcesFromEnvironmentFiles(files))
}

func FromCompose(compose map[string]any, summary map[string]any, graph store.EnvironmentComponentGraph) environmentfiles.ProjectionReport {
	return environmentfiles.FromCompose(compose, summary, AssetsFromGraph(graph))
}

func FromComposeWithEnvironmentFiles(compose map[string]any, summary map[string]any, graph store.EnvironmentComponentGraph, files []store.EnvironmentFile) environmentfiles.ProjectionReport {
	return environmentfiles.FromComposeWithSources(compose, summary, AssetsFromGraph(graph), SourcesFromEnvironmentFiles(files))
}

func AssetsFromGraph(graph store.EnvironmentComponentGraph) []environmentfiles.ProjectionAsset {
	out := make([]environmentfiles.ProjectionAsset, 0, len(graph.Assets))
	for _, asset := range graph.Assets {
		out = append(out, environmentfiles.ProjectionAsset{
			OwnerComponentID:  asset.OwnerComponentID,
			AssetID:           asset.AssetID,
			AssetKind:         asset.AssetKind,
			TargetComponentID: asset.TargetComponentID,
			TargetPath:        asset.TargetPath,
			ContentInline:     asset.ContentInline,
			RemoteRefJSON:     asset.RemoteRefJSON,
		})
	}
	return out
}

func jsonObject(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func SourcesFromEnvironmentFiles(files []store.EnvironmentFile) []environmentfiles.ProjectionSource {
	out := make([]environmentfiles.ProjectionSource, 0, len(files))
	for _, file := range files {
		out = append(out, environmentfiles.ProjectionSource{
			Path:   file.Path,
			Source: "environment_files",
		})
	}
	return out
}
