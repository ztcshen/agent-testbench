package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"agent-testbench/internal/domain/redaction"
	"agent-testbench/internal/store"
)

type requestEditorRecord struct {
	ID             string         `json:"id"`
	NodeID         string         `json:"nodeId"`
	ServiceID      string         `json:"serviceId"`
	DisplayName    string         `json:"displayName"`
	Description    string         `json:"description"`
	Status         string         `json:"status"`
	BaseURL        string         `json:"baseUrl"`
	TimeoutSeconds int            `json:"timeoutSeconds"`
	Execution      map[string]any `json:"execution"`
	Variables      []string       `json:"variables"`
	Editable       bool           `json:"editable"`
}

func registerRequestEditorRoutes(mux *http.ServeMux, runtime store.Store) {
	mux.HandleFunc("/api/request-editor", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, snapshot, ok := currentVersionedCaseCatalog(w, r, runtime)
			if ok {
				writeRequestEditorSnapshot(w, snapshot)
			}
		case http.MethodPut:
			handleRequestEditorSave(w, r, runtime)
		default:
			w.Header().Set("Allow", "GET, PUT")
			writeJSONStatus(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		}
	})
}

func writeRequestEditorSnapshot(w http.ResponseWriter, snapshot store.ProfileCatalogSnapshot) {
	catalog := snapshot.Catalog
	items := make([]requestEditorRecord, 0, len(catalog.APICases))
	for _, item := range catalog.APICases {
		execution, _ := workflowStudioBase(catalog, "", "", item.ID)
		serviceID := ""
		for _, node := range catalog.InterfaceNodes {
			if node.ID == item.NodeID {
				serviceID = node.ServiceID
			}
		}
		preview, _ := redaction.Value(execution).(map[string]any)
		baseURL := item.BaseURL
		if parsed, err := url.Parse(baseURL); err == nil {
			parsed.User = nil
			baseURL = redaction.URL(parsed.String())
		}
		items = append(items, requestEditorRecord{ID: item.ID, NodeID: item.NodeID, ServiceID: serviceID, DisplayName: firstNonEmpty(item.DisplayName, item.ID), Description: item.Description, Status: item.Status, BaseURL: baseURL, TimeoutSeconds: item.TimeoutSeconds, Execution: preview, Variables: workflowStudioVariables(execution), Editable: requestEditorNative(item)})
	}
	w.Header().Set("ETag", caseCatalogETag(snapshot))
	writeJSON(w, map[string]any{"ok": true, "profileId": catalog.ProfileID, "revision": snapshot.Revision, "requests": items})
}

func requestEditorNative(item store.CatalogAPICase) bool {
	return item.CasePath == "" && item.ExecutorID == "" && item.SourcePath == ""
}

func handleRequestEditorSave(w http.ResponseWriter, r *http.Request, runtime store.Store) {
	mutation, ok := prepareCaseCatalogMutation(w, r, runtime)
	if !ok {
		return
	}
	raw, err := json.Marshal(mutation.Payload["request"])
	var draft requestEditorRecord
	if err == nil {
		err = json.Unmarshal(raw, &draft)
	}
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid request draft"})
		return
	}
	catalog, err := compileRequestEditor(mutation.Current.Catalog, draft)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	summary, _ := json.Marshal(map[string]any{"caseId": draft.ID, "nodeId": draft.NodeID})
	snapshot, err := mutation.Store.CompareAndSwapProfileCatalog(r.Context(), mutation.ExpectedRevision, catalog, store.ProfileCatalogMutation{Operation: "request-editor-save", SummaryJSON: string(summary)})
	if err != nil {
		writeCaseCatalogMutationError(w, mutation.ExpectedRevision, err)
		return
	}
	writeRequestEditorSnapshot(w, snapshot)
}

func validateRequestEditor(draft requestEditorRecord) error {
	if strings.TrimSpace(draft.ID) == "" || strings.TrimSpace(draft.NodeID) == "" || strings.TrimSpace(draft.DisplayName) == "" {
		return fmt.Errorf("request id, nodeId and displayName are required")
	}
	method := strings.ToUpper(valueString(draft.Execution["method"]))
	if !slices.Contains([]string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}, method) {
		return fmt.Errorf("unsupported HTTP method")
	}
	path := valueString(draft.Execution["path"])
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return fmt.Errorf("request path must start with a single slash")
	}
	if draft.BaseURL != "" {
		parsed, err := url.Parse(draft.BaseURL)
		if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("baseUrl must be an HTTP origin or base path without credentials or query parameters")
		}
	}
	if draft.TimeoutSeconds < 0 || draft.TimeoutSeconds > 300 {
		return fmt.Errorf("timeoutSeconds must be between 0 and 300")
	}
	var execution caseExecutionConfig
	raw, _ := json.Marshal(draft.Execution)
	if err := json.Unmarshal(raw, &execution); err != nil {
		return fmt.Errorf("invalid native HTTP configuration: %w", err)
	}
	for _, status := range execution.ExpectedHTTPCodes {
		if status < 100 || status > 599 {
			return fmt.Errorf("expected HTTP codes must be between 100 and 599")
		}
	}
	return nil
}
