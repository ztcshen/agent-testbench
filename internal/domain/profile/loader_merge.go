package profile

import "strings"

func mergeServices(groups ...[]Service) []Service {
	return mergeProfileAssets(groups, func(item Service) string { return item.ID }, mergeService)
}

func mergeService(base Service, next Service) Service {
	base = mergeServiceIdentity(base, next)
	base = mergeServiceSource(base, next)
	base = mergeServiceRuntime(base, next)
	return mergeServiceHealthAndOrder(base, next)
}

func mergeServiceIdentity(base Service, next Service) Service {
	if next.ID != "" {
		base.ID = next.ID
	}
	if next.DisplayName != "" {
		base.DisplayName = next.DisplayName
	}
	if next.Kind != "" {
		base.Kind = next.Kind
	}
	if len(next.AttachedTemplateIDs) > 0 {
		base.AttachedTemplateIDs = next.AttachedTemplateIDs
	}
	return base
}

func mergeServiceSource(base Service, next Service) Service {
	if next.GitURL != "" {
		base.GitURL = next.GitURL
	}
	if next.GitBranch != "" {
		base.GitBranch = next.GitBranch
	}
	if next.RepoEnv != "" {
		base.RepoEnv = next.RepoEnv
	}
	if next.SourcePath != "" {
		base.SourcePath = next.SourcePath
	}
	return base
}

func mergeServiceRuntime(base Service, next Service) Service {
	if next.ContainerName != "" {
		base.ContainerName = next.ContainerName
	}
	if next.Image != "" {
		base.Image = next.Image
	}
	if next.DockerService != "" {
		base.DockerService = next.DockerService
	}
	if next.ServicePort != 0 {
		base.ServicePort = next.ServicePort
	}
	if next.ManagementPort != 0 {
		base.ManagementPort = next.ManagementPort
	}
	if next.MemoryMb != 0 {
		base.MemoryMb = next.MemoryMb
	}
	if next.CPUMilli != 0 {
		base.CPUMilli = next.CPUMilli
	}
	return base
}

func mergeServiceHealthAndOrder(base Service, next Service) Service {
	if next.StartupCommand != "" {
		base.StartupCommand = next.StartupCommand
	}
	if next.HealthURL != "" {
		base.HealthURL = next.HealthURL
	}
	if next.LogPath != "" {
		base.LogPath = next.LogPath
	}
	if next.Status != "" {
		base.Status = next.Status
	}
	if next.SortOrder != 0 {
		base.SortOrder = next.SortOrder
	}
	return base
}

func mergeAPICases(groups ...[]APICase) []APICase {
	return mergeProfileAssets(groups, func(item APICase) string { return item.ID }, mergeAPICase)
}

func mergeAPICase(base APICase, next APICase) APICase {
	base = mergeAPICaseIdentity(base, next)
	base = mergeAPICaseRequest(base, next)
	base = mergeAPICaseAdmission(base, next)
	return mergeAPICaseExecution(base, next)
}

func mergeAPICaseIdentity(base APICase, next APICase) APICase {
	if next.ID != "" {
		base.ID = next.ID
	}
	if next.DisplayName != "" {
		base.DisplayName = next.DisplayName
	}
	if next.Description != "" {
		base.Description = next.Description
	}
	if next.NodeID != "" {
		base.NodeID = next.NodeID
	}
	if next.CaseType != "" {
		base.CaseType = next.CaseType
	}
	if next.Scenario != "" {
		base.Scenario = next.Scenario
	}
	if next.Tags != nil {
		base.Tags = next.Tags
	}
	return base
}

func mergeAPICaseRequest(base APICase, next APICase) APICase {
	if next.PayloadTemplateJSON != "" {
		base.PayloadTemplateJSON = next.PayloadTemplateJSON
	}
	if next.RequestTemplateID != "" {
		base.RequestTemplateID = next.RequestTemplateID
	}
	if next.PatchJSON != "" {
		base.PatchJSON = next.PatchJSON
	}
	if next.RenderMode != "" {
		base.RenderMode = next.RenderMode
	}
	if next.ExpectedJSON != "" {
		base.ExpectedJSON = next.ExpectedJSON
	}
	return base
}

func mergeAPICaseAdmission(base APICase, next APICase) APICase {
	if next.Priority != "" {
		base.Priority = next.Priority
	}
	if next.Owner != "" {
		base.Owner = next.Owner
	}
	if next.requiredForAdmissionSet {
		base.RequiredForAdmission = next.RequiredForAdmission
	}
	if next.Status != "" {
		base.Status = next.Status
	}
	if next.SortOrder != 0 {
		base.SortOrder = next.SortOrder
	}
	return base
}

func mergeAPICaseExecution(base APICase, next APICase) APICase {
	if next.CasePath != "" {
		base.CasePath = next.CasePath
	}
	if next.BaseURL != "" {
		base.BaseURL = next.BaseURL
	}
	if next.EvidenceDir != "" {
		base.EvidenceDir = next.EvidenceDir
	}
	if next.TimeoutSeconds != 0 {
		base.TimeoutSeconds = next.TimeoutSeconds
	}
	if next.DefaultOverrides != nil {
		base.DefaultOverrides = next.DefaultOverrides
	}
	return base
}

func mergeProfileAssets[T any](groups [][]T, id func(T) string, merge func(T, T) T) []T {
	merged := map[string]T{}
	order := []string{}
	for _, group := range groups {
		for _, item := range group {
			value := strings.TrimSpace(id(item))
			if value == "" {
				continue
			}
			current, exists := merged[value]
			if !exists {
				order = append(order, value)
			}
			merged[value] = merge(current, item)
		}
	}
	out := make([]T, 0, len(order))
	for _, value := range order {
		out = append(out, merged[value])
	}
	return out
}
