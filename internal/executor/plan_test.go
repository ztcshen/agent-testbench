package executor_test

import (
	"context"
	"testing"

	"open-test-sandbox/internal/executor"
	"open-test-sandbox/internal/profile"
)

func TestPlanValidatesExternalToolDescriptors(t *testing.T) {
	bundle := profile.Bundle{
		ID: "sample",
		Executors: []profile.ExecutorDescriptor{
			{ID: "executor.command", DisplayName: "No-op command", Kind: "custom-command", Command: "true", Status: "active", ArtifactPaths: []string{"reports/command.json"}},
			{ID: "executor.karate", DisplayName: "Karate API suite", Kind: "karate", SourcePath: "tests/api.feature", Status: "active"},
			{ID: "executor.pytest", DisplayName: "Pytest suite", Kind: "pytest", Status: "active"},
			{ID: "executor.unknown", DisplayName: "Unknown suite", Kind: "unknown", SourcePath: "tests/unknown", Status: "active"},
		},
	}

	report := executor.Plan(context.Background(), bundle)

	if report.OK || report.ProfileID != "sample" || report.Counts.Total != 4 || report.Counts.Ready != 2 || report.Counts.Blocked != 2 {
		t.Fatalf("plan summary = %#v", report)
	}
	byID := map[string]executor.PlanItem{}
	for _, item := range report.Items {
		byID[item.ID] = item
	}
	if !byID["executor.command"].Ready || byID["executor.command"].RunMode != "dry-run" || byID["executor.command"].Command != "true" {
		t.Fatalf("command executor = %#v", byID["executor.command"])
	}
	if !byID["executor.karate"].Ready || byID["executor.karate"].SourcePath != "tests/api.feature" {
		t.Fatalf("karate executor = %#v", byID["executor.karate"])
	}
	if byID["executor.pytest"].Ready || !containsIssue(byID["executor.pytest"].Issues, "missing-source-path") {
		t.Fatalf("pytest executor = %#v", byID["executor.pytest"])
	}
	if byID["executor.unknown"].Ready || !containsIssue(byID["executor.unknown"].Issues, "unsupported-kind") {
		t.Fatalf("unknown executor = %#v", byID["executor.unknown"])
	}
}

func containsIssue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
