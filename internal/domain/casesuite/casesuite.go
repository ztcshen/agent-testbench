package casesuite

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	domaincatalog "agent-testbench/internal/domain/catalog"
	"agent-testbench/internal/domain/execution"
	"agent-testbench/internal/domain/profile"
)

func SelectCases(bundle profile.Bundle, filter Filter) []profile.APICase {
	filter = NormalizeFilter(filter)
	out := make([]profile.APICase, 0)
	for _, item := range bundle.APICases {
		if CaseMatches(item, filter) {
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].NodeID != out[j].NodeID {
			return out[i].NodeID < out[j].NodeID
		}
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func Coverage(ctx context.Context, bundle profile.Bundle, runtime RecordStore, filter Filter, cases []profile.APICase) (Report, error) {
	report := Report{
		OK:          true,
		ProfileID:   bundle.ID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Filters:     NormalizeFilter(filter),
		Counts:      Counts{Total: len(cases)},
		Items:       []Item{},
	}
	if runtime == nil {
		report.OK = len(cases) == 0
		report.Counts.NotRun = len(cases)
		report.Warnings = append(report.Warnings, "runtime store is not configured")
	}
	records, err := RecordsForCaseIDs(ctx, runtime, CaseIDs(cases))
	if err != nil {
		return Report{}, err
	}
	stateByCase := StateByCase(records)
	nodesByID := map[string]profile.InterfaceNode{}
	for _, node := range bundle.InterfaceNodes {
		nodesByID[node.ID] = node
	}
	for _, item := range cases {
		state := stateByCase[item.ID]
		node := nodesByID[item.NodeID]
		row := Item{
			CaseID:      item.ID,
			Title:       firstNonEmpty(item.DisplayName, item.ID),
			Description: item.Description,
			NodeID:      item.NodeID,
			NodeName:    firstNonEmpty(node.DisplayName, item.NodeID),
			Tags:        append([]string(nil), item.Tags...),
			Priority:    item.Priority,
			Owner:       item.Owner,
			HasPassed:   state.HasPassed,
		}
		if state.Latest.CaseRun.ID == "" {
			row.LatestStatus = "not-run"
			row.Reason = ReasonNoRunRecorded
			report.Counts.NotRun++
			report.OK = false
		} else {
			row.LatestStatus = state.Latest.CaseRun.Status
			row.LatestRunID = state.Latest.Run.ID
			row.CaseRunID = state.Latest.CaseRun.ID
			row.DetailURL = DetailURL(row.CaseRunID)
			row.ElapsedMs = ElapsedMs(state.Latest.CaseRun.StartedAt, state.Latest.CaseRun.FinishedAt)
			if isPassedStatus(state.Latest.CaseRun.Status) {
				report.Counts.Passed++
			} else {
				report.Counts.Failed++
				report.OK = false
				row.Reason = firstNonEmpty(AssertionSummaryReason(state.Latest.CaseRun.AssertionSummaryJSON), "latest run is "+state.Latest.CaseRun.Status)
			}
		}
		report.Items = append(report.Items, row)
	}
	return report, nil
}

func Plan(ctx context.Context, bundle profile.Bundle, runtime RecordStore, filter Filter, cases []profile.APICase, options PlanOptions) (PlanReport, error) {
	inspection, err := Inspect(ctx, bundle, runtime, filter, cases)
	if err != nil {
		return PlanReport{}, err
	}
	options.Actions = NormalizeStringList(options.Actions)
	actionSet := actionSet(options.Actions)
	report := PlanReport{
		OK:          true,
		ProfileID:   bundle.ID,
		GeneratedAt: inspection.GeneratedAt,
		Filters:     inspection.Filters,
		Options:     options,
		Counts: PlanCounts{
			Total:   inspection.Counts.Total,
			Ready:   inspection.Counts.Ready,
			Blocked: inspection.Counts.Blocked,
		},
		Selected: []InspectionItem{},
		Blocked:  []InspectionItem{},
		Skipped:  []InspectionItem{},
		Warnings: append([]string(nil), inspection.Warnings...),
	}
	for _, item := range inspection.Items {
		if !item.Ready {
			report.Blocked = append(report.Blocked, item)
			continue
		}
		if len(actionSet) > 0 && !actionSet[item.SuggestedAction] {
			report.Skipped = append(report.Skipped, item)
			continue
		}
		report.Selected = append(report.Selected, item)
		report.CaseIDs = append(report.CaseIDs, item.CaseID)
	}
	report.Counts.Selected = len(report.Selected)
	report.Counts.Skipped = len(report.Skipped)
	report.BatchRequest = newBatchRequest(report.CaseIDs, options.RequestID, options.BaseURL, options.EvidenceDir, options.TimeoutSeconds)
	if len(report.CaseIDs) == 0 {
		report.OK = false
		report.Warnings = append(report.Warnings, "no ready cases selected for execution")
	}
	return report, nil
}

type State struct {
	Latest    execution.APICaseRunRecord
	HasPassed bool
}

func StateByCase(records []execution.APICaseRunRecord) map[string]State {
	out := map[string]State{}
	for _, record := range records {
		caseID := record.CaseRun.CaseID
		state := out[caseID]
		if isPassedStatus(record.CaseRun.Status) {
			state.HasPassed = true
		}
		if state.Latest.CaseRun.ID == "" || RecordNewer(record, state.Latest) {
			state.Latest = record
		}
		out[caseID] = state
	}
	return out
}

func RecordNewer(left execution.APICaseRunRecord, right execution.APICaseRunRecord) bool {
	if left.CaseRun.CreatedAt.After(right.CaseRun.CreatedAt) {
		return true
	}
	return left.CaseRun.CreatedAt.Equal(right.CaseRun.CreatedAt) && left.CaseRun.ID > right.CaseRun.ID
}

func RecordsForCaseIDs(ctx context.Context, runtime RecordStore, caseIDs []string) ([]execution.APICaseRunRecord, error) {
	if runtime == nil || len(caseIDs) == 0 {
		return []execution.APICaseRunRecord{}, nil
	}
	if fast, ok := runtime.(interface {
		ListAPICaseRunRecordsForCaseIDs(context.Context, []string) ([]execution.APICaseRunRecord, error)
	}); ok {
		return fast.ListAPICaseRunRecordsForCaseIDs(ctx, caseIDs)
	}
	caseSet := map[string]bool{}
	for _, id := range caseIDs {
		caseSet[id] = true
	}
	runs, err := runtime.ListRuns(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]execution.APICaseRunRecord, 0)
	for _, run := range runs {
		caseRuns, err := runtime.ListAPICaseRuns(ctx, run.ID)
		if err != nil {
			return nil, err
		}
		for _, caseRun := range caseRuns {
			if caseSet[caseRun.CaseID] {
				out = append(out, execution.APICaseRunRecord{Run: run, CaseRun: caseRun})
			}
		}
	}
	return out, nil
}

func ExecutionConfigSet(ctx context.Context, bundle profile.Bundle, runtime RecordStore) map[string]bool {
	out := map[string]bool{}
	addProfileTemplateConfigs(out, bundle.TemplateConfigs)
	if catalogRuntime, ok := runtime.(interface {
		GetProfileCatalog(context.Context) (domaincatalog.ProfileCatalog, error)
	}); ok {
		if catalog, err := catalogRuntime.GetProfileCatalog(ctx); err == nil {
			addCatalogTemplateConfigs(out, catalog.TemplateConfigs)
		}
	}
	return out
}

func addProfileTemplateConfigs(out map[string]bool, configs []profile.TemplateConfig) {
	for _, config := range configs {
		addExecutionTemplateConfig(out, config.Status, config.ScopeType, config.ScopeID, config.ConfigJSON)
	}
}

func addCatalogTemplateConfigs(out map[string]bool, configs []domaincatalog.TemplateConfig) {
	for _, config := range configs {
		addExecutionTemplateConfig(out, config.Status, config.ScopeType, config.ScopeID, config.ConfigJSON)
	}
}

func addExecutionTemplateConfig(out map[string]bool, status string, scopeType string, scopeID string, configJSON string) {
	if !activeStatus(status) {
		return
	}
	if scopeType == "case" && strings.TrimSpace(scopeID) != "" {
		out[strings.TrimSpace(scopeID)] = true
		return
	}
	if caseID := executionConfigCaseID(configJSON); caseID != "" {
		out[caseID] = true
	}
}

func executionConfigCaseID(configJSON string) string {
	var payload struct {
		CaseID        string         `json:"caseId"`
		CaseExecution map[string]any `json:"caseExecution"`
	}
	if json.Unmarshal([]byte(configJSON), &payload) != nil {
		return ""
	}
	if strings.TrimSpace(payload.CaseID) == "" || len(payload.CaseExecution) == 0 {
		return ""
	}
	return strings.TrimSpace(payload.CaseID)
}

func activeStatus(status string) bool {
	return strings.TrimSpace(status) == "" || strings.EqualFold(strings.TrimSpace(status), "active")
}

func SuggestedAction(item InspectionItem) string {
	if !IsExecutableCaseLifecycle(item.Status) {
		return "review-status"
	}
	if !item.HasRunnableFile && !item.HasExecutionConfig {
		return QualityActionAddRunnable
	}
	if NormalizeRunState(item.LatestStatus) == execution.StatusFailed {
		return "rerun"
	}
	if NormalizeRunState(item.LatestStatus) == "not-run" {
		return "run"
	}
	return "keep"
}

func actionSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out[value] = true
		}
	}
	return out
}
