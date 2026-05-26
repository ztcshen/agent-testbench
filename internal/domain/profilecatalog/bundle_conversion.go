// Package profilecatalog converts profile bundles into catalog read models and back.
package profilecatalog

import (
	domaincatalog "agent-testbench/internal/domain/catalog"
	"agent-testbench/internal/domain/profile"
)

func catalogServicesFromProfile(items []profile.Service, runtimeEnv map[string]string) []domaincatalog.Service {
	out := make([]domaincatalog.Service, 0, len(items))
	for _, item := range items {
		out = append(out, catalogServiceFromProfile(item, runtimeEnv))
	}
	return out
}

func catalogWorkflowsFromProfile(items []profile.Workflow) []domaincatalog.Workflow {
	out := make([]domaincatalog.Workflow, 0, len(items))
	for _, item := range items {
		out = append(out, domaincatalog.Workflow{
			ID:                item.ID,
			DisplayName:       item.DisplayName,
			Description:       item.Description,
			BaseStepTimeoutMs: item.BaseStepTimeoutMs,
			TimeoutOffsetMs:   item.TimeoutOffsetMs,
		})
	}
	return out
}

func catalogInterfaceNodesFromProfile(items []profile.InterfaceNode) []domaincatalog.InterfaceNode {
	out := make([]domaincatalog.InterfaceNode, 0, len(items))
	for _, item := range items {
		out = append(out, catalogInterfaceNodeFromProfile(item))
	}
	return out
}

func catalogInterfaceNodeFromProfile(item profile.InterfaceNode) domaincatalog.InterfaceNode {
	var out domaincatalog.InterfaceNode
	copySharedFields(&out, item, interfaceNodeSharedFields)
	return out
}

func catalogAPICasesFromProfile(items []profile.APICase) []domaincatalog.APICase {
	out := make([]domaincatalog.APICase, 0, len(items))
	for _, item := range items {
		out = append(out, catalogAPICaseFromProfile(item))
	}
	return out
}

func catalogAPICaseFromProfile(item profile.APICase) domaincatalog.APICase {
	var out domaincatalog.APICase
	copySharedFields(&out, item, apiCaseSharedFields)
	out.DefaultOverridesJSON = jsonStringMap(item.DefaultOverrides)
	return out
}

func catalogRequestTemplatesFromProfile(items []profile.RequestTemplate) []domaincatalog.RequestTemplate {
	out := make([]domaincatalog.RequestTemplate, 0, len(items))
	for _, item := range items {
		out = append(out, domaincatalog.RequestTemplate{
			ID:           item.ID,
			DisplayName:  item.DisplayName,
			NodeID:       item.NodeID,
			Method:       item.Method,
			Path:         item.Path,
			TemplateJSON: item.TemplateJSON,
		})
	}
	return out
}

func catalogWorkflowBindingsFromProfile(items []profile.WorkflowBinding) []domaincatalog.WorkflowBinding {
	out := make([]domaincatalog.WorkflowBinding, 0, len(items))
	for _, item := range items {
		out = append(out, domaincatalog.WorkflowBinding{
			WorkflowID: item.WorkflowID,
			StepID:     item.StepID,
			NodeID:     item.NodeID,
			CaseID:     item.CaseID,
			Required:   item.Required,
			SortOrder:  item.SortOrder,
		})
	}
	return out
}

func catalogCaseDependenciesFromProfile(items []profile.CaseDependency) []domaincatalog.CaseDependency {
	out := make([]domaincatalog.CaseDependency, 0, len(items))
	for _, item := range items {
		out = append(out, domaincatalog.CaseDependency{
			ID:           item.ID,
			CaseID:       item.CaseID,
			FixtureID:    item.FixtureID,
			MappingsJSON: item.MappingsJSON,
		})
	}
	return out
}

func catalogFixturesFromProfile(items []profile.Fixture) []domaincatalog.Fixture {
	out := make([]domaincatalog.Fixture, 0, len(items))
	for _, item := range items {
		out = append(out, domaincatalog.Fixture{
			ID:          item.ID,
			DisplayName: item.DisplayName,
			Kind:        item.Kind,
			DataJSON:    item.DataJSON,
		})
	}
	return out
}

func catalogTemplateConfigsFromProfile(items []profile.TemplateConfig) []domaincatalog.TemplateConfig {
	out := make([]domaincatalog.TemplateConfig, 0, len(items))
	for _, item := range items {
		out = append(out, catalogTemplateConfigFromProfile(item))
	}
	return out
}

func catalogTemplateConfigFromProfile(item profile.TemplateConfig) domaincatalog.TemplateConfig {
	var out domaincatalog.TemplateConfig
	copySharedFields(&out, item, templateConfigSharedFields)
	return out
}

func profileServicesFromCatalog(items []domaincatalog.Service) []profile.Service {
	out := make([]profile.Service, 0, len(items))
	for _, item := range items {
		out = append(out, profileServiceFromCatalog(item))
	}
	return out
}

func profileWorkflowsFromCatalog(items []domaincatalog.Workflow) []profile.Workflow {
	out := make([]profile.Workflow, 0, len(items))
	for _, item := range items {
		out = append(out, profile.Workflow{
			ID:                item.ID,
			DisplayName:       item.DisplayName,
			Description:       item.Description,
			BaseStepTimeoutMs: item.BaseStepTimeoutMs,
			TimeoutOffsetMs:   item.TimeoutOffsetMs,
		})
	}
	return out
}

func profileInterfaceNodesFromCatalog(items []domaincatalog.InterfaceNode) []profile.InterfaceNode {
	out := make([]profile.InterfaceNode, 0, len(items))
	for _, item := range items {
		out = append(out, profileInterfaceNodeFromCatalog(item))
	}
	return out
}

func profileInterfaceNodeFromCatalog(item domaincatalog.InterfaceNode) profile.InterfaceNode {
	var out profile.InterfaceNode
	copySharedFields(&out, item, interfaceNodeSharedFields)
	return out
}

func profileAPICasesFromCatalog(items []domaincatalog.APICase) []profile.APICase {
	out := make([]profile.APICase, 0, len(items))
	for _, item := range items {
		out = append(out, profileAPICaseFromCatalog(item))
	}
	return out
}

func profileAPICaseFromCatalog(item domaincatalog.APICase) profile.APICase {
	var out profile.APICase
	copySharedFields(&out, item, apiCaseSharedFields)
	out.DefaultOverrides = jsonMap(item.DefaultOverridesJSON)
	return out
}

func profileRequestTemplatesFromCatalog(items []domaincatalog.RequestTemplate) []profile.RequestTemplate {
	out := make([]profile.RequestTemplate, 0, len(items))
	for _, item := range items {
		out = append(out, profile.RequestTemplate{
			ID:           item.ID,
			DisplayName:  item.DisplayName,
			NodeID:       item.NodeID,
			Method:       item.Method,
			Path:         item.Path,
			TemplateJSON: item.TemplateJSON,
		})
	}
	return out
}

func profileWorkflowBindingsFromCatalog(items []domaincatalog.WorkflowBinding) []profile.WorkflowBinding {
	out := make([]profile.WorkflowBinding, 0, len(items))
	for _, item := range items {
		out = append(out, profile.WorkflowBinding{
			WorkflowID: item.WorkflowID,
			StepID:     item.StepID,
			NodeID:     item.NodeID,
			CaseID:     item.CaseID,
			Required:   item.Required,
			SortOrder:  item.SortOrder,
		})
	}
	return out
}

func profileCaseDependenciesFromCatalog(items []domaincatalog.CaseDependency) []profile.CaseDependency {
	out := make([]profile.CaseDependency, 0, len(items))
	for _, item := range items {
		out = append(out, profile.CaseDependency{
			ID:           item.ID,
			CaseID:       item.CaseID,
			FixtureID:    item.FixtureID,
			MappingsJSON: item.MappingsJSON,
		})
	}
	return out
}

func profileFixturesFromCatalog(items []domaincatalog.Fixture) []profile.Fixture {
	out := make([]profile.Fixture, 0, len(items))
	for _, item := range items {
		out = append(out, profile.Fixture{
			ID:          item.ID,
			DisplayName: item.DisplayName,
			Kind:        item.Kind,
			DataJSON:    item.DataJSON,
		})
	}
	return out
}

func profileTemplateConfigsFromCatalog(items []domaincatalog.TemplateConfig) []profile.TemplateConfig {
	out := make([]profile.TemplateConfig, 0, len(items))
	for _, item := range items {
		out = append(out, profileTemplateConfigFromCatalog(item))
	}
	return out
}

func profileTemplateConfigFromCatalog(item domaincatalog.TemplateConfig) profile.TemplateConfig {
	var out profile.TemplateConfig
	copySharedFields(&out, item, templateConfigSharedFields)
	return out
}
