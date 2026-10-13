package controlplane

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"agent-testbench/internal/store"
)

var workflowStudioToken = regexp.MustCompile(`\{\{override:([^{}|]+)(?:\|[^{}]*)?\}\}`)

func workflowStudioVariables(execution map[string]any) []string {
	seen := map[string]bool{}
	for _, key := range []string{"path", "query", "headers", "auth", "body", "traceEndpoint"} {
		raw, _ := json.Marshal(execution[key])
		for _, match := range workflowStudioToken.FindAllStringSubmatch(string(raw), -1) {
			seen[strings.TrimSpace(match[1])] = true
		}
	}
	values := make([]string, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	slices.Sort(values)
	return values
}

func compileWorkflowStudio(original store.ProfileCatalog, draft workflowStudioDraft) (store.ProfileCatalog, error) {
	var err error
	draft, err = resolveWorkflowStepGroups(original, draft)
	if err != nil {
		return store.ProfileCatalog{}, err
	}
	if strings.TrimSpace(draft.Workflow.ID) == "" || strings.TrimSpace(draft.Workflow.DisplayName) == "" {
		return store.ProfileCatalog{}, fmt.Errorf("workflow id and display name are required")
	}
	// Copy through the catalog representation so failed edits cannot mutate the snapshot.
	raw, err := json.Marshal(original)
	var catalog store.ProfileCatalog
	if err != nil || json.Unmarshal(raw, &catalog) != nil {
		return catalog, fmt.Errorf("catalog could not be copied")
	}
	cases := map[string]store.CatalogAPICase{}
	for _, item := range catalog.APICases {
		cases[item.ID] = item
	}
	docs, indices, err := prepareWorkflowStudioSteps(original, draft, cases)
	if err != nil {
		return catalog, err
	}
	if err := applyWorkflowStudioBindings(draft, docs, indices); err != nil {
		return catalog, err
	}
	workflow := store.CatalogWorkflow{ID: draft.Workflow.ID, DisplayName: draft.Workflow.DisplayName, Description: draft.Workflow.Description}
	updated := false
	for index, item := range catalog.Workflows {
		if item.ID == workflow.ID {
			workflow.BaseStepTimeoutMs, workflow.TimeoutOffsetMs = item.BaseStepTimeoutMs, item.TimeoutOffsetMs
			catalog.Workflows[index], updated = workflow, true
		}
	}
	if !updated {
		catalog.Workflows = append(catalog.Workflows, workflow)
	}
	catalog.WorkflowBindings = slices.DeleteFunc(catalog.WorkflowBindings, func(binding store.CatalogWorkflowBinding) bool { return binding.WorkflowID == workflow.ID })
	remaining := map[string]bool{}
	for index, step := range draft.Steps {
		remaining[step.ID] = true
		catalog.WorkflowBindings = append(catalog.WorkflowBindings, store.CatalogWorkflowBinding{WorkflowID: workflow.ID, StepID: step.ID, CaseID: step.CaseID, NodeID: cases[step.CaseID].NodeID, Required: step.Required, SortOrder: index + 1})
		meta := mapFromAny(docs[index]["workflowStudio"])
		applyWorkflowStudioValues(docs[index], step.Values)
		meta["compiledDigest"] = workflowStudioDigest(docs[index]["caseExecution"])
		configRaw, _ := json.Marshal(docs[index])
		_, oldIndex := workflowStudioScopedDocument(catalog, workflow.ID, step.ID, step.CaseID)
		config := store.CatalogTemplateConfig{ID: "workflow-studio." + workflowStudioDigest([]string{workflow.ID, step.ID})[:24], ScopeType: "step", ScopeID: step.ID, WorkflowID: workflow.ID, NodeID: cases[step.CaseID].NodeID, Status: "active", Title: firstNonEmpty(step.Title, step.ID), SortOrder: index + 1, ConfigJSON: string(configRaw)}
		if oldIndex >= 0 {
			config.ID, config.TemplateID, config.Description = catalog.TemplateConfigs[oldIndex].ID, catalog.TemplateConfigs[oldIndex].TemplateID, catalog.TemplateConfigs[oldIndex].Description
			catalog.TemplateConfigs[oldIndex] = config
		} else {
			catalog.TemplateConfigs = append(catalog.TemplateConfigs, config)
		}
	}
	catalog.TemplateConfigs = slices.DeleteFunc(catalog.TemplateConfigs, func(config store.CatalogTemplateConfig) bool {
		return config.WorkflowID == workflow.ID && config.ScopeType == "step" && !remaining[config.ScopeID]
	})
	return catalog, nil
}

func prepareWorkflowStudioSteps(original store.ProfileCatalog, draft workflowStudioDraft, cases map[string]store.CatalogAPICase) ([]map[string]any, map[string]int, error) {
	indices := map[string]int{}
	docs := make([]map[string]any, len(draft.Steps))
	for index, step := range draft.Steps {
		if strings.TrimSpace(step.ID) == "" {
			return nil, nil, fmt.Errorf("step id is required")
		}
		if _, exists := indices[step.ID]; exists {
			return nil, nil, workflowStudioError(step.ID, "duplicate step id")
		}
		indices[step.ID] = index
		item, exists := cases[step.CaseID]
		if !exists || item.NodeID == "" {
			return nil, nil, workflowStudioError(step.ID, "case must reference a catalog interface")
		}
		execution, doc := workflowStudioBase(original, draft.Workflow.ID, step.ID, step.CaseID)
		meta := mapFromAny(doc["workflowStudio"])
		if digest := valueString(meta["compiledDigest"]); digest != "" && digest != workflowStudioDigest(doc["caseExecution"]) {
			return nil, nil, workflowStudioError(step.ID, "request configuration changed outside this editor; review the step configuration first")
		}
		baseRaw, _ := json.Marshal(execution)
		base := jsonObject(string(baseRaw))
		variables := workflowStudioVariables(base)
		for name := range step.Values {
			if !slices.Contains(variables, name) || slices.ContainsFunc(step.Bindings, func(b workflowStudioBinding) bool { return b.Parameter == name }) {
				return nil, nil, workflowStudioError(step.ID, "fixed values must select an unbound template parameter")
			}
		}
		doc["caseId"] = step.CaseID
		doc["title"] = firstNonEmpty(step.Title, item.DisplayName, step.ID)
		doc["caseExecution"] = execution
		clearStudioValueOverrides(doc)
		baseInputs := listFromAny(doc["inputs"])
		if saved, exists := meta["baseInputs"]; exists {
			baseInputs = listFromAny(saved)
		}
		doc["inputs"] = baseInputs
		managed := stringSetFromAny(meta["managedExports"])
		previousExports := map[string]map[string]any{}
		for _, value := range listFromAny(doc["exports"]) {
			export := mapFromAny(value)
			if !managed[valueString(export["name"])] {
				previousExports[valueString(export["name"])] = export
			}
		}
		exports := []any{}
		seen := map[string]bool{}
		for _, output := range step.Outputs {
			if output.Name == "" || seen[output.Name] || strings.HasPrefix(output.Name, "studio_") {
				return nil, nil, workflowStudioError(step.ID, "output names must be unique and non-empty")
			}
			if err := validateWorkflowStudioSource(output.From, output.Path); err != nil {
				return nil, nil, workflowStudioError(step.ID, err.Error())
			}
			seen[output.Name] = true
			export := previousExports[output.Name]
			if export == nil {
				export = map[string]any{}
			}
			export["name"], export["from"], export["path"] = output.Name, output.From, output.Path
			exports = append(exports, export)
		}
		doc["exports"] = exports
		doc["workflowStudio"] = map[string]any{"position": step.Position, "bindings": step.Bindings, "baseExecution": base, "baseInputs": baseInputs, "managedExports": []any{}, "managedInputs": []any{}, "group": step.Group, "values": step.Values}
		docs[index] = doc
	}
	return docs, indices, nil
}

func applyWorkflowStudioBindings(draft workflowStudioDraft, docs []map[string]any, indices map[string]int) error {
	for index, step := range draft.Steps {
		doc := docs[index]
		execution := mapFromAny(doc["caseExecution"])
		variables := workflowStudioVariables(execution)
		seen := map[string]bool{}
		for _, binding := range step.Bindings {
			source, exists := indices[binding.SourceStepID]
			if !exists || source >= index {
				return workflowStudioError(step.ID, "source step must precede its consumer")
			}
			if !slices.Contains(variables, binding.Parameter) || seen[binding.Parameter] {
				return workflowStudioError(step.ID, "binding must select one existing template parameter")
			}
			if len(mapFromAny(docs[source]["caseExecution"])) == 0 {
				return workflowStudioError(step.ID, "source case needs a Store-native request configuration")
			}
			if err := validateWorkflowStudioSource(binding.From, binding.Path); err != nil {
				return workflowStudioError(step.ID, err.Error())
			}
			seen[binding.Parameter] = true
			alias := studioParameterAlias(draft.Workflow.ID, binding)
			sourceMeta := mapFromAny(docs[source]["workflowStudio"])
			if !stringSetFromAny(sourceMeta["managedExports"])[alias] {
				docs[source]["exports"] = append(listFromAny(docs[source]["exports"]), map[string]any{"name": alias, "from": binding.From, "path": binding.Path})
				sourceMeta["managedExports"] = append(listFromAny(sourceMeta["managedExports"]), alias)
			}
			doc["inputs"] = append(listFromAny(doc["inputs"]), map[string]any{"name": alias, "source": "previous", "required": true})
			meta := mapFromAny(doc["workflowStudio"])
			meta["managedInputs"] = append(listFromAny(meta["managedInputs"]), alias)
			if contract, exists := step.ValueContracts[binding.Parameter]; exists {
				contracts := mapFromAny(meta["parameterContracts"])
				contracts[alias] = contract
				meta["parameterContracts"] = contracts
			}
			for _, key := range []string{"path", "query", "headers", "auth", "body", "traceEndpoint"} {
				execution[key] = replaceWorkflowStudioParameter(execution[key], binding.Parameter, alias)
			}
			// The original parameter's requirement is fulfilled by the selected export.
			inputs := []any{}
			for _, input := range listFromAny(doc["inputs"]) {
				if valueString(mapFromAny(input)["name"]) != binding.Parameter {
					inputs = append(inputs, input)
				}
			}
			doc["inputs"] = inputs
		}
	}
	return nil
}

func validateWorkflowStudioSource(from, path string) error {
	if from != "responsebody" && from != "requestbody" {
		return fmt.Errorf("select requestbody or responsebody as the data source")
	}
	if strings.TrimSpace(path) == "" || strings.ContainsAny(path, "[]*{}") {
		return fmt.Errorf("use a dot path such as $.data.id or $.items.0.id")
	}
	return nil
}

func workflowStudioDigest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func replaceWorkflowStudioParameter(value any, parameter, alias string) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = replaceWorkflowStudioParameter(item, parameter, alias)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = replaceWorkflowStudioParameter(item, parameter, alias)
		}
		return out
	case string:
		return workflowStudioToken.ReplaceAllStringFunc(typed, func(match string) string {
			parts := workflowStudioToken.FindStringSubmatch(match)
			if strings.TrimSpace(parts[1]) == parameter {
				return "{{override:" + alias + "}}"
			}
			return match
		})
	default:
		return value
	}
}
