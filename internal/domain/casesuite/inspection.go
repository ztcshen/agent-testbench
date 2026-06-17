package casesuite

import (
	"context"
	"strings"

	"agent-testbench/internal/domain/execution"
	"agent-testbench/internal/domain/profile"
)

func Inspect(ctx context.Context, bundle profile.Bundle, runtime RecordStore, filter Filter, cases []profile.APICase) (InspectionReport, error) {
	coverage, err := Coverage(ctx, bundle, runtime, filter, cases)
	if err != nil {
		return InspectionReport{}, err
	}
	configs := ExecutionConfigSet(ctx, bundle, runtime)
	coverageByCase := coverageItemsByCase(coverage.Items)
	nodesByID := interfaceNodesByID(bundle.InterfaceNodes)
	servicesByID := servicesByID(bundle.Services)
	report := newInspectionReport(bundle.ID, coverage, len(cases))
	for _, item := range cases {
		row := inspectionRow(item, coverageByCase[item.ID], configs[item.ID], nodesByID[item.NodeID], servicesByID)
		addInspectionRow(&report, row)
	}
	return report, nil
}

func newInspectionReport(profileID string, coverage Report, total int) InspectionReport {
	report := InspectionReport{
		OK:          true,
		ProfileID:   profileID,
		GeneratedAt: coverage.GeneratedAt,
		Filters:     coverage.Filters,
		Counts:      InspectionCounts{Total: total},
		Items:       []InspectionItem{},
		Warnings:    append([]string(nil), coverage.Warnings...),
	}
	if total == 0 {
		report.OK = false
		report.Warnings = append(report.Warnings, "no cases matched selector")
	}
	return report
}

func coverageItemsByCase(items []Item) map[string]Item {
	out := make(map[string]Item, len(items))
	for _, item := range items {
		out[item.CaseID] = item
	}
	return out
}

func inspectionRow(item profile.APICase, coverageItem Item, hasExecutionConfig bool, node profile.InterfaceNode, services map[string]profile.Service) InspectionItem {
	status := CaseStatus(item)
	serviceID := strings.TrimSpace(node.ServiceID)
	serviceReady, serviceIssues := inspectCaseServiceReadiness(serviceID, services)
	row := InspectionItem{
		CaseID:             item.ID,
		Title:              firstNonEmpty(item.DisplayName, item.ID),
		Description:        item.Description,
		NodeID:             item.NodeID,
		NodeName:           coverageItem.NodeName,
		ServiceID:          serviceID,
		ServiceReady:       serviceReady,
		ServiceIssues:      serviceIssues,
		Tags:               append([]string(nil), item.Tags...),
		Priority:           item.Priority,
		Owner:              item.Owner,
		Status:             status,
		HasRunnableFile:    strings.TrimSpace(item.CasePath) != "",
		HasExecutionConfig: hasExecutionConfig,
		LatestStatus:       firstNonEmpty(coverageItem.LatestStatus, "not-run"),
		LatestRunID:        coverageItem.LatestRunID,
		CaseRunID:          coverageItem.CaseRunID,
		DetailURL:          coverageItem.DetailURL,
		ElapsedMs:          coverageItem.ElapsedMs,
		HasPassed:          coverageItem.HasPassed,
	}
	row.Issues = inspectionIssues(row)
	row.Ready = len(row.Issues) == 0
	row.SuggestedAction = SuggestedAction(row)
	return row
}

func inspectionIssues(row InspectionItem) []string {
	var issues []string
	if !IsExecutableCaseLifecycle(row.Status) {
		issues = append(issues, "case status is "+row.Status)
	}
	if !row.HasRunnableFile && !row.HasExecutionConfig {
		issues = append(issues, "missing runnable case file or execution config")
	}
	for _, issue := range row.ServiceIssues {
		issues = append(issues, "service "+issue)
	}
	return issues
}

func inspectCaseServiceReadiness(serviceID string, services map[string]profile.Service) (bool, []string) {
	if strings.TrimSpace(serviceID) == "" {
		return true, nil
	}
	service, ok := services[serviceID]
	if !ok {
		return false, []string{"missing-service"}
	}
	status := strings.ToLower(strings.TrimSpace(service.Status))
	if status != "" && status != CaseLifecycleActive {
		return false, []string{"service-status-" + status}
	}
	if strings.TrimSpace(service.StartupCommand) == "" && strings.TrimSpace(service.HealthURL) == "" && !hasDockerServiceFacts(service) {
		return false, []string{"missing-service-startup-command"}
	}
	return true, nil
}

func hasDockerServiceFacts(service profile.Service) bool {
	return strings.TrimSpace(service.DockerService) != "" ||
		strings.TrimSpace(service.ContainerName) != "" ||
		strings.TrimSpace(service.Image) != ""
}

func addInspectionRow(report *InspectionReport, row InspectionItem) {
	if row.Ready {
		report.Counts.Ready++
	} else {
		report.OK = false
		report.Counts.Blocked++
	}
	if !IsExecutableCaseLifecycle(row.Status) {
		report.Counts.Inactive++
	}
	if !row.HasRunnableFile {
		report.Counts.MissingRunnable++
	}
	if !row.HasExecutionConfig {
		report.Counts.MissingExecution++
	}
	switch NormalizeRunState(row.LatestStatus) {
	case execution.StatusPassed:
		report.Counts.Passed++
	case execution.StatusFailed:
		report.Counts.Failed++
	default:
		report.Counts.NotRun++
	}
	report.Items = append(report.Items, row)
}
