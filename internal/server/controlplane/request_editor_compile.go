package controlplane

import (
	"encoding/json"
	"fmt"
	"strings"

	"agent-testbench/internal/domain/casemaintenance"
	"agent-testbench/internal/domain/redaction"
	"agent-testbench/internal/store"
)

func compileRequestEditor(original store.ProfileCatalog, draft requestEditorRecord) (store.ProfileCatalog, error) {
	if err := validateRequestEditor(draft); err != nil {
		return store.ProfileCatalog{}, err
	}
	raw, err := json.Marshal(original)
	var catalog store.ProfileCatalog
	if err != nil || json.Unmarshal(raw, &catalog) != nil {
		return catalog, fmt.Errorf("catalog could not be copied")
	}
	if !catalogHasService(catalog.Services, draft.ServiceID) {
		return catalog, fmt.Errorf("select a registered HTTP service")
	}
	var existing store.CatalogAPICase
	found := false
	for _, item := range catalog.APICases {
		if item.ID == draft.ID {
			existing, found = item, true
		}
	}
	if found && (!requestEditorNative(existing) || existing.NodeID != draft.NodeID) {
		return catalog, fmt.Errorf("this request is managed by its external adapter or a different interface")
	}
	base, doc := workflowStudioBase(catalog, "", "", draft.ID)
	execution := cloneJSONObject(base)
	for _, key := range []string{"method", "path", "query", "headers", "body", "expectedHttpCodes", "expectedResponseContains", "expectedResponseNotContains"} {
		value, err := preserveRequestEditorMasks(draft.Execution[key], base[key])
		if err != nil {
			return catalog, err
		}
		execution[key] = value
	}
	execution["method"], execution["nodeId"] = strings.ToUpper(valueString(execution["method"])), draft.NodeID
	node, template, item, config, err := catalogInterfacePartsFromRegistration(sandboxInterfaceRegistrationRequest{
		ID: draft.NodeID, ServiceID: draft.ServiceID, DisplayName: draft.DisplayName, Description: draft.Description,
		Method: valueString(execution["method"]), Path: valueString(execution["path"]), Status: "active", CaseExecution: execution,
		Case: sandboxInterfaceCase{ID: draft.ID, DisplayName: draft.DisplayName, Description: draft.Description, Status: firstNonEmpty(draft.Status, "active"), TimeoutSeconds: draft.TimeoutSeconds},
	})
	if err != nil {
		return catalog, err
	}
	if found {
		item = existing
		item.DisplayName, item.Description, item.TimeoutSeconds = draft.DisplayName, draft.Description, draft.TimeoutSeconds
		for _, savedNode := range catalog.InterfaceNodes {
			if savedNode.ID == node.ID {
				savedNode.ServiceID = draft.ServiceID
				savedNode.Method, savedNode.Path, savedNode.DisplayName, savedNode.Description = node.Method, node.Path, node.DisplayName, node.Description
				node = savedNode
			}
		}
	} else {
		catalog.RequestTemplates = upsertCatalogRequestTemplate(catalog.RequestTemplates, template)
	}
	item.BaseURL = draft.BaseURL
	if !found && item.BaseURL == "" {
		return catalog, fmt.Errorf("new requests need an HTTP target URL")
	}
	catalog.InterfaceNodes = upsertCatalogInterfaceNode(catalog.InterfaceNodes, node)
	catalog.APICases = upsertCatalogAPICase(catalog.APICases, item)
	doc = cloneJSONObject(doc)
	doc["caseId"], doc["caseExecution"] = draft.ID, execution
	configRaw, _ := json.Marshal(doc)
	config.ConfigJSON = string(configRaw)
	index := -1
	for i, saved := range catalog.TemplateConfigs {
		if saved.ScopeType != "step" && saved.ScopeID == draft.ID && casemaintenance.ExecutionConfigTargetsCase(saved.ScopeType, saved.ScopeID, saved.ConfigJSON, draft.ID) {
			config = saved
			config.ConfigJSON = string(configRaw)
			index = i
			break
		}
	}
	if index >= 0 {
		catalog.TemplateConfigs[index] = config
	} else {
		config.ID = "config.editor." + draft.ID
		catalog.TemplateConfigs = append(catalog.TemplateConfigs, config)
	}
	return catalog, nil
}

func preserveRequestEditorMasks(next any, previous any) (any, error) {
	if text, ok := next.(string); ok && text == redaction.Mask {
		if previous == nil {
			return nil, fmt.Errorf("a masked field has no saved value; enter the real value or remove it")
		}
		return previous, nil
	}
	if text, ok := next.(string); ok && strings.Contains(text, redaction.Mask) {
		var nextJSON, previousJSON any
		if json.Unmarshal([]byte(text), &nextJSON) == nil && json.Unmarshal([]byte(valueString(previous)), &previousJSON) == nil {
			switch nextJSON.(type) {
			case map[string]any, []any:
				preserved, err := preserveRequestEditorMasks(nextJSON, previousJSON)
				if err != nil {
					return nil, err
				}
				raw, _ := json.Marshal(preserved)
				return string(raw), nil
			}
		}
		return nil, fmt.Errorf("masked text cannot replace a saved credential; enter a new value or remove the field")
	}
	switch value := next.(type) {
	case map[string]any:
		result := map[string]any{}
		for key, item := range value {
			preserved, err := preserveRequestEditorMasks(item, mapFromAny(previous)[key])
			if err != nil {
				return nil, err
			}
			result[key] = preserved
		}
		return result, nil
	case []any:
		result := make([]any, len(value))
		old := listFromAny(previous)
		for index, item := range value {
			var saved any
			if index < len(old) {
				saved = old[index]
			}
			preserved, err := preserveRequestEditorMasks(item, saved)
			if err != nil {
				return nil, err
			}
			result[index] = preserved
		}
		return result, nil
	default:
		return next, nil
	}
}
