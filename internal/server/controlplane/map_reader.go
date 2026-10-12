package controlplane

import (
	"errors"
	"net/http"
	"strings"

	"agent-testbench/internal/domain/plangraph"
	"agent-testbench/internal/domain/redaction"
	"agent-testbench/internal/store"
)

func registerMapReaderRoutes(mux *http.ServeMux, runtime store.Store) {
	handleMethod(mux, "/api/maps", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		if !requireMapStore(w, runtime) {
			return
		}
		items, err := runtime.ListTestPlanMaps(r.Context())
		if writeMapReadError(w, err) {
			return
		}
		maps := make([]map[string]any, 0, len(items))
		for _, item := range items {
			maps = append(maps, map[string]any{"id": item.ID, "profileId": item.ProfileID, "displayName": item.DisplayName, "description": item.Description, "status": item.Status, "nodeCount": item.NodeCount, "edgeCount": item.EdgeCount, "pathCount": item.PathCount, "materializationCount": item.MaterializationCount, "createdAt": item.CreatedAt, "updatedAt": item.UpdatedAt})
		}
		writeJSON(w, redaction.Value(map[string]any{"ok": true, "count": len(maps), "maps": maps}))
	})
	handleMethod(mux, "/api/map", http.MethodGet, func(w http.ResponseWriter, r *http.Request) { handleMapRead(w, r, runtime) })
	handleMethod(mux, "/api/map/plan", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		if !requireMapStore(w, runtime) {
			return
		}
		plan, err := runtime.GetTestMapPlan(r.Context(), strings.TrimSpace(r.URL.Query().Get("id")))
		if writeMapReadError(w, err) {
			return
		}
		if plan.Instance.MapID != r.URL.Query().Get("mapId") {
			writeMapReadError(w, store.ErrNotFound)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "plan": mapPlanReadPayload(plan)})
	})
	handleMethod(mux, "/api/map/explanation", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		if !requireMapStore(w, runtime) {
			return
		}
		graph, err := runtime.GetTestPlanGraph(r.Context(), r.URL.Query().Get("id"))
		if writeMapReadError(w, err) {
			return
		}
		explanation, err := plangraph.ExplainCase(graph, plangraph.ExplainOptions{NodeID: r.URL.Query().Get("nodeId")})
		if err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "explanation": redaction.Value(cloneJSONObject(explanation))})
	})
}

func requireMapStore(w http.ResponseWriter, runtime store.Store) bool {
	if runtime != nil {
		return true
	}
	writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "runtime Store is not configured"})
	return false
}

func writeMapReadError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	status := http.StatusInternalServerError
	message := err.Error()
	if errors.Is(err, store.ErrNotFound) {
		status, message = http.StatusNotFound, "map or plan was not found"
	}
	writeJSONStatus(w, status, map[string]any{"ok": false, "error": message})
	return true
}

func handleMapRead(w http.ResponseWriter, r *http.Request, runtime store.Store) {
	if !requireMapStore(w, runtime) {
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "map id is required"})
		return
	}
	graph, err := runtime.GetTestPlanGraph(r.Context(), id)
	if writeMapReadError(w, err) {
		return
	}
	catalog, err := runtime.GetProfileCatalogByID(r.Context(), graph.Map.ProfileID)
	warnings := []string{}
	if errors.Is(err, store.ErrNotFound) {
		warnings = append(warnings, "The map profile catalog is unavailable; unresolved references retain their original IDs.")
		catalog = store.ProfileCatalog{ProfileID: graph.Map.ProfileID}
	} else if writeMapReadError(w, err) {
		return
	}
	plans, err := runtime.ListTestMapPlans(r.Context(), id, 50)
	if writeMapReadError(w, err) {
		return
	}
	versions, err := runtime.ListTestPlanMapVersions(r.Context(), id)
	if writeMapReadError(w, err) {
		return
	}
	planItems := make([]map[string]any, 0, len(plans))
	for _, plan := range plans {
		planItems = append(planItems, mapPlanSummary(plan))
	}
	versionItems := make([]map[string]any, 0, len(versions))
	for _, version := range versions {
		versionItems = append(versionItems, map[string]any{"id": version.ID, "version": version.Version, "status": version.Status, "summary": version.Summary, "createdAt": version.CreatedAt})
	}
	writeJSON(w, map[string]any{"ok": true, "graph": redaction.Value(cloneJSONObject(graph)), "context": mapReadContext(graph, catalog), "plans": redaction.Value(planItems), "versions": redaction.Value(versionItems), "warnings": warnings})
}
