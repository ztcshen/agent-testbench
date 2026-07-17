package profilecatalog

import (
	"strings"

	domaincatalog "agent-testbench/internal/domain/catalog"
	"agent-testbench/internal/domain/profile"
)

// RuntimeBundle converts the Store catalog into the active request snapshot.
// Fields that are intentionally not persisted in the catalog remain available
// from the bootstrap bundle when both snapshots describe the same profile.
func RuntimeBundle(catalog domaincatalog.ProfileCatalog, bootstrap profile.Bundle) profile.Bundle {
	current := ToBundle(catalog)
	if strings.TrimSpace(current.ID) == "" || strings.TrimSpace(current.ID) != strings.TrimSpace(bootstrap.ID) {
		return current
	}
	current.DisplayName = firstNonEmptyRuntimeValue(bootstrap.DisplayName, current.DisplayName)
	current.Description = bootstrap.Description
	current.BaseDir = bootstrap.BaseDir
	current.RuntimeEnvFiles = append([]string(nil), bootstrap.RuntimeEnvFiles...)
	current.Executors = append([]profile.ExecutorDescriptor(nil), bootstrap.Executors...)
	current.FailureCategories = append([]profile.FailureCategoryRule(nil), bootstrap.FailureCategories...)
	current.AgentTestProfiles = append([]profile.AgentTestProfile(nil), bootstrap.AgentTestProfiles...)
	current.ConfigAuthoring = bootstrap.ConfigAuthoring
	return current
}

func firstNonEmptyRuntimeValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
