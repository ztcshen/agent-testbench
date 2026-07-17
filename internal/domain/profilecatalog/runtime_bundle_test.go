package profilecatalog

import (
	"testing"

	domaincatalog "agent-testbench/internal/domain/catalog"
	"agent-testbench/internal/domain/profile"
)

func TestRuntimeBundlePreservesBootstrapOnlyRuntimeMetadata(t *testing.T) {
	bootstrap := profile.Bundle{
		ID:                "profile.alpha",
		DisplayName:       "Profile Alpha",
		Description:       "Runtime metadata",
		BaseDir:           "/profiles/alpha",
		RuntimeEnvFiles:   []string{"runtime.env"},
		Executors:         []profile.ExecutorDescriptor{{ID: "executor.karate", Kind: "karate"}},
		FailureCategories: []profile.FailureCategoryRule{{Name: "timeout", Category: "timeout"}},
		AgentTestProfiles: []profile.AgentTestProfile{{ID: "agent.alpha"}},
		ConfigAuthoring:   profile.ConfigAuthoring{SchemaVersion: "1"},
	}
	catalog := domaincatalog.ProfileCatalog{
		ProfileID: "profile.alpha",
		APICases:  []domaincatalog.APICase{{ID: "case.store", ExecutorID: "executor.karate"}},
	}

	current := RuntimeBundle(catalog, bootstrap)
	if current.DisplayName != bootstrap.DisplayName || current.Description != bootstrap.Description || current.BaseDir != bootstrap.BaseDir {
		t.Fatalf("runtime identity metadata = %#v", current)
	}
	if len(current.Executors) != 1 || current.Executors[0].ID != "executor.karate" || len(current.FailureCategories) != 1 || len(current.AgentTestProfiles) != 1 || current.ConfigAuthoring.SchemaVersion != "1" {
		t.Fatalf("runtime-only metadata was not preserved: %#v", current)
	}
	if len(current.APICases) != 1 || current.APICases[0].ID != "case.store" {
		t.Fatalf("Store catalog cases were not retained: %#v", current.APICases)
	}
}

func TestRuntimeBundleDoesNotMergeDifferentBootstrapProfile(t *testing.T) {
	current := RuntimeBundle(
		domaincatalog.ProfileCatalog{ProfileID: "profile.store"},
		profile.Bundle{ID: "profile.bootstrap", Executors: []profile.ExecutorDescriptor{{ID: "executor.bootstrap"}}},
	)
	if len(current.Executors) != 0 {
		t.Fatalf("different profile runtime metadata leaked into Store snapshot: %#v", current.Executors)
	}
}
