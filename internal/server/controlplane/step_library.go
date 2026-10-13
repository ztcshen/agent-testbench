package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"agent-testbench/internal/domain/redaction"
	"agent-testbench/internal/domain/stepblock"
	"agent-testbench/internal/store"
)

const stepLibraryScope = "step-block"

type stepBlockUsage struct {
	WorkflowID   string `json:"workflowId"`
	WorkflowName string `json:"workflowName"`
	InstanceID   string `json:"instanceId"`
	Revision     string `json:"revision"`
	Outdated     bool   `json:"outdated"`
}

type stepBlockRecord struct {
	Definition stepblock.Definition `json:"definition"`
	Usages     []stepBlockUsage     `json:"usages"`
}

func registerStepLibraryRoutes(mux *http.ServeMux, runtime store.Store) {
	mux.HandleFunc("/api/step-library", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, snapshot, ok := currentVersionedCaseCatalog(w, r, runtime)
			if ok {
				writeStepLibrarySnapshot(w, snapshot)
			}
		case http.MethodPut, http.MethodDelete:
			handleStepLibraryMutation(w, r, runtime)
		default:
			w.Header().Set("Allow", "GET, PUT, DELETE")
			writeJSONStatus(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		}
	})
}

func stepLibraryDefinitions(catalog store.ProfileCatalog, activeOnly bool) []stepblock.Definition {
	defs := []stepblock.Definition{}
	for _, config := range catalog.TemplateConfigs {
		if config.ScopeType != stepLibraryScope || (activeOnly && config.Status != "active") {
			continue
		}
		var def stepblock.Definition
		if json.Unmarshal([]byte(config.ConfigJSON), &def) == nil && def.ID != "" {
			defs = append(defs, def)
		}
	}
	return defs
}

func findStepBlock(catalog store.ProfileCatalog, id, revision string) (stepblock.Definition, bool) {
	for _, def := range stepLibraryDefinitions(catalog, revision == "") {
		if def.ID == id && (revision == "" || def.Revision == revision) {
			return def, true
		}
	}
	return stepblock.Definition{}, false
}

func stepBlockUsages(catalog store.ProfileCatalog, def stepblock.Definition) []stepBlockUsage {
	names := map[string]string{}
	for _, workflow := range catalog.Workflows {
		names[workflow.ID] = firstNonEmpty(workflow.DisplayName, workflow.ID)
	}
	seen := map[string]bool{}
	usages := []stepBlockUsage{}
	for _, config := range catalog.TemplateConfigs {
		if config.ScopeType != "step" {
			continue
		}
		meta := mapFromAny(jsonObject(config.ConfigJSON)["workflowStudio"])
		group := mapFromAny(meta["group"])
		key := config.WorkflowID + ":" + valueString(group["id"])
		if valueString(group["blockId"]) != def.ID || seen[key] {
			continue
		}
		seen[key] = true
		revision := valueString(group["revision"])
		usages = append(usages, stepBlockUsage{WorkflowID: config.WorkflowID, WorkflowName: names[config.WorkflowID], InstanceID: valueString(group["id"]), Revision: revision, Outdated: revision != def.Revision})
	}
	slices.SortFunc(usages, func(a, b stepBlockUsage) int {
		return strings.Compare(a.WorkflowID+a.InstanceID, b.WorkflowID+b.InstanceID)
	})
	return usages
}

func safeStepBlock(def stepblock.Definition) stepblock.Definition {
	raw, _ := json.Marshal(def)
	var safe stepblock.Definition
	_ = json.Unmarshal(raw, &safe)
	for index := range safe.Parameters {
		p := &safe.Parameters[index]
		p.DefaultValue = safeStepParameterValue(p.Name, p.DefaultValue)
	}
	for index := range safe.Steps {
		safe.Steps[index].Values = safeStepParameterValues(safe.Steps[index].Values)
	}
	return safe
}

func safeStepParameterValue(name string, value any) any {
	key := strings.ToLower(strings.ReplaceAll(name, "_", ""))
	if strings.Contains(key, "apikey") || strings.Contains(key, "credential") {
		return redaction.Mask
	}
	return mapFromAny(redaction.Value(map[string]any{name: value}))[name]
}

func safeStepParameterValues(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	result := map[string]any{}
	for name, value := range values {
		result[name] = safeStepParameterValue(name, value)
	}
	return result
}

func writeStepLibrarySnapshot(w http.ResponseWriter, snapshot store.ProfileCatalogSnapshot) {
	blocks := []stepBlockRecord{}
	for _, def := range stepLibraryDefinitions(snapshot.Catalog, true) {
		blocks = append(blocks, stepBlockRecord{Definition: safeStepBlock(def), Usages: stepBlockUsages(snapshot.Catalog, def)})
	}
	slices.SortFunc(blocks, func(a, b stepBlockRecord) int {
		return strings.Compare(a.Definition.DisplayName, b.Definition.DisplayName)
	})
	w.Header().Set("ETag", caseCatalogETag(snapshot))
	writeJSON(w, map[string]any{"ok": true, "profileId": snapshot.Catalog.ProfileID, "revision": snapshot.Revision, "blocks": blocks})
}

func handleStepLibraryMutation(w http.ResponseWriter, r *http.Request, runtime store.Store) {
	request, ok := prepareCaseCatalogMutation(w, r, runtime)
	if !ok {
		return
	}
	raw, err := json.Marshal(request.Current.Catalog)
	var catalog store.ProfileCatalog
	if err != nil || json.Unmarshal(raw, &catalog) != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "catalog could not be copied"})
		return
	}
	operation := "step-library-save"
	var def stepblock.Definition
	if r.Method == http.MethodDelete {
		id := valueString(request.Payload["id"])
		var exists bool
		def, exists = findStepBlock(catalog, id, "")
		if !exists {
			writeJSONStatus(w, http.StatusNotFound, map[string]any{"ok": false, "error": "step block not found"})
			return
		}
		usages := stepBlockUsages(catalog, def)
		if len(usages) > 0 {
			writeJSONStatus(w, http.StatusConflict, map[string]any{"ok": false, "error": "step block is still referenced by workflows", "usages": usages})
			return
		}
		catalog.TemplateConfigs = slices.DeleteFunc(catalog.TemplateConfigs, func(c store.CatalogTemplateConfig) bool { return c.ScopeType == stepLibraryScope && c.ScopeID == id })
		operation = "step-library-delete"
	} else {
		raw, _ = json.Marshal(request.Payload["definition"])
		if err = json.Unmarshal(raw, &def); err == nil {
			def, err = prepareStepBlockDefinition(catalog, def)
		}
		if err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		for index := range catalog.TemplateConfigs {
			config := &catalog.TemplateConfigs[index]
			if config.ScopeType == stepLibraryScope && config.ScopeID == def.ID {
				config.Status = "archived"
			}
		}
		id := "step-block." + workflowStudioDigest([]string{def.ID, def.Revision})[:32]
		raw, _ = json.Marshal(def)
		config := store.CatalogTemplateConfig{ID: id, ScopeType: stepLibraryScope, ScopeID: def.ID, Title: def.DisplayName, Description: def.Description, Status: "active", ConfigJSON: string(raw)}
		index := slices.IndexFunc(catalog.TemplateConfigs, func(c store.CatalogTemplateConfig) bool { return c.ID == id })
		if index < 0 {
			catalog.TemplateConfigs = append(catalog.TemplateConfigs, config)
		} else {
			catalog.TemplateConfigs[index] = config
		}
	}
	summary, _ := json.Marshal(map[string]any{"blockId": def.ID, "blockRevision": def.Revision})
	updated, err := request.Store.CompareAndSwapProfileCatalog(r.Context(), request.ExpectedRevision, catalog, store.ProfileCatalogMutation{Operation: operation, SummaryJSON: string(summary)})
	if err != nil {
		writeCaseCatalogMutationError(w, request.ExpectedRevision, err)
		return
	}
	writeStepLibrarySnapshot(w, updated)
}

func prepareStepBlockDefinition(catalog store.ProfileCatalog, def stepblock.Definition) (stepblock.Definition, error) {
	if old, exists := findStepBlock(catalog, def.ID, ""); exists {
		for index := range def.Parameters {
			p := &def.Parameters[index]
			if p.DefaultValue != redaction.Mask {
				continue
			}
			for _, previous := range old.Parameters {
				if previous.Name == p.Name {
					p.DefaultValue = previous.DefaultValue
				}
			}
		}
	}
	canonical, err := stepblock.Canonical(def)
	if err != nil {
		return def, err
	}
	cases := map[string]store.CatalogAPICase{}
	for _, item := range catalog.APICases {
		cases[item.ID] = item
	}
	draft := workflowStudioDraft{Workflow: workflowStudioDefinition{ID: "step-library.validation", DisplayName: def.DisplayName}, Steps: canonical.Steps}
	docs, indices, err := prepareWorkflowStudioSteps(catalog, draft, cases)
	if err == nil {
		err = applyWorkflowStudioBindings(draft, docs, indices)
	}
	if err != nil {
		return def, err
	}
	for _, parameter := range canonical.Parameters {
		for _, target := range parameter.Targets {
			index := indices[target.StepID]
			variables := workflowStudioVariables(mapFromAny(mapFromAny(docs[index]["workflowStudio"])["baseExecution"]))
			if !slices.Contains(variables, target.Parameter) {
				return def, fmt.Errorf("parameter %s targets an unknown request parameter", parameter.Name)
			}
			if slices.ContainsFunc(canonical.Steps[index].Bindings, func(b workflowStudioBinding) bool { return b.Parameter == target.Parameter }) {
				return def, fmt.Errorf("parameter %s targets a field already bound inside the block", parameter.Name)
			}
		}
	}
	return canonical, nil
}
