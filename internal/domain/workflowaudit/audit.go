package workflowaudit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"agent-testbench/internal/domain/auditrefs"
	"agent-testbench/internal/domain/execution"
	"agent-testbench/internal/domain/profile"
)

type Options struct {
	Bundle     profile.Bundle
	WorkflowID string
	Store      Store
}

type Store interface {
	ListRuns(context.Context) ([]execution.Run, error)
	ListAPICaseRuns(context.Context, string) ([]execution.APICaseRun, error)
}

type Report struct {
	OK           bool         `json:"ok"`
	ProfileID    string       `json:"profileId"`
	WorkflowID   string       `json:"workflowId"`
	DisplayName  string       `json:"displayName,omitempty"`
	BindingCount int          `json:"bindingCount"`
	Bindings     []BindingRef `json:"bindings"`
	IssueCount   int          `json:"issueCount"`
	Issues       []Issue      `json:"issues"`
	Store        *StoreReport `json:"store,omitempty"`
}

type BindingRef struct {
	StepID   string `json:"stepId"`
	NodeID   string `json:"nodeId"`
	CaseID   string `json:"caseId,omitempty"`
	Required bool   `json:"required"`
}

type Issue = auditrefs.Issue

type StoreReport struct {
	LatestRun    *RunState          `json:"latestRun,omitempty"`
	BindingCases []BindingCaseState `json:"bindingCases"`
}

type RunState struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	CreatedAt  time.Time `json:"createdAt,omitempty"`
}

type BindingCaseState struct {
	StepID       string `json:"stepId"`
	CaseID       string `json:"caseId"`
	Required     bool   `json:"required"`
	HasPassed    bool   `json:"hasPassed"`
	LatestStatus string `json:"latestStatus,omitempty"`
	LatestRunID  string `json:"latestRunId,omitempty"`
}

func Audit(ctx context.Context, options Options) (Report, error) {
	workflowID := strings.TrimSpace(options.WorkflowID)
	if workflowID == "" {
		return Report{}, errors.New("workflow id is required")
	}
	workflow, ok := findWorkflow(options.Bundle, workflowID)
	if !ok {
		return Report{}, fmt.Errorf("workflow not found: %s", workflowID)
	}

	bindings := workflowBindings(options.Bundle, workflowID)
	report := Report{
		OK:           true,
		ProfileID:    options.Bundle.ID,
		WorkflowID:   workflowID,
		DisplayName:  workflow.DisplayName,
		BindingCount: len(bindings),
		Bindings:     bindingRefs(bindings),
		Issues:       []Issue{},
	}

	auditor := referenceAuditor{
		nodes:    auditrefs.IDSetFrom(options.Bundle.InterfaceNodes, func(item profile.InterfaceNode) string { return item.ID }),
		apiCases: auditrefs.IDSetFrom(options.Bundle.APICases, func(item profile.APICase) string { return item.ID }),
		fixtures: auditrefs.IDSetFrom(options.Bundle.Fixtures, func(item profile.Fixture) string { return item.ID }),
		casesByID: auditrefs.ItemMapFrom(options.Bundle.APICases, func(item profile.APICase) string {
			return item.ID
		}),
		fixturesByID: auditrefs.ItemMapFrom(options.Bundle.Fixtures, func(item profile.Fixture) string {
			return item.ID
		}),
	}
	report.Issues = append(report.Issues, auditor.issues(options.Bundle, bindings)...)
	if options.Store != nil {
		storeReport, err := auditStore(ctx, options.Bundle.ID, workflowID, bindings, options.Store)
		if err != nil {
			return Report{}, err
		}
		report.Store = &storeReport
	}
	report.IssueCount = len(report.Issues)
	report.OK = report.IssueCount == 0
	return report, nil
}

type referenceAuditor struct {
	nodes        map[string]bool
	apiCases     map[string]bool
	fixtures     map[string]bool
	casesByID    map[string]profile.APICase
	fixturesByID map[string]profile.Fixture
}

func (a referenceAuditor) issues(bundle profile.Bundle, bindings []profile.WorkflowBinding) []Issue {
	var issues []Issue
	caseIDs := map[string]bool{}
	nodeIDs := map[string]bool{}
	for _, binding := range bindings {
		issues = append(issues, a.auditBindingReference(binding, caseIDs, nodeIDs)...)
	}

	fixtureIDs, dependencyIssues := a.fixtureIDsForBoundCases(bundle.CaseDependencies, caseIDs)
	issues = append(issues, dependencyIssues...)
	issues = append(issues, a.requestTemplateIssues(bundle.RequestTemplates, nodeIDs)...)
	issues = append(issues, a.fixtureJSONIssues(fixtureIDs)...)
	return issues
}

func (a referenceAuditor) auditBindingReference(binding profile.WorkflowBinding, caseIDs map[string]bool, nodeIDs map[string]bool) []Issue {
	var issues []Issue
	subject := auditrefs.BindingSubject(binding.WorkflowID, binding.StepID)
	if strings.TrimSpace(binding.StepID) == "" {
		issues = append(issues, auditrefs.NewIssue("workflow-binding-step-required", "workflowBinding", subject, "stepId", "Workflow binding must include a step id"))
	}
	if strings.TrimSpace(binding.NodeID) == "" {
		issues = append(issues, auditrefs.NewIssue("workflow-binding-node-required", "workflowBinding", subject, "nodeId", "Workflow binding must reference an interface node"))
	} else {
		nodeIDs[binding.NodeID] = true
		if !a.nodes[binding.NodeID] {
			issues = append(issues, auditrefs.NewIssue("workflow-binding-node-missing", "workflowBinding", subject, "nodeId", "Workflow binding references a missing interface node"))
		}
	}
	if strings.TrimSpace(binding.CaseID) == "" {
		return issues
	}
	if !a.apiCases[binding.CaseID] {
		return append(issues, auditrefs.NewIssue("workflow-binding-case-missing", "workflowBinding", subject, "caseId", "Workflow binding references a missing API Case"))
	}
	caseIDs[binding.CaseID] = true
	apiCase := a.casesByID[binding.CaseID]
	if strings.TrimSpace(apiCase.NodeID) != "" {
		nodeIDs[apiCase.NodeID] = true
		if !a.nodes[apiCase.NodeID] {
			issues = append(issues, auditrefs.NewIssue("api-case-node-missing", "apiCase", auditrefs.SubjectID(apiCase.ID), "nodeId", "API Case references a missing interface node"))
		}
	}
	return issues
}

func (a referenceAuditor) fixtureIDsForBoundCases(dependencies []profile.CaseDependency, caseIDs map[string]bool) ([]string, []Issue) {
	fixtureIDs := make([]string, 0)
	seenFixtureIDs := map[string]bool{}
	var issues []Issue
	for _, item := range dependencies {
		if !caseIDs[item.CaseID] {
			continue
		}
		if strings.TrimSpace(item.FixtureID) == "" {
			issues = append(issues, auditrefs.NewIssue("case-dependency-fixture-required", "caseDependency", auditrefs.SubjectID(item.ID), "fixtureId", "Case dependency must reference a fixture"))
			continue
		}
		if !seenFixtureIDs[item.FixtureID] {
			fixtureIDs = append(fixtureIDs, item.FixtureID)
			seenFixtureIDs[item.FixtureID] = true
		}
		if !a.fixtures[item.FixtureID] {
			issues = append(issues, auditrefs.NewIssue("case-dependency-fixture-missing", "caseDependency", auditrefs.SubjectID(item.ID), "fixtureId", "Case dependency references a missing fixture"))
		}
	}
	return fixtureIDs, issues
}

func (a referenceAuditor) requestTemplateIssues(templates []profile.RequestTemplate, nodeIDs map[string]bool) []Issue {
	var issues []Issue
	for _, item := range templates {
		if strings.TrimSpace(item.NodeID) == "" || !nodeIDs[item.NodeID] {
			continue
		}
		if !a.nodes[item.NodeID] {
			issues = append(issues, auditrefs.NewIssue("request-template-node-missing", "requestTemplate", auditrefs.SubjectID(item.ID), "nodeId", "Request template references a missing interface node"))
		}
	}
	return issues
}

func (a referenceAuditor) fixtureJSONIssues(fixtureIDs []string) []Issue {
	var issues []Issue
	for _, fixtureID := range fixtureIDs {
		item, ok := a.fixturesByID[fixtureID]
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(item.Kind), "json") && strings.TrimSpace(item.DataJSON) != "" && !json.Valid([]byte(item.DataJSON)) {
			issues = append(issues, auditrefs.NewIssue("fixture-data-json-invalid", "fixture", auditrefs.SubjectID(item.ID), "dataJson", "Fixture dataJson must be valid JSON"))
		}
	}
	return issues
}

func auditStore(ctx context.Context, profileID string, workflowID string, bindings []profile.WorkflowBinding, s Store) (StoreReport, error) {
	workflowRuns, err := workflowRunsFor(ctx, profileID, workflowID, s)
	if err != nil {
		return StoreReport{}, err
	}
	passed, latestStatus, latestRunID, latestRun, err := caseRunStateByCase(ctx, workflowRuns, s)
	if err != nil {
		return StoreReport{}, err
	}
	return StoreReport{
		LatestRun:    latestRun,
		BindingCases: bindingCaseStates(bindings, passed, latestStatus, latestRunID),
	}, nil
}

func workflowRunsFor(ctx context.Context, profileID string, workflowID string, s Store) ([]execution.Run, error) {
	runs, err := s.ListRuns(ctx)
	if err != nil {
		return nil, err
	}
	workflowRuns := make([]execution.Run, 0)
	for _, run := range runs {
		if run.ProfileID == profileID && run.WorkflowID == workflowID {
			workflowRuns = append(workflowRuns, run)
		}
	}
	return workflowRuns, nil
}

func caseRunStateByCase(ctx context.Context, workflowRuns []execution.Run, s Store) (map[string]bool, map[string]string, map[string]string, *RunState, error) {
	passed := map[string]bool{}
	latestStatus := map[string]string{}
	latestRunID := map[string]string{}
	var latestRun *RunState
	for i := len(workflowRuns) - 1; i >= 0; i-- {
		run := workflowRuns[i]
		if latestRun == nil {
			latestRun = &RunState{
				ID:         run.ID,
				Status:     run.Status,
				StartedAt:  run.StartedAt,
				FinishedAt: run.FinishedAt,
				CreatedAt:  run.CreatedAt,
			}
		}
		caseRuns, err := s.ListAPICaseRuns(ctx, run.ID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		for _, item := range caseRuns {
			if latestStatus[item.CaseID] == "" {
				latestStatus[item.CaseID] = item.Status
				latestRunID[item.CaseID] = run.ID
			}
			if strings.EqualFold(item.Status, execution.StatusPassed) {
				passed[item.CaseID] = true
			}
		}
	}
	return passed, latestStatus, latestRunID, latestRun, nil
}

func bindingCaseStates(bindings []profile.WorkflowBinding, passed map[string]bool, latestStatus map[string]string, latestRunID map[string]string) []BindingCaseState {
	states := make([]BindingCaseState, 0, len(bindings))
	for _, binding := range bindings {
		if strings.TrimSpace(binding.CaseID) == "" {
			continue
		}
		states = append(states, BindingCaseState{
			StepID:       binding.StepID,
			CaseID:       binding.CaseID,
			Required:     binding.Required,
			HasPassed:    passed[binding.CaseID],
			LatestStatus: latestStatus[binding.CaseID],
			LatestRunID:  latestRunID[binding.CaseID],
		})
	}
	return states
}

func findWorkflow(bundle profile.Bundle, id string) (profile.Workflow, bool) {
	for _, workflow := range bundle.Workflows {
		if workflow.ID == id {
			return workflow, true
		}
	}
	return profile.Workflow{}, false
}

func workflowBindings(bundle profile.Bundle, workflowID string) []profile.WorkflowBinding {
	out := make([]profile.WorkflowBinding, 0)
	for _, binding := range bundle.WorkflowBindings {
		if binding.WorkflowID == workflowID {
			out = append(out, binding)
		}
	}
	return out
}

func bindingRefs(bindings []profile.WorkflowBinding) []BindingRef {
	out := make([]BindingRef, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, BindingRef{
			StepID:   binding.StepID,
			NodeID:   binding.NodeID,
			CaseID:   binding.CaseID,
			Required: binding.Required,
		})
	}
	return out
}
