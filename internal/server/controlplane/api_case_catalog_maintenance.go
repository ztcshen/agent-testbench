package controlplane

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"agent-testbench/internal/domain/casemaintenance"
	"agent-testbench/internal/store"
)

const (
	caseCatalogPatchOperation    = "case-patch"
	caseCatalogRollbackOperation = "rollback"
)

type caseCatalogSnapshotResponse struct {
	OK               bool                          `json:"ok"`
	ProfileID        string                        `json:"profileId"`
	Revision         int64                         `json:"revision"`
	SHA256           string                        `json:"sha256"`
	UpdatedAt        time.Time                     `json:"updatedAt"`
	Cases            []caseCatalogCaseResponse     `json:"cases"`
	InterfaceNodes   []caseCatalogNodeResponse     `json:"interfaceNodes"`
	RequestTemplates []caseCatalogTemplateResponse `json:"requestTemplates"`
}

type caseCatalogCaseResponse struct {
	ID                   string   `json:"id"`
	DisplayName          string   `json:"displayName,omitempty"`
	Description          string   `json:"description,omitempty"`
	NodeID               string   `json:"nodeId"`
	CaseType             string   `json:"caseType,omitempty"`
	Scenario             string   `json:"scenario,omitempty"`
	Tags                 []string `json:"tags,omitempty"`
	Priority             string   `json:"priority,omitempty"`
	Owner                string   `json:"owner,omitempty"`
	RequestTemplateID    string   `json:"requestTemplateId,omitempty"`
	RenderMode           string   `json:"renderMode,omitempty"`
	RequiredForAdmission bool     `json:"requiredForAdmission"`
	Status               string   `json:"status"`
	SortOrder            int      `json:"sortOrder"`
	CasePath             string   `json:"casePath,omitempty"`
	SourceKind           string   `json:"sourceKind,omitempty"`
	SourcePath           string   `json:"sourcePath,omitempty"`
	ExecutorID           string   `json:"executorId,omitempty"`
	BaseURL              string   `json:"baseUrl,omitempty"`
	EvidenceDir          string   `json:"evidenceDir,omitempty"`
	TimeoutSeconds       int      `json:"timeoutSeconds,omitempty"`
}

type caseCatalogNodeResponse struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	Method      string `json:"method,omitempty"`
	Path        string `json:"path,omitempty"`
	Status      string `json:"status,omitempty"`
}

type caseCatalogTemplateResponse struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	NodeID      string `json:"nodeId,omitempty"`
	Method      string `json:"method,omitempty"`
	Path        string `json:"path,omitempty"`
	Status      string `json:"status,omitempty"`
}

type caseCatalogHistoryResponse struct {
	OK        bool                             `json:"ok"`
	ProfileID string                           `json:"profileId"`
	Revision  int64                            `json:"revision"`
	Items     []caseCatalogHistoryResponseItem `json:"items"`
}

type caseCatalogHistoryResponseItem struct {
	Revision  int64          `json:"revision"`
	SHA256    string         `json:"sha256"`
	Operation string         `json:"operation"`
	Summary   map[string]any `json:"summary,omitempty"`
	CaseCount int            `json:"caseCount"`
	CreatedAt time.Time      `json:"createdAt"`
}

type caseCatalogMutationRequest struct {
	Store            store.VersionedProfileCatalogStore
	Current          store.ProfileCatalogSnapshot
	ExpectedRevision int64
	Payload          map[string]any
}

func registerCaseCatalogMaintenanceRoutes(mux *http.ServeMux, runtime store.Store) {
	mux.HandleFunc("/api/case-catalog", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleCaseCatalogGet(w, r, runtime)
		case http.MethodPatch:
			handleCaseCatalogPatch(w, r, runtime)
		default:
			w.Header().Set("Allow", http.MethodGet+", "+http.MethodPatch)
			writeJSONStatus(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		}
	})
	handleMethod(mux, "/api/case-catalog/history", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleCaseCatalogHistory(w, r, runtime)
	})
	handleMethod(mux, "/api/case-catalog/rollback", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleCaseCatalogRollback(w, r, runtime)
	})
}

func handleCaseCatalogGet(w http.ResponseWriter, r *http.Request, runtime store.Store) {
	_, snapshot, ok := currentVersionedCaseCatalog(w, r, runtime)
	if !ok {
		return
	}
	writeCaseCatalogSnapshot(w, snapshot)
}

func handleCaseCatalogPatch(w http.ResponseWriter, r *http.Request, runtime store.Store) {
	request, ok := prepareCaseCatalogMutation(w, r, runtime)
	if !ok {
		return
	}
	catalogValue, changedCase, created, err := patchCaseCatalog(request.Current.Catalog, request.Payload)
	if err != nil {
		writeCaseCatalogPatchError(w, err)
		return
	}
	summary, err := json.Marshal(map[string]any{"caseId": changedCase.ID, "created": created})
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": fmt.Errorf("encode case catalog mutation summary: %w", err).Error()})
		return
	}
	updated, err := request.Store.CompareAndSwapProfileCatalog(r.Context(), request.ExpectedRevision, catalogValue, store.ProfileCatalogMutation{
		Operation:   caseCatalogPatchOperation,
		SummaryJSON: string(summary),
	})
	if err != nil {
		writeCaseCatalogMutationError(w, request.ExpectedRevision, err)
		return
	}
	w.Header().Set("ETag", caseCatalogETag(updated))
	writeJSON(w, map[string]any{
		"ok":        true,
		"profileId": updated.Catalog.ProfileID,
		"revision":  updated.Revision,
		"sha256":    updated.SHA256,
		"updatedAt": updated.UpdatedAt,
		"created":   created,
		"case":      caseCatalogCaseView(changedCase),
	})
}

func handleCaseCatalogHistory(w http.ResponseWriter, r *http.Request, runtime store.Store) {
	versioned, current, ok := currentVersionedCaseCatalog(w, r, runtime)
	if !ok {
		return
	}
	limit := queryIntValue(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	versions, err := versioned.ListProfileCatalogVersions(r.Context(), current.Catalog.ProfileID, limit)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	items := make([]caseCatalogHistoryResponseItem, 0, len(versions))
	for _, version := range versions {
		items = append(items, caseCatalogHistoryResponseItem{
			Revision:  version.Revision,
			SHA256:    version.SHA256,
			Operation: version.Operation,
			Summary:   safeCaseCatalogHistorySummary(version.SummaryJSON),
			CaseCount: len(version.Catalog.APICases),
			CreatedAt: version.CreatedAt,
		})
	}
	writeJSON(w, caseCatalogHistoryResponse{
		OK:        true,
		ProfileID: current.Catalog.ProfileID,
		Revision:  current.Revision,
		Items:     items,
	})
}

func handleCaseCatalogRollback(w http.ResponseWriter, r *http.Request, runtime store.Store) {
	request, ok := prepareCaseCatalogMutation(w, r, runtime)
	if !ok {
		return
	}
	targetRevision, err := caseCatalogRevisionValue(request.Payload["revision"])
	if err != nil || targetRevision <= 0 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "revision must be a positive integer"})
		return
	}
	target, err := request.Store.GetProfileCatalogVersion(r.Context(), request.Current.Catalog.ProfileID, targetRevision)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"ok": false, "error": "catalog revision was not found"})
		return
	}
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if target.SHA256 == request.Current.SHA256 {
		writeJSONStatus(w, http.StatusConflict, map[string]any{
			"ok":       false,
			"error":    "requested catalog revision is already current",
			"revision": request.Current.Revision,
		})
		return
	}
	summary, err := json.Marshal(map[string]any{
		"targetRevision":   target.Revision,
		"previousRevision": request.Current.Revision,
	})
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": fmt.Errorf("encode case catalog rollback summary: %w", err).Error()})
		return
	}
	rollbackCatalog := target.Catalog
	rollbackCatalog.IndexedAt = request.Current.Catalog.IndexedAt
	updated, err := request.Store.CompareAndSwapProfileCatalog(r.Context(), request.ExpectedRevision, rollbackCatalog, store.ProfileCatalogMutation{
		Operation:   caseCatalogRollbackOperation,
		SummaryJSON: string(summary),
	})
	if err != nil {
		writeCaseCatalogMutationError(w, request.ExpectedRevision, err)
		return
	}
	if updated.Revision <= request.Current.Revision {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "rollback did not create a new catalog revision"})
		return
	}
	w.Header().Set("ETag", caseCatalogETag(updated))
	writeJSON(w, map[string]any{
		"ok":               true,
		"profileId":        updated.Catalog.ProfileID,
		"revision":         updated.Revision,
		"sha256":           updated.SHA256,
		"updatedAt":        updated.UpdatedAt,
		"targetRevision":   target.Revision,
		"previousRevision": request.Current.Revision,
	})
}

func prepareCaseCatalogMutation(w http.ResponseWriter, r *http.Request, runtime store.Store) (caseCatalogMutationRequest, bool) {
	versioned, current, ok := currentVersionedCaseCatalog(w, r, runtime)
	if !ok {
		return caseCatalogMutationRequest{}, false
	}
	expectedRevision, ok := requireCaseCatalogIfMatch(w, r, current)
	if !ok {
		return caseCatalogMutationRequest{}, false
	}
	payload, err := readJSONPayload(r)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return caseCatalogMutationRequest{}, false
	}
	return caseCatalogMutationRequest{
		Store:            versioned,
		Current:          current,
		ExpectedRevision: expectedRevision,
		Payload:          payload,
	}, true
}

func currentVersionedCaseCatalog(w http.ResponseWriter, r *http.Request, runtime store.Store) (store.VersionedProfileCatalogStore, store.ProfileCatalogSnapshot, bool) {
	if runtime == nil {
		writeJSONStatus(w, http.StatusNotImplemented, map[string]any{"ok": false, "error": "runtime store is not configured"})
		return nil, store.ProfileCatalogSnapshot{}, false
	}
	versioned, ok := runtime.(store.VersionedProfileCatalogStore)
	if !ok {
		writeJSONStatus(w, http.StatusNotImplemented, map[string]any{"ok": false, "error": "runtime store does not support versioned profile catalogs"})
		return nil, store.ProfileCatalogSnapshot{}, false
	}
	catalogValue, err := runtime.GetProfileCatalog(r.Context())
	if errors.Is(err, store.ErrNotFound) {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"ok": false, "error": "profile catalog was not found"})
		return nil, store.ProfileCatalogSnapshot{}, false
	}
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return nil, store.ProfileCatalogSnapshot{}, false
	}
	snapshot, err := versioned.GetProfileCatalogSnapshot(r.Context(), catalogValue.ProfileID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"ok": false, "error": "versioned profile catalog was not found"})
		return nil, store.ProfileCatalogSnapshot{}, false
	}
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return nil, store.ProfileCatalogSnapshot{}, false
	}
	return versioned, snapshot, true
}

func requireCaseCatalogIfMatch(w http.ResponseWriter, r *http.Request, current store.ProfileCatalogSnapshot) (int64, bool) {
	header := strings.TrimSpace(r.Header.Get("If-Match"))
	if header == "" {
		writeJSONStatus(w, http.StatusPreconditionRequired, map[string]any{
			"ok":         false,
			"error":      "If-Match is required",
			apiFieldCode: "if_match_required",
		})
		return 0, false
	}
	expectedRevision, err := parseCaseCatalogETag(header)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"ok":         false,
			"error":      "If-Match must contain one strong case catalog ETag",
			apiFieldCode: "invalid_if_match",
		})
		return 0, false
	}
	if header != caseCatalogETag(current) {
		writeCaseCatalogPreconditionFailed(w, expectedRevision, current.Revision)
		return 0, false
	}
	return expectedRevision, true
}

func writeCaseCatalogMutationError(w http.ResponseWriter, expectedRevision int64, err error) {
	var conflict *store.ProfileCatalogRevisionConflictError
	if errors.As(err, &conflict) || errors.Is(err, store.ErrProfileCatalogRevisionConflict) {
		actualRevision := int64(0)
		if conflict != nil {
			actualRevision = conflict.ActualRevision
		}
		writeCaseCatalogPreconditionFailed(w, expectedRevision, actualRevision)
		return
	}
	writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
}

func writeCaseCatalogPreconditionFailed(w http.ResponseWriter, expectedRevision int64, actualRevision int64) {
	writeJSONStatus(w, http.StatusPreconditionFailed, map[string]any{
		"ok":               false,
		"error":            "profile catalog revision changed",
		apiFieldCode:       "catalog_revision_conflict",
		"expectedRevision": expectedRevision,
		"currentRevision":  actualRevision,
	})
}

func writeCaseCatalogSnapshot(w http.ResponseWriter, snapshot store.ProfileCatalogSnapshot) {
	w.Header().Set("ETag", caseCatalogETag(snapshot))
	writeJSON(w, caseCatalogSnapshotView(snapshot))
}

func caseCatalogSnapshotView(snapshot store.ProfileCatalogSnapshot) caseCatalogSnapshotResponse {
	response := caseCatalogSnapshotResponse{
		OK:               true,
		ProfileID:        snapshot.Catalog.ProfileID,
		Revision:         snapshot.Revision,
		SHA256:           snapshot.SHA256,
		UpdatedAt:        snapshot.UpdatedAt,
		Cases:            make([]caseCatalogCaseResponse, 0, len(snapshot.Catalog.APICases)),
		InterfaceNodes:   make([]caseCatalogNodeResponse, 0, len(snapshot.Catalog.InterfaceNodes)),
		RequestTemplates: make([]caseCatalogTemplateResponse, 0, len(snapshot.Catalog.RequestTemplates)),
	}
	for _, apiCase := range snapshot.Catalog.APICases {
		response.Cases = append(response.Cases, caseCatalogCaseView(apiCase))
	}
	for _, node := range snapshot.Catalog.InterfaceNodes {
		response.InterfaceNodes = append(response.InterfaceNodes, caseCatalogNodeResponse{
			ID: node.ID, DisplayName: node.DisplayName, Method: node.Method, Path: node.Path, Status: node.Status,
		})
	}
	for _, template := range snapshot.Catalog.RequestTemplates {
		response.RequestTemplates = append(response.RequestTemplates, caseCatalogTemplateResponse{
			ID: template.ID, DisplayName: template.DisplayName, NodeID: template.NodeID,
			Method: template.Method, Path: template.Path, Status: template.Status,
		})
	}
	return response
}

func caseCatalogCaseView(apiCase store.CatalogAPICase) caseCatalogCaseResponse {
	status := apiCase.Status
	if normalized, err := casemaintenance.NormalizeCaseStatus(status, false); err == nil {
		status = normalized
	}
	return caseCatalogCaseResponse{
		ID:                   apiCase.ID,
		DisplayName:          apiCase.DisplayName,
		Description:          apiCase.Description,
		NodeID:               apiCase.NodeID,
		CaseType:             apiCase.CaseType,
		Scenario:             apiCase.Scenario,
		Tags:                 append([]string(nil), apiCase.Tags...),
		Priority:             apiCase.Priority,
		Owner:                apiCase.Owner,
		RequestTemplateID:    apiCase.RequestTemplateID,
		RenderMode:           apiCase.RenderMode,
		RequiredForAdmission: apiCase.RequiredForAdmission,
		Status:               status,
		SortOrder:            apiCase.SortOrder,
		CasePath:             apiCase.CasePath,
		SourceKind:           apiCase.SourceKind,
		SourcePath:           apiCase.SourcePath,
		ExecutorID:           apiCase.ExecutorID,
		BaseURL:              apiCase.BaseURL,
		EvidenceDir:          apiCase.EvidenceDir,
		TimeoutSeconds:       apiCase.TimeoutSeconds,
	}
}

func caseCatalogETag(snapshot store.ProfileCatalogSnapshot) string {
	return fmt.Sprintf(`"case-catalog-r%d-%s"`, snapshot.Revision, snapshot.SHA256)
}

func parseCaseCatalogETag(value string) (int64, error) {
	if strings.Contains(value, ",") || strings.HasPrefix(value, "W/") || len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' {
		return 0, errors.New("invalid case catalog ETag")
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(value, `"case-catalog-r`), `"`)
	separator := strings.IndexByte(inner, '-')
	if separator <= 0 || separator == len(inner)-1 {
		return 0, errors.New("invalid case catalog ETag")
	}
	revision, err := strconv.ParseInt(inner[:separator], 10, 64)
	if err != nil || revision <= 0 {
		return 0, errors.New("invalid case catalog ETag")
	}
	for _, character := range inner[separator+1:] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') && (character < 'A' || character > 'F') {
			return 0, errors.New("invalid case catalog ETag")
		}
	}
	return revision, nil
}

func safeCaseCatalogHistorySummary(raw string) map[string]any {
	var decoded map[string]any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&decoded) != nil {
		return nil
	}
	safe := map[string]any{}
	for _, key := range []string{"caseId"} {
		if value, ok := decoded[key].(string); ok && strings.TrimSpace(value) != "" {
			safe[key] = value
		}
	}
	for _, key := range []string{"created"} {
		if value, ok := decoded[key].(bool); ok {
			safe[key] = value
		}
	}
	for _, key := range []string{"targetRevision", "previousRevision"} {
		if value, err := caseCatalogRevisionValue(decoded[key]); err == nil && value > 0 {
			safe[key] = value
		}
	}
	if len(safe) == 0 {
		return nil
	}
	return safe
}

func caseCatalogRevisionValue(value any) (int64, error) {
	switch typed := value.(type) {
	case json.Number:
		return typed.Int64()
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case float64:
		if typed != float64(int64(typed)) {
			return 0, errors.New("revision must be an integer")
		}
		return int64(typed), nil
	case string:
		return strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
	default:
		return 0, errors.New("revision must be an integer")
	}
}
