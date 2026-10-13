package controlplane

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/redaction"
	"agent-testbench/internal/domain/stepblock"
	"agent-testbench/internal/store"
)

// References expand to ordinary workflow steps, keeping the existing runner,
// step identities and scoped data bindings as the execution contract.
func resolveWorkflowStepGroups(catalog store.ProfileCatalog, draft workflowStudioDraft) (workflowStudioDraft, error) {
	result := workflowStudioDraft{Workflow: draft.Workflow, Steps: []workflowStudioStep{}}
	seenGroups := map[string]bool{}
	for index := 0; index < len(draft.Steps); {
		step := draft.Steps[index]
		if step.Group == nil {
			result.Steps = append(result.Steps, step)
			index++
			continue
		}
		group := *step.Group
		if !stepblock.ValidID(group.ID) || len(group.ID) > 80 || seenGroups[group.ID] {
			return result, fmt.Errorf("step group requires a unique instance id")
		}
		def, exists := findStepBlock(catalog, group.BlockID, group.Revision)
		if !exists || group.Revision == "" {
			return result, fmt.Errorf("step group %s references an unavailable block revision", group.ID)
		}
		end := index
		for end < len(draft.Steps) && draft.Steps[end].Group != nil && draft.Steps[end].Group.ID == group.ID {
			end++
		}
		if end-index != len(def.Steps) {
			return result, fmt.Errorf("step group %s must include all of its steps", group.ID)
		}
		for offset, item := range draft.Steps[index:end] {
			if item.ID != group.ID+"."+def.Steps[offset].ID || item.CaseID != def.Steps[offset].CaseID || workflowStudioDigest(item.Group) != workflowStudioDigest(step.Group) {
				return result, fmt.Errorf("step group %s content differs from its referenced revision", group.ID)
			}
		}
		group, err := resolveStepGroupParameters(catalog, draft.Workflow.ID, def, group, result.Steps)
		if err != nil {
			return result, err
		}
		members, err := expandStepGroup(def, group)
		if err != nil {
			return result, err
		}
		for offset := range members {
			members[offset].Position = draft.Steps[index+offset].Position
		}
		result.Steps = append(result.Steps, members...)
		seenGroups[group.ID] = true
		index = end
	}
	return result, nil
}

func resolveStepGroupParameters(catalog store.ProfileCatalog, workflowID string, def stepblock.Definition, group stepblock.Instance, preceding []workflowStudioStep) (stepblock.Instance, error) {
	names := map[string]stepblock.Parameter{}
	values := map[string]any{}
	for _, p := range def.Parameters {
		names[p.Name] = p
		if p.HasDefault {
			values[p.Name] = p.DefaultValue
		}
	}
	previous := map[string]any{}
	for _, config := range catalog.TemplateConfigs {
		if config.ScopeType != "step" || config.WorkflowID != workflowID {
			continue
		}
		stored := mapFromAny(mapFromAny(jsonObject(config.ConfigJSON)["workflowStudio"])["group"])
		if valueString(stored["id"]) == group.ID && valueString(stored["blockId"]) == group.BlockID {
			previous = mapFromAny(stored["values"])
			break
		}
	}
	for name, value := range group.Values {
		if _, exists := names[name]; !exists {
			return group, fmt.Errorf("step group %s has unknown parameter %s", group.ID, name)
		}
		if value == redaction.Mask {
			if old, ok := previous[name]; ok {
				value = old
			}
		}
		values[name] = value
	}
	bound := map[string]bool{}
	for _, binding := range group.Bindings {
		if _, exists := names[binding.Parameter]; !exists || bound[binding.Parameter] {
			return group, fmt.Errorf("step group %s has an invalid parameter binding", group.ID)
		}
		if !slices.ContainsFunc(preceding, func(s workflowStudioStep) bool { return s.ID == binding.SourceStepID }) {
			return group, fmt.Errorf("step group %s data source must precede the whole group", group.ID)
		}
		if err := validateWorkflowStudioSource(binding.From, binding.Path); err != nil {
			return group, err
		}
		bound[binding.Parameter] = true
		delete(values, binding.Parameter)
	}
	for _, parameter := range def.Parameters {
		if bound[parameter.Name] {
			continue
		}
		value, exists := values[parameter.Name]
		if !exists {
			if parameter.Required {
				return group, fmt.Errorf("step group %s requires parameter %s", group.ID, parameter.Name)
			}
			continue
		}
		if err := stepblock.ValidateValue(parameter, value); err != nil {
			return group, err
		}
	}
	group.Title = firstNonEmpty(group.Title, def.DisplayName)
	group.Values, group.Parameters = values, def.Parameters
	return group, nil
}

func expandStepGroup(def stepblock.Definition, group stepblock.Instance) ([]workflowStudioStep, error) {
	raw, err := json.Marshal(def.Steps)
	var steps []workflowStudioStep
	if err != nil || json.Unmarshal(raw, &steps) != nil {
		return nil, fmt.Errorf("step group could not be copied")
	}
	indices := map[string]int{}
	for index := range steps {
		step := &steps[index]
		indices[step.ID] = index
		step.ID = group.ID + "." + step.ID
		step.Group = &group
		for i := range step.Bindings {
			step.Bindings[i].SourceStepID = group.ID + "." + step.Bindings[i].SourceStepID
		}
		for i := range step.Outputs {
			step.Outputs[i].Name = group.ID + "." + step.Outputs[i].Name
		}
		if step.Values == nil {
			step.Values = map[string]any{}
		}
		step.ValueContracts = map[string]stepblock.Parameter{}
	}
	bindings := map[string]workflowStudioBinding{}
	for _, binding := range group.Bindings {
		bindings[binding.Parameter] = binding
	}
	for _, parameter := range def.Parameters {
		for _, target := range parameter.Targets {
			step := &steps[indices[target.StepID]]
			step.ValueContracts[target.Parameter] = parameter
			if binding, exists := bindings[parameter.Name]; exists {
				binding.Parameter = target.Parameter
				step.Bindings = append(step.Bindings, binding)
			} else if value, exists := group.Values[parameter.Name]; exists {
				step.Values[target.Parameter] = value
			}
		}
	}
	return steps, nil
}

func applyWorkflowStudioValues(doc map[string]any, values map[string]any) {
	execution := mapFromAny(doc["caseExecution"])
	meta := mapFromAny(doc["workflowStudio"])
	overrides := mapFromAny(doc["overrides"])
	for parameter, value := range values {
		alias := "studio_value_" + workflowStudioDigest([]string{valueString(doc["caseId"]), parameter, workflowStudioDigest(values)})[:20]
		for _, key := range []string{"path", "query", "headers", "auth", "body", "traceEndpoint"} {
			execution[key] = replaceWorkflowStudioParameter(execution[key], parameter, alias)
		}
		overrides[alias] = value
		inputs := []any{}
		for _, input := range listFromAny(doc["inputs"]) {
			if valueString(mapFromAny(input)["name"]) != parameter {
				inputs = append(inputs, input)
			}
		}
		doc["inputs"] = inputs
	}
	doc["overrides"] = overrides
	meta["values"] = values
}

func validateStepGroupRuntimeParameters(doc map[string]any, overrides map[string]any) error {
	meta := mapFromAny(doc["workflowStudio"])
	for alias, raw := range mapFromAny(meta["parameterContracts"]) {
		data, _ := json.Marshal(raw)
		var parameter stepblock.Parameter
		if json.Unmarshal(data, &parameter) != nil {
			return fmt.Errorf("invalid stored parameter contract")
		}
		value, exists := overrides[alias]
		if !exists && !parameter.Required {
			continue
		}
		if !exists {
			return fmt.Errorf("missing step group parameter %s", parameter.Name)
		}
		converted, err := stepblock.CoerceBinding(parameter, value)
		if err != nil {
			return err
		}
		overrides[alias] = converted
	}
	return nil
}

func workflowStepDocumentFromBundle(bundleConfigs []profile.TemplateConfig, workflowID, stepID, caseID string) map[string]any {
	for _, config := range bundleConfigs {
		if config.ScopeType == "step" && config.WorkflowID == workflowID && config.ScopeID == stepID {
			doc := jsonObject(config.ConfigJSON)
			if valueString(doc["caseId"]) == caseID {
				return doc
			}
		}
	}
	return map[string]any{}
}

func safeWorkflowStudioGroup(group *stepblock.Instance) *stepblock.Instance {
	if group == nil {
		return nil
	}
	copy := *group
	copy.Values = safeStepParameterValues(group.Values)
	copy.Parameters = append([]stepblock.Parameter(nil), group.Parameters...)
	for index := range copy.Parameters {
		p := &copy.Parameters[index]
		p.DefaultValue = safeStepParameterValue(p.Name, p.DefaultValue)
	}
	return &copy
}

func studioParameterAlias(workflowID string, binding workflowStudioBinding) string {
	return "studio_" + workflowStudioDigest([]string{workflowID, binding.SourceStepID, binding.From, binding.Path})[:20]
}

func clearStudioValueOverrides(doc map[string]any) {
	for key := range mapFromAny(doc["overrides"]) {
		if strings.HasPrefix(key, "studio_value_") {
			delete(mapFromAny(doc["overrides"]), key)
		}
	}
}

func stepTemplateOverrides(template *caseExecutionTemplateConfig) map[string]any {
	if template == nil {
		return nil
	}
	return template.Overrides
}

func workflowStepGroupRunMetadata(configs []profile.TemplateConfig, workflowID string, plan apiCaseBatchCasePlan) map[string]any {
	doc := workflowStepDocumentFromBundle(configs, workflowID, plan.StepID, plan.ID)
	meta := mapFromAny(doc["workflowStudio"])
	group := mapFromAny(meta["group"])
	if valueString(group["id"]) == "" {
		return nil
	}
	values := mapFromAny(group["values"])
	for alias, raw := range mapFromAny(meta["parameterContracts"]) {
		data, _ := json.Marshal(raw)
		var parameter stepblock.Parameter
		if json.Unmarshal(data, &parameter) == nil {
			if value, exists := plan.Overrides[alias]; exists {
				values[parameter.Name] = value
			}
		}
	}
	return map[string]any{"stepGroup": map[string]any{"id": group["id"], "blockId": group["blockId"], "revision": group["revision"], "title": group["title"], "parameters": safeStepParameterValues(values)}}
}
