package casesuite

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode"

	"agent-testbench/internal/domain/profile"
)

type QualityCounts struct {
	Nodes                  int `json:"nodes"`
	NodesWithoutCases      int `json:"nodesWithoutCases"`
	Cases                  int `json:"cases"`
	CompleteCases          int `json:"completeCases"`
	IncompleteCases        int `json:"incompleteCases"`
	MissingDescription     int `json:"missingDescription"`
	MissingTags            int `json:"missingTags"`
	MissingPriority        int `json:"missingPriority"`
	MissingOwner           int `json:"missingOwner"`
	MissingRunnable        int `json:"missingRunnable"`
	MissingExecution       int `json:"missingExecution"`
	Inactive               int `json:"inactive"`
	NonExecutableLifecycle int `json:"nonExecutableLifecycle"`
	InvalidStatus          int `json:"invalidStatus"`
}

type QualityCase struct {
	CaseID             string   `json:"caseId"`
	Title              string   `json:"title"`
	NodeID             string   `json:"nodeId,omitempty"`
	NodeName           string   `json:"nodeName,omitempty"`
	Status             string   `json:"status"`
	Lifecycle          string   `json:"lifecycle"`
	Tags               []string `json:"tags,omitempty"`
	Priority           string   `json:"priority,omitempty"`
	Owner              string   `json:"owner,omitempty"`
	HasDescription     bool     `json:"hasDescription"`
	HasRunnableFile    bool     `json:"hasRunnableFile"`
	HasExecutionConfig bool     `json:"hasExecutionConfig"`
	Complete           bool     `json:"complete"`
	Issues             []string `json:"issues,omitempty"`
}

type QualityNode struct {
	NodeID      string   `json:"nodeId"`
	DisplayName string   `json:"displayName,omitempty"`
	ServiceID   string   `json:"serviceId,omitempty"`
	Operation   string   `json:"operation,omitempty"`
	Method      string   `json:"method,omitempty"`
	Path        string   `json:"path,omitempty"`
	CaseCount   int      `json:"caseCount"`
	Issues      []string `json:"issues,omitempty"`
}

type QualityReport struct {
	OK          bool          `json:"ok"`
	ProfileID   string        `json:"profileId"`
	GeneratedAt string        `json:"generatedAt"`
	Filters     Filter        `json:"filters"`
	Counts      QualityCounts `json:"counts"`
	Cases       []QualityCase `json:"cases"`
	Nodes       []QualityNode `json:"nodes"`
	Warnings    []string      `json:"warnings,omitempty"`
}

type QualityPlanCounts struct {
	Total            int `json:"total"`
	DraftCase        int `json:"draftCase"`
	CompleteMetadata int `json:"completeMetadata"`
	AddRunnable      int `json:"addRunnable"`
	AddExecution     int `json:"addExecution"`
	ReviewLifecycle  int `json:"reviewLifecycle"`
}

type QualityPlanAction struct {
	Type            string   `json:"type"`
	NodeID          string   `json:"nodeId,omitempty"`
	NodeName        string   `json:"nodeName,omitempty"`
	CaseID          string   `json:"caseId,omitempty"`
	CaseTitle       string   `json:"caseTitle,omitempty"`
	SuggestedCaseID string   `json:"suggestedCaseId,omitempty"`
	Fields          []string `json:"fields,omitempty"`
	Issues          []string `json:"issues,omitempty"`
	Command         []string `json:"command,omitempty"`
	Reason          string   `json:"reason,omitempty"`
}

type QualityPlanReport struct {
	OK          bool                `json:"ok"`
	ProfileID   string              `json:"profileId"`
	GeneratedAt string              `json:"generatedAt"`
	Filters     Filter              `json:"filters"`
	Counts      QualityPlanCounts   `json:"counts"`
	Actions     []QualityPlanAction `json:"actions"`
	Quality     QualityReport       `json:"quality"`
	Warnings    []string            `json:"warnings,omitempty"`
}

func QualityPlan(ctx context.Context, bundle profile.Bundle, runtime RecordStore, filter Filter, cases []profile.APICase) (QualityPlanReport, error) {
	quality, err := Quality(ctx, bundle, runtime, filter, cases)
	if err != nil {
		return QualityPlanReport{}, err
	}
	report := QualityPlanReport{
		OK:          true,
		ProfileID:   bundle.ID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Filters:     quality.Filters,
		Actions:     []QualityPlanAction{},
		Quality:     quality,
		Warnings:    append([]string(nil), quality.Warnings...),
	}
	addQualityNodeActions(&report, quality.Nodes)
	addQualityCaseActions(&report, quality.Cases)
	sortQualityPlanActions(report.Actions)
	report.Counts.Total = len(report.Actions)
	return report, nil
}

func addQualityNodeActions(report *QualityPlanReport, nodes []QualityNode) {
	for _, node := range nodes {
		action := QualityPlanAction{
			Type:            "draft-case",
			NodeID:          node.NodeID,
			NodeName:        node.DisplayName,
			SuggestedCaseID: suggestedCaseID(node.NodeID),
			Issues:          append([]string(nil), node.Issues...),
			Reason:          "interface node has no maintained cases",
		}
		action.Command = []string{"interface-node", "case", "draft", "--node", node.NodeID, "--case-id", action.SuggestedCaseID, "--title", firstNonEmpty(node.DisplayName, node.NodeID) + " Default Case"}
		report.Actions = append(report.Actions, action)
		report.Counts.DraftCase++
	}
}

func addQualityCaseActions(report *QualityPlanReport, cases []QualityCase) {
	for _, item := range cases {
		if item.Complete {
			continue
		}
		addLifecycleAction(report, item)
		addMetadataAction(report, item)
		addRunnableAction(report, item)
		addExecutionConfigAction(report, item)
	}
}

func sortQualityPlanActions(actions []QualityPlanAction) {
	sort.SliceStable(actions, func(i, j int) bool {
		if actionRank(actions[i].Type) != actionRank(actions[j].Type) {
			return actionRank(actions[i].Type) < actionRank(actions[j].Type)
		}
		if actions[i].NodeID != actions[j].NodeID {
			return actions[i].NodeID < actions[j].NodeID
		}
		return actions[i].CaseID < actions[j].CaseID
	})
}

func addLifecycleAction(report *QualityPlanReport, item QualityCase) {
	if lifecycleIssues := caseLifecycleIssues(item.Issues); len(lifecycleIssues) > 0 {
		report.Actions = append(report.Actions, QualityPlanAction{
			Type:      "review-case-lifecycle",
			CaseID:    item.CaseID,
			CaseTitle: item.Title,
			NodeID:    item.NodeID,
			Issues:    lifecycleIssues,
			Reason:    "case lifecycle status is not executable",
		})
		report.Counts.ReviewLifecycle++
	}
}

func addMetadataAction(report *QualityPlanReport, item QualityCase) {
	fields := missingMetadataFields(item)
	if len(fields) == 0 {
		return
	}
	report.Actions = append(report.Actions, QualityPlanAction{
		Type:      "complete-case-metadata",
		CaseID:    item.CaseID,
		CaseTitle: item.Title,
		NodeID:    item.NodeID,
		Fields:    fields,
		Issues:    metadataIssues(item.Issues),
		Reason:    "case metadata is incomplete",
	})
	report.Counts.CompleteMetadata++
}

func addRunnableAction(report *QualityPlanReport, item QualityCase) {
	if item.HasRunnableFile {
		return
	}
	report.Actions = append(report.Actions, QualityPlanAction{
		Type:      QualityActionAddRunnable,
		CaseID:    item.CaseID,
		CaseTitle: item.Title,
		NodeID:    item.NodeID,
		Issues:    []string{"missing-runnable-source"},
		Reason:    "case has no runnable API case file",
	})
	report.Counts.AddRunnable++
}

func addExecutionConfigAction(report *QualityPlanReport, item QualityCase) {
	if item.HasExecutionConfig {
		return
	}
	report.Actions = append(report.Actions, QualityPlanAction{
		Type:      "add-execution-config",
		CaseID:    item.CaseID,
		CaseTitle: item.Title,
		NodeID:    item.NodeID,
		Issues:    []string{"missing-execution-config"},
		Reason:    "case has no execution config",
	})
	report.Counts.AddExecution++
}

func qualityNodeMatchesFilter(node profile.InterfaceNode, filter Filter) bool {
	if filter.NodeID != "" && node.ID != filter.NodeID {
		return false
	}
	if filter.Status != "" && !strings.EqualFold(interfaceNodeStatus(node), filter.Status) {
		return false
	}
	return MatchesText(filter.Filter, node.ID, node.DisplayName, node.ServiceID, node.Operation, node.Method, node.Path, node.Description, strings.Join(node.Tags, " "))
}

func interfaceNodeStatus(node profile.InterfaceNode) string {
	status := strings.ToLower(strings.TrimSpace(node.Status))
	if status == "" {
		return CaseLifecycleActive
	}
	return status
}

func missingMetadataFields(item QualityCase) []string {
	fields := []string{}
	if !item.HasDescription {
		fields = append(fields, "description")
	}
	if len(item.Tags) == 0 {
		fields = append(fields, "tags")
	}
	if strings.TrimSpace(item.Priority) == "" {
		fields = append(fields, "priority")
	}
	if strings.TrimSpace(item.Owner) == "" {
		fields = append(fields, "owner")
	}
	return fields
}

func metadataIssues(issues []string) []string {
	out := []string{}
	for _, issue := range issues {
		switch issue {
		case "missing-description", "missing-tags", "missing-priority", "missing-owner":
			out = append(out, issue)
		}
	}
	return out
}

func caseLifecycleIssues(issues []string) []string {
	out := []string{}
	for _, issue := range issues {
		switch issue {
		case "invalid-status", "non-executable-lifecycle":
			out = append(out, issue)
		}
	}
	return out
}

func hasRunnableSource(item profile.APICase) bool {
	return strings.TrimSpace(item.CasePath) != "" || hasExternalSource(item)
}

func hasExternalSource(item profile.APICase) bool {
	return strings.TrimSpace(item.SourcePath) != "" || strings.TrimSpace(item.SourceKind) != "" || strings.TrimSpace(item.ExecutorID) != ""
}

func executorReferenceSet(bundle profile.Bundle) map[string]profile.ExecutorDescriptor {
	refs := map[string]profile.ExecutorDescriptor{}
	for _, item := range bundle.Executors {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		refs[id] = item
	}
	return refs
}

func hasUsableExecutorReference(item profile.APICase, refs map[string]profile.ExecutorDescriptor) bool {
	executorID := strings.TrimSpace(item.ExecutorID)
	if executorID == "" || strings.TrimSpace(item.SourcePath) == "" {
		return false
	}
	executor, ok := refs[executorID]
	if !ok {
		return false
	}
	status := strings.TrimSpace(strings.ToLower(executor.Status))
	if status != "" && status != "active" {
		return false
	}
	sourceKind := strings.TrimSpace(strings.ToLower(item.SourceKind))
	executorKind := strings.TrimSpace(strings.ToLower(executor.Kind))
	return sourceKind == "" || executorKind == "" || sourceKind == executorKind
}

func suggestedCaseID(nodeID string) string {
	return "case." + slugValue(nodeID) + ".default"
}

func slugValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(builder.String(), "-")
	if out == "" {
		return "case"
	}
	return out
}

func actionRank(actionType string) int {
	switch actionType {
	case "draft-case":
		return 0
	case "complete-case-metadata":
		return 1
	case "review-case-lifecycle":
		return 2
	case QualityActionAddRunnable:
		return 3
	case "add-execution-config":
		return 4
	default:
		return 99
	}
}
