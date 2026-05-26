package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkflowPlanCommandPrintsBoundSteps(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowProfile(t, dir)
	configureNamedPostgreSQLActiveStore(t, "daily-workflow-plan-pg")
	runWorkflowPlanCommandPrintsBoundSteps(t, dir, "PostgreSQL")
}

func TestWorkflowPlanCommandPrintsBoundStepsWithMySQLStore(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowProfile(t, dir)
	configureNamedMySQLActiveStore(t, "daily-workflow-plan-mysql")
	runWorkflowPlanCommandPrintsBoundSteps(t, dir, "MySQL")
}

func runWorkflowPlanCommandPrintsBoundSteps(t *testing.T, dir string, label string) {
	t.Helper()
	runCLI(t, "config", "publish", "--from", dir)

	out := runCLI(t, "workflow", "plan", "--workflow", "workflow.alpha")

	for _, want := range []string{
		"Workflow: workflow.alpha",
		"Step: step.one",
		"Node: node.alpha",
		"Case: case.alpha",
		"Required: true",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("%s workflow plan output missing %q: %q", label, want, out)
		}
	}
}

func TestWorkflowPlanCommandCanEmitJSONFromStore(t *testing.T) {
	profileDir := t.TempDir()
	writeWorkflowProfile(t, profileDir)
	configureNamedPostgreSQLActiveStore(t, "daily-workflow-plan-json-pg")
	runWorkflowPlanCommandCanEmitJSONFromStore(t, profileDir, "PostgreSQL")
}

func TestWorkflowPlanCommandCanEmitJSONFromMySQLStore(t *testing.T) {
	profileDir := t.TempDir()
	writeWorkflowProfile(t, profileDir)
	configureNamedMySQLActiveStore(t, "daily-workflow-plan-json-mysql")
	runWorkflowPlanCommandCanEmitJSONFromStore(t, profileDir, "MySQL")
}

func runWorkflowPlanCommandCanEmitJSONFromStore(t *testing.T, profileDir string, label string) {
	t.Helper()
	runCLI(t, "config", "publish", "--from", profileDir)

	out := runCLI(t, "workflow", "plan", "--workflow", "workflow.alpha", "--json")

	var payload struct {
		OK         bool   `json:"ok"`
		ProfileID  string `json:"profileId"`
		WorkflowID string `json:"workflowId"`
		Counts     struct {
			Steps         int `json:"steps"`
			RequiredSteps int `json:"requiredSteps"`
		} `json:"counts"`
		Steps []struct {
			StepID string `json:"stepId"`
			NodeID string `json:"nodeId"`
			CaseID string `json:"caseId"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode %s workflow plan json: %v\n%s", label, err, out)
	}
	if !payload.OK || payload.ProfileID != "sample" || payload.WorkflowID != "workflow.alpha" || payload.Counts.Steps != 1 || payload.Counts.RequiredSteps != 1 {
		t.Fatalf("%s workflow plan json summary = %#v", label, payload)
	}
	if len(payload.Steps) != 1 || payload.Steps[0].StepID != "step.one" || payload.Steps[0].NodeID != "node.alpha" || payload.Steps[0].CaseID != "case.alpha" {
		t.Fatalf("%s workflow plan json steps = %#v", label, payload.Steps)
	}
}

func TestWorkflowPlanCommandRejectsMissingWorkflow(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowProfile(t, dir)
	configureNamedPostgreSQLActiveStore(t, "daily-workflow-plan-missing-pg")
	runWorkflowPlanCommandRejectsMissingWorkflow(t, dir, "PostgreSQL")
}

func TestWorkflowPlanCommandRejectsMissingWorkflowWithMySQLStore(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowProfile(t, dir)
	configureNamedMySQLActiveStore(t, "daily-workflow-plan-missing-mysql")
	runWorkflowPlanCommandRejectsMissingWorkflow(t, dir, "MySQL")
}

func runWorkflowPlanCommandRejectsMissingWorkflow(t *testing.T, dir string, label string) {
	t.Helper()
	runCLI(t, "config", "publish", "--from", dir)

	out := runCLIFails(t, "workflow", "plan", "--workflow", "workflow.missing")
	if !strings.Contains(out, "workflow not found") || !strings.Contains(out, "workflow.missing") {
		t.Fatalf("%s missing workflow output = %q", label, out)
	}
}
