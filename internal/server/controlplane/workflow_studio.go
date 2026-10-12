package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"agent-testbench/internal/domain/casemaintenance"
	"agent-testbench/internal/domain/redaction"
	"agent-testbench/internal/store"
)

type workflowStudioDefinition struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

type workflowStudioPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type workflowStudioBinding struct {
	Parameter    string `json:"parameter"`
	SourceStepID string `json:"sourceStepId"`
	From         string `json:"from"`
	Path         string `json:"path"`
}

type workflowStudioOutput struct {
	Name string `json:"name"`
	From string `json:"from"`
	Path string `json:"path"`
}

type workflowStudioStep struct {
	ID        string                  `json:"id"`
	Title     string                  `json:"title"`
	CaseID    string                  `json:"caseId"`
	Required  bool                    `json:"required"`
	Position  workflowStudioPosition  `json:"position"`
	Bindings  []workflowStudioBinding `json:"bindings"`
	Outputs   []workflowStudioOutput  `json:"outputs"`
	Variables []string                `json:"variables,omitempty"`
	Request   map[string]any          `json:"request,omitempty"`
}

type workflowStudioDraft struct {
	Workflow workflowStudioDefinition `json:"workflow"`
	Steps    []workflowStudioStep     `json:"steps"`
}

type workflowStudioCase struct {
	ID             string         `json:"id"`
	DisplayName    string         `json:"displayName"`
	NodeID         string         `json:"nodeId"`
	Status         string         `json:"status"`
	Method         string         `json:"method"`
	Path           string         `json:"path"`
	ExecutionReady bool           `json:"executionReady"`
	Variables      []string       `json:"variables"`
	ResponseFields []string       `json:"responseFields"`
	Request        map[string]any `json:"request"`
}

func registerWorkflowStudioRoutes(mux *http.ServeMux, runtime store.Store) {
	mux.HandleFunc("/api/workflow-studio", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, snapshot, ok := currentVersionedCaseCatalog(w, r, runtime)
			if ok {
				writeWorkflowStudioSnapshot(w, snapshot)
			}
		case http.MethodPut:
			handleWorkflowStudioSave(w, r, runtime)
		default:
			w.Header().Set("Allow", "GET, PUT")
			writeJSONStatus(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		}
	})
}

func handleWorkflowStudioSave(w http.ResponseWriter, r *http.Request, runtime store.Store) {
	request, ok := prepareCaseCatalogMutation(w, r, runtime)
	if !ok {
		return
	}
	raw, err := json.Marshal(request.Payload)
	var draft workflowStudioDraft
	if err == nil {
		err = json.Unmarshal(raw, &draft)
	}
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid workflow draft"})
		return
	}
	catalog, err := compileWorkflowStudio(request.Current.Catalog, draft)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	summary, _ := json.Marshal(map[string]any{"workflowId": draft.Workflow.ID, "stepCount": len(draft.Steps)})
	updated, err := request.Store.CompareAndSwapProfileCatalog(r.Context(), request.ExpectedRevision, catalog, store.ProfileCatalogMutation{Operation: "workflow-studio-save", SummaryJSON: string(summary)})
	if err != nil {
		writeCaseCatalogMutationError(w, request.ExpectedRevision, err)
		return
	}
	writeWorkflowStudioSnapshot(w, updated)
}

func writeWorkflowStudioSnapshot(w http.ResponseWriter, snapshot store.ProfileCatalogSnapshot) {
	catalog := snapshot.Catalog
	cases := make([]workflowStudioCase, 0, len(catalog.APICases))
	for _, item := range catalog.APICases {
		execution, _ := workflowStudioBase(catalog, "", "", item.ID)
		fields := []string{}
		for _, field := range catalog.InterfaceFields {
			if field.NodeID == item.NodeID && strings.EqualFold(field.Direction, "out") && field.FieldPath != "" {
				fields = append(fields, field.FieldPath)
			}
		}
		preview, _ := redaction.Value(execution).(map[string]any)
		cases = append(cases, workflowStudioCase{ID: item.ID, DisplayName: firstNonEmpty(item.DisplayName, item.ID), NodeID: item.NodeID, Status: item.Status, Method: valueString(execution["method"]), Path: valueString(execution["path"]), ExecutionReady: casemaintenance.ExecutionReady(catalog, item), Variables: workflowStudioVariables(execution), ResponseFields: fields, Request: preview})
	}
	workflows := make([]workflowStudioDraft, 0, len(catalog.Workflows))
	for _, item := range catalog.Workflows {
		draft := workflowStudioDraft{Workflow: workflowStudioDefinition{ID: item.ID, DisplayName: firstNonEmpty(item.DisplayName, item.ID), Description: item.Description}, Steps: []workflowStudioStep{}}
		bindings := []store.CatalogWorkflowBinding{}
		for _, binding := range catalog.WorkflowBindings {
			if binding.WorkflowID == item.ID {
				bindings = append(bindings, binding)
			}
		}
		sort.SliceStable(bindings, func(i, j int) bool {
			if bindings[i].SortOrder != bindings[j].SortOrder {
				return bindings[i].SortOrder < bindings[j].SortOrder
			}
			return bindings[i].StepID < bindings[j].StepID
		})
		for index, binding := range bindings {
			doc, _ := workflowStudioScopedDocument(catalog, item.ID, binding.StepID, binding.CaseID)
			base, baseDoc := workflowStudioBase(catalog, item.ID, binding.StepID, binding.CaseID)
			if len(doc) == 0 {
				doc = baseDoc
			}
			meta := mapFromAny(doc["workflowStudio"])
			step := workflowStudioStep{ID: binding.StepID, CaseID: binding.CaseID, Required: binding.Required, Title: firstNonEmpty(valueString(doc["title"]), binding.StepID), Position: workflowStudioPosition{X: 80, Y: float64(index * 190)}, Bindings: []workflowStudioBinding{}, Outputs: []workflowStudioOutput{}}
			step.Variables = workflowStudioVariables(base)
			step.Request, _ = redaction.Value(base).(map[string]any)
			if raw, err := json.Marshal(meta); err == nil {
				var saved workflowStudioStep
				if json.Unmarshal(raw, &saved) == nil {
					if _, exists := meta["position"]; exists {
						step.Position = saved.Position
					}
					step.Bindings = append(step.Bindings, saved.Bindings...)
				}
			}
			managed := stringSetFromAny(meta["managedExports"])
			for _, export := range listFromAny(doc["exports"]) {
				value := mapFromAny(export)
				if !managed[valueString(value["name"])] {
					step.Outputs = append(step.Outputs, workflowStudioOutput{Name: valueString(value["name"]), From: firstNonEmpty(valueString(value["from"]), "responsebody"), Path: valueString(value["path"])})
				}
			}
			draft.Steps = append(draft.Steps, step)
		}
		workflows = append(workflows, draft)
	}
	w.Header().Set("ETag", caseCatalogETag(snapshot))
	writeJSON(w, map[string]any{"ok": true, "profileId": catalog.ProfileID, "revision": snapshot.Revision, "workflows": workflows, "cases": cases})
}

func stringSetFromAny(value any) map[string]bool {
	result := map[string]bool{}
	for _, item := range listFromAny(value) {
		result[valueString(item)] = true
	}
	return result
}

func workflowStudioScopedDocument(catalog store.ProfileCatalog, workflowID, stepID, caseID string) (map[string]any, int) {
	for index, config := range catalog.TemplateConfigs {
		if config.ScopeType == "step" && config.WorkflowID == workflowID && config.ScopeID == stepID {
			doc := jsonObject(config.ConfigJSON)
			if id := valueString(doc["caseId"]); id == "" || id == caseID {
				return doc, index
			}
		}
	}
	return map[string]any{}, -1
}

func workflowStudioBase(catalog store.ProfileCatalog, workflowID, stepID, caseID string) (map[string]any, map[string]any) {
	doc, _ := workflowStudioScopedDocument(catalog, workflowID, stepID, caseID)
	meta := mapFromAny(doc["workflowStudio"])
	if base := mapFromAny(meta["baseExecution"]); len(base) > 0 {
		return base, doc
	}
	if execution := mapFromAny(doc["caseExecution"]); len(execution) > 0 {
		return execution, doc
	}
	for _, item := range catalog.TemplateConfigs {
		if item.ScopeType == "step" || (item.Status != "" && item.Status != "active") {
			continue
		}
		candidate := jsonObject(item.ConfigJSON)
		if casemaintenance.ExecutionConfigTargetsCase(item.ScopeType, item.ScopeID, item.ConfigJSON, caseID) {
			if execution := mapFromAny(candidate["caseExecution"]); len(execution) > 0 {
				if len(doc) == 0 {
					doc = candidate
				}
				return execution, doc
			}
		}
	}
	for _, item := range catalog.APICases {
		if item.ID == caseID {
			if execution := deriveCaseExecutionConfigFromCatalog(catalog, item); execution != nil {
				raw, _ := json.Marshal(execution)
				return jsonObject(string(raw)), doc
			}
		}
	}
	return map[string]any{}, doc
}

func workflowStudioError(stepID, reason string) error {
	return fmt.Errorf("step %s: %s", stepID, reason)
}
