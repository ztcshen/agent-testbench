package environmentprojection

import (
	"agent-testbench/internal/domain/environmentfiles"
	"agent-testbench/internal/store"
)

func FromEnvironment(env store.Environment, graph store.EnvironmentComponentGraph) environmentfiles.ProjectionReport {
	return environmentfiles.FromJSON(env.ComposeJSON, env.SummaryJSON, AssetsFromGraph(graph))
}

func FromCompose(compose map[string]any, summary map[string]any, graph store.EnvironmentComponentGraph) environmentfiles.ProjectionReport {
	return environmentfiles.FromCompose(compose, summary, AssetsFromGraph(graph))
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
