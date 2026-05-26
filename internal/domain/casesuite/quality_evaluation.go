package casesuite

import (
	"context"
	"strings"
	"time"

	"agent-testbench/internal/domain/profile"
)

type qualityEvaluation struct {
	configs      map[string]bool
	executorRefs map[string]profile.ExecutorDescriptor
	nodesByID    map[string]profile.InterfaceNode
	casesByNode  map[string]int
}

func Quality(ctx context.Context, bundle profile.Bundle, runtime RecordStore, filter Filter, cases []profile.APICase) (QualityReport, error) {
	filter = NormalizeFilter(filter)
	evaluation := newQualityEvaluation(ctx, bundle, runtime, cases)
	report := newQualityReport(bundle.ID, filter)
	for _, item := range cases {
		report.addCase(evaluation.caseRow(item))
	}
	for _, node := range bundle.InterfaceNodes {
		report.addNodeIfUncovered(node, filter, evaluation.casesByNode)
	}
	report.finish()
	return report, nil
}

func newQualityEvaluation(ctx context.Context, bundle profile.Bundle, runtime RecordStore, cases []profile.APICase) qualityEvaluation {
	return qualityEvaluation{
		configs:      ExecutionConfigSet(ctx, bundle, runtime),
		executorRefs: executorReferenceSet(bundle),
		nodesByID:    interfaceNodesByID(bundle.InterfaceNodes),
		casesByNode:  caseCountsByNode(cases),
	}
}

func newQualityReport(profileID string, filter Filter) QualityReport {
	return QualityReport{
		OK:          true,
		ProfileID:   profileID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Filters:     filter,
		Cases:       []QualityCase{},
		Nodes:       []QualityNode{},
	}
}

func (e qualityEvaluation) caseRow(item profile.APICase) QualityCase {
	node := e.nodesByID[item.NodeID]
	row := QualityCase{
		CaseID:             item.ID,
		Title:              firstNonEmpty(item.DisplayName, item.ID),
		NodeID:             item.NodeID,
		NodeName:           firstNonEmpty(node.DisplayName, item.NodeID),
		Status:             CaseStatus(item),
		Lifecycle:          CaseStatus(item),
		Tags:               append([]string(nil), item.Tags...),
		Priority:           item.Priority,
		Owner:              item.Owner,
		HasDescription:     strings.TrimSpace(item.Description) != "",
		HasRunnableFile:    hasRunnableSource(item),
		HasExecutionConfig: e.configs[item.ID] || hasUsableExecutorReference(item, e.executorRefs),
	}
	row.Issues = qualityCaseIssues(item, row, e.executorRefs)
	row.Complete = len(row.Issues) == 0
	return row
}

func qualityCaseIssues(item profile.APICase, row QualityCase, executorRefs map[string]profile.ExecutorDescriptor) []string {
	var issues []string
	if hasExternalSource(item) && !hasUsableExecutorReference(item, executorRefs) {
		issues = append(issues, "missing-executor")
	}
	if row.Lifecycle == CaseLifecycleInvalid {
		issues = append(issues, "invalid-status")
	}
	if !IsExecutableCaseLifecycle(row.Lifecycle) {
		issues = append(issues, "inactive", "non-executable-lifecycle")
	}
	if !row.HasDescription {
		issues = append(issues, "missing-description")
	}
	if len(row.Tags) == 0 {
		issues = append(issues, "missing-tags")
	}
	if strings.TrimSpace(row.Priority) == "" {
		issues = append(issues, "missing-priority")
	}
	if strings.TrimSpace(row.Owner) == "" {
		issues = append(issues, "missing-owner")
	}
	if !row.HasRunnableFile {
		issues = append(issues, "missing-runnable-source")
	}
	if !row.HasExecutionConfig {
		issues = append(issues, "missing-execution-config")
	}
	return issues
}

func (r *QualityReport) addCase(row QualityCase) {
	if row.Complete {
		r.Counts.CompleteCases++
	} else {
		r.Counts.IncompleteCases++
	}
	for _, issue := range row.Issues {
		r.countCaseIssue(issue)
	}
	r.Cases = append(r.Cases, row)
}

func (r *QualityReport) countCaseIssue(issue string) {
	switch issue {
	case "invalid-status":
		r.Counts.InvalidStatus++
	case "inactive":
		r.Counts.Inactive++
	case "non-executable-lifecycle":
		r.Counts.NonExecutableLifecycle++
	case "missing-description":
		r.Counts.MissingDescription++
	case "missing-tags":
		r.Counts.MissingTags++
	case "missing-priority":
		r.Counts.MissingPriority++
	case "missing-owner":
		r.Counts.MissingOwner++
	case "missing-runnable-source":
		r.Counts.MissingRunnable++
	case "missing-execution-config":
		r.Counts.MissingExecution++
	}
}

func (r *QualityReport) addNodeIfUncovered(node profile.InterfaceNode, filter Filter, casesByNode map[string]int) {
	if !qualityNodeMatchesFilter(node, filter) {
		return
	}
	r.Counts.Nodes++
	caseCount := casesByNode[node.ID]
	if caseCount > 0 {
		return
	}
	r.Counts.NodesWithoutCases++
	r.Nodes = append(r.Nodes, QualityNode{
		NodeID:      node.ID,
		DisplayName: node.DisplayName,
		ServiceID:   node.ServiceID,
		Operation:   node.Operation,
		Method:      node.Method,
		Path:        node.Path,
		CaseCount:   0,
		Issues:      []string{"no-maintained-cases"},
	})
}

func (r *QualityReport) finish() {
	r.Counts.Cases = len(r.Cases)
	r.OK = r.Counts.IncompleteCases == 0 && r.Counts.NodesWithoutCases == 0
	if r.Counts.Cases == 0 {
		r.Warnings = append(r.Warnings, "no cases matched selector")
	}
}

func interfaceNodesByID(nodes []profile.InterfaceNode) map[string]profile.InterfaceNode {
	out := make(map[string]profile.InterfaceNode, len(nodes))
	for _, node := range nodes {
		out[node.ID] = node
	}
	return out
}

func caseCountsByNode(cases []profile.APICase) map[string]int {
	out := map[string]int{}
	for _, item := range cases {
		if strings.TrimSpace(item.NodeID) != "" {
			out[item.NodeID]++
		}
	}
	return out
}
