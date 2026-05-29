package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"agent-testbench/internal/store"
)

func addEnvironmentMigrationAsset(graph store.EnvironmentComponentGraph, edge environmentMigrationEdge, asset store.ComponentConfigAsset, force bool) (store.EnvironmentComponentGraph, environmentMigrationItem, error) {
	if !environmentMigrationComponentExists(graph, edge.Owner) {
		return graph, environmentMigrationItem{}, fmt.Errorf("migration owner component is not registered: %s", edge.Owner)
	}
	if !environmentMigrationComponentExists(graph, edge.Provider) {
		return graph, environmentMigrationItem{}, fmt.Errorf("migration provider component is not registered: %s", edge.Provider)
	}
	depIndex := -1
	for index := range graph.Dependencies {
		if graph.Dependencies[index].ConsumerComponentID == edge.Owner && graph.Dependencies[index].ProviderComponentID == edge.Provider {
			depIndex = index
			break
		}
	}
	if depIndex < 0 {
		return graph, environmentMigrationItem{}, fmt.Errorf("migration edge is not registered: %s:%s", edge.Owner, edge.Provider)
	}
	replaced := false
	for index := range graph.Assets {
		if graph.Assets[index].OwnerComponentID == asset.OwnerComponentID && graph.Assets[index].AssetID == asset.AssetID {
			if !force {
				return graph, environmentMigrationItem{}, fmt.Errorf("migration asset already exists: %s", asset.AssetID)
			}
			graph.Assets[index] = asset
			replaced = true
			break
		}
	}
	if !replaced {
		graph.Assets = append(graph.Assets, asset)
	}
	graph.Dependencies[depIndex].ProfileJSON = addEnvironmentMigrationAssetID(graph.Dependencies[depIndex].ProfileJSON, asset.AssetID)
	item, _ := environmentMigrationItemFromAsset(asset, edge.Provider)
	item.Status = "registered"
	item.OK = true
	return graph, item, nil
}

func environmentMigrationItems(graph store.EnvironmentComponentGraph, filter environmentMigrationEdge, database string, throughVersion string) []environmentMigrationItem {
	assets := map[string]store.ComponentConfigAsset{}
	for _, asset := range graph.Assets {
		if environmentMigrationAssetMetadata(asset).Version == "" {
			continue
		}
		key := asset.OwnerComponentID + "\x00" + asset.AssetID
		assets[key] = asset
	}
	var out []environmentMigrationItem
	for _, dep := range graph.Dependencies {
		edge := environmentMigrationEdge{Owner: dep.ConsumerComponentID, Provider: dep.ProviderComponentID}
		if filter.Owner != "" && (edge.Owner != filter.Owner || edge.Provider != filter.Provider) {
			continue
		}
		for _, assetID := range environmentRestoreDependencyAssetIDs(dep) {
			asset, ok := assets[edge.Owner+"\x00"+assetID]
			if !ok {
				continue
			}
			item, ok := environmentMigrationItemFromAsset(asset, edge.Provider)
			if !ok {
				continue
			}
			if database != "" && item.Database != database {
				continue
			}
			if environmentMigrationVersionAfter(item.Version, item.ApplyOrder, throughVersion) {
				continue
			}
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return compareEnvironmentMigrationItems(out[i], out[j]) < 0
	})
	return out
}

func compareEnvironmentMigrationItems(left environmentMigrationItem, right environmentMigrationItem) int {
	if cmp := strings.Compare(left.ProviderComponent, right.ProviderComponent); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(left.OwnerComponentID, right.OwnerComponentID); cmp != 0 {
		return cmp
	}
	if left.ApplyOrder != right.ApplyOrder {
		if left.ApplyOrder < right.ApplyOrder {
			return -1
		}
		return 1
	}
	return strings.Compare(left.Version, right.Version)
}

func environmentMigrationItemFromAsset(asset store.ComponentConfigAsset, provider string) (environmentMigrationItem, bool) {
	metadata := environmentMigrationAssetMetadata(asset)
	if metadata.Version == "" || metadata.Database == "" {
		return environmentMigrationItem{}, false
	}
	return environmentMigrationItem{
		AssetID:           asset.AssetID,
		EnvironmentID:     asset.EnvID,
		OwnerComponentID:  asset.OwnerComponentID,
		ProviderComponent: provider,
		TargetComponentID: asset.TargetComponentID,
		TargetPath:        asset.TargetPath,
		AssetKind:         asset.AssetKind,
		Version:           metadata.Version,
		Description:       metadata.Description,
		Database:          metadata.Database,
		Checksum:          firstNonEmpty(metadata.Checksum, asset.SHA256, sha256Hex(asset.ContentInline)),
		Preconditions:     metadata.Preconditions,
		ApplyOrder:        asset.ApplyOrder,
		Bytes:             len(asset.ContentInline),
		Content:           asset.ContentInline,
	}, true
}

func environmentMigrationAssetMetadata(asset store.ComponentConfigAsset) environmentMigrationMetadata {
	var summary environmentMigrationSummary
	if err := json.Unmarshal([]byte(stringDefault(asset.SummaryJSON, "{}")), &summary); err != nil {
		return environmentMigrationMetadata{}
	}
	if summary.Migration.Checksum == "" {
		summary.Migration.Checksum = firstNonEmpty(asset.SHA256, sha256Hex(asset.ContentInline))
	}
	return summary.Migration
}

func environmentMigrationIsAsset(asset store.ComponentConfigAsset) bool {
	metadata := environmentMigrationAssetMetadata(asset)
	if metadata.Version != "" && metadata.Database != "" {
		return true
	}
	kind := strings.ToLower(strings.TrimSpace(asset.AssetKind))
	return strings.Contains(kind, "migration") && environmentRestoreIsMySQLSQLAsset(asset, store.ComponentDependency{Capability: "sql", ProviderComponentID: asset.TargetComponentID})
}

func parseEnvironmentMigrationEdge(raw string) (environmentMigrationEdge, error) {
	edge, err := parseOptionalEnvironmentMigrationEdge(raw)
	if err != nil {
		return environmentMigrationEdge{}, err
	}
	if edge.Owner == "" || edge.Provider == "" {
		return environmentMigrationEdge{}, errors.New("--edge OWNER:PROVIDER is required")
	}
	return edge, nil
}

func parseOptionalEnvironmentMigrationEdge(raw string) (environmentMigrationEdge, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return environmentMigrationEdge{}, nil
	}
	parts := strings.Split(trimmed, ":")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return environmentMigrationEdge{}, fmt.Errorf("invalid migration edge %q; expected OWNER:PROVIDER", raw)
	}
	return environmentMigrationEdge{Owner: strings.TrimSpace(parts[0]), Provider: strings.TrimSpace(parts[1])}, nil
}

func parseEnvironmentMigrationPreconditions(values []string) []environmentMigrationPrecondition {
	out := []environmentMigrationPrecondition{}
	for _, value := range values {
		kind, body, ok := strings.Cut(strings.TrimSpace(value), ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(kind) {
		case environmentMigrationPreconditionColumnNotExists:
			table, column, ok := strings.Cut(strings.TrimSpace(body), ".")
			if ok && strings.TrimSpace(table) != "" && strings.TrimSpace(column) != "" {
				out = append(out, environmentMigrationPrecondition{Type: environmentMigrationPreconditionColumnNotExists, Table: strings.TrimSpace(table), Column: strings.TrimSpace(column)})
			}
		}
	}
	return out
}

func addEnvironmentMigrationAssetID(profileJSON string, assetID string) string {
	profile := jsonObjectString(profileJSON)
	ids := stringSliceFromAny(profile["assetIds"])
	ids = append(ids, assetID)
	profile["assetIds"] = dedupeStrings(ids)
	return mustCompactJSON(profile)
}

func environmentMigrationComponentExists(graph store.EnvironmentComponentGraph, id string) bool {
	for _, component := range graph.Components {
		if component.ComponentID == id {
			return true
		}
	}
	return false
}

func environmentMigrationTargetService(graph store.EnvironmentComponentGraph, id string) string {
	for _, component := range graph.Components {
		if component.ComponentID == id {
			return environmentRestoreComponentComposeService(component, id)
		}
	}
	return strings.TrimSpace(id)
}

func defaultEnvironmentMigrationAssetID(edge environmentMigrationEdge, metadata environmentMigrationMetadata) string {
	parts := []string{edge.Owner, edge.Provider, "migration", metadata.Version}
	if metadata.Description != "" {
		parts = append(parts, metadata.Description)
	}
	return sanitizeEnvironmentMigrationID(strings.Join(parts, "."))
}

func sanitizeEnvironmentMigrationID(value string) string {
	var b strings.Builder
	lastDot := false
	for _, r := range strings.ToLower(value) {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDot = false
			continue
		}
		if !lastDot {
			b.WriteByte('.')
			lastDot = true
		}
	}
	return strings.Trim(b.String(), ".")
}

func environmentMigrationDefaultApplyOrder(version string) int {
	digits := strings.Builder{}
	for _, r := range version {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	if digits.Len() == 0 {
		return 0
	}
	value, err := strconv.Atoi(digits.String())
	if err != nil {
		return 0
	}
	return value
}

func environmentMigrationVersionAfter(version string, applyOrder int, throughVersion string) bool {
	throughVersion = strings.TrimSpace(throughVersion)
	if throughVersion == "" {
		return false
	}
	throughOrder := environmentMigrationDefaultApplyOrder(throughVersion)
	itemOrder := applyOrder
	if itemOrder == 0 {
		itemOrder = environmentMigrationDefaultApplyOrder(version)
	}
	if itemOrder != 0 && throughOrder != 0 {
		return itemOrder > throughOrder
	}
	return strings.Compare(version, throughVersion) > 0
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
