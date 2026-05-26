package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agent-testbench/internal/domain/casesuite"
	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/store"
)

func runCaseSuite(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing case suite command")
	}
	switch args[0] {
	case "report":
		return runCaseSuiteReport(ctx, args[1:])
	case "coverage":
		return runCaseSuiteCoverage(ctx, args[1:])
	case "stability":
		return runCaseSuiteStability(ctx, args[1:])
	case "priority":
		return runCaseSuitePriority(ctx, args[1:])
	case "brief":
		return runCaseSuiteBrief(ctx, args[1:])
	case "quality":
		return runCaseSuiteQuality(ctx, args[1:])
	case "quality-plan":
		return runCaseSuiteQualityPlan(ctx, args[1:])
	case "quality-report":
		return runCaseSuiteQualityReport(ctx, args[1:])
	case "inspect":
		return runCaseSuiteInspect(ctx, args[1:])
	case "plan":
		return runCaseSuitePlan(ctx, args[1:])
	case "impact":
		return runCaseSuiteImpact(ctx, args[1:])
	case "impact-report":
		return runCaseSuiteImpactReport(ctx, args[1:])
	default:
		return fmt.Errorf("unknown case suite command: %s", args[0])
	}
}

type caseSuiteCoverageReport struct {
	OK             bool             `json:"ok"`
	ProfileID      string           `json:"profileId"`
	GeneratedAt    string           `json:"generatedAt"`
	Filters        casesuite.Filter `json:"filters"`
	Counts         casesuite.Counts `json:"counts"`
	Items          []casesuite.Item `json:"items"`
	Warnings       []string         `json:"warnings,omitempty"`
	SourceStoreURL string           `json:"sourceStoreUrl,omitempty"`
}

func runCaseSuiteCoverage(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case suite coverage", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	filter := flags.String("filter", "", "Filter by id, display name, scenario, description, tag, owner, or priority")
	nodeID := flags.String("node", "", "Only include cases attached to this interface node id")
	status := flags.String("status", "active", "Only include cases with this status")
	owner := flags.String("owner", "", "Only include cases owned by this value")
	priority := flags.String("priority", "", "Only include cases with this priority")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var tags stringListFlag
	flags.Var(&tags, "tag", "Only include cases with this tag; repeat for multiple tags")
	if err := flags.Parse(args); err != nil {
		return err
	}
	bundle, sourceStore, resolvedStoreURL, cleanup, err := loadRequiredInterfaceNodeReportBundleFromStoreFlags(ctx, *profilePath, *profileHome, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	filters := caseListFilter{
		Filter:   *filter,
		NodeID:   *nodeID,
		Tags:     tags.Values(),
		Status:   *status,
		Owner:    *owner,
		Priority: *priority,
	}
	cases := selectedCaseSuiteCases(bundle, filters)
	report, err := caseSuiteCoverage(ctx, bundle, sourceStore, resolvedStoreURL, filters, cases)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printCaseSuiteCoverage(report)
	return nil
}

func caseSuiteCoverage(ctx context.Context, bundle profile.Bundle, runtime store.Store, sourceStoreURL string, filters caseListFilter, cases []profile.APICase) (caseSuiteCoverageReport, error) {
	report, err := casesuite.Coverage(ctx, bundle, runtime, caseSuiteFilter(filters), cases)
	if err != nil {
		return caseSuiteCoverageReport{}, err
	}
	return caseSuiteCoverageReport{
		OK:             report.OK,
		ProfileID:      report.ProfileID,
		GeneratedAt:    report.GeneratedAt,
		Filters:        report.Filters,
		Counts:         report.Counts,
		Items:          report.Items,
		Warnings:       report.Warnings,
		SourceStoreURL: sourceStoreURL,
	}, nil
}

func caseSuiteFilter(filters caseListFilter) casesuite.Filter {
	filters = normalizeCaseListFilter(filters)
	return casesuite.Filter{
		Filter:   filters.Filter,
		NodeID:   filters.NodeID,
		Tags:     append([]string(nil), filters.Tags...),
		Status:   filters.Status,
		Owner:    filters.Owner,
		Priority: filters.Priority,
	}
}

func printCaseSuiteCoverage(report caseSuiteCoverageReport) {
	fmt.Println("Case Suite Coverage")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Total: %d Passed: %d Failed: %d Not Run: %d\n", report.Counts.Total, report.Counts.Passed, report.Counts.Failed, report.Counts.NotRun)
	for _, item := range report.Items {
		fmt.Printf("- %s [%s]", item.CaseID, item.LatestStatus)
		if item.CaseRunID != "" {
			fmt.Printf(" %s", item.CaseRunID)
		}
		if item.Reason != "" {
			fmt.Printf(" %s", item.Reason)
		}
		fmt.Println()
	}
	for _, warning := range report.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}

func runCaseSuiteStability(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case suite stability", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	filter := flags.String("filter", "", "Filter by id, display name, scenario, description, tag, owner, or priority")
	nodeID := flags.String("node", "", "Only include cases attached to this interface node id")
	status := flags.String("status", "active", "Only include cases with this status")
	owner := flags.String("owner", "", "Only include cases owned by this value")
	priority := flags.String("priority", "", "Only include cases with this priority")
	limit := flags.Int("limit", 10, "Recent runs per case to analyze")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var tags stringListFlag
	flags.Var(&tags, "tag", "Only include cases with this tag; repeat for multiple tags")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *limit <= 0 {
		return errors.New("--limit must be greater than zero")
	}
	bundle, sourceStore, _, cleanup, err := loadRequiredInterfaceNodeReportBundleFromStoreFlags(ctx, *profilePath, *profileHome, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	filterValue := caseListFilter{
		Filter:   *filter,
		NodeID:   *nodeID,
		Tags:     tags.Values(),
		Status:   *status,
		Owner:    *owner,
		Priority: *priority,
	}
	cases := selectedCaseSuiteCases(bundle, filterValue)
	report, err := casesuite.Stability(ctx, bundle, sourceStore, caseSuiteFilter(filterValue), cases, casesuite.StabilityOptions{Limit: *limit})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printCaseSuiteStability(report)
	return nil
}

func printCaseSuiteStability(report casesuite.StabilityReport) {
	fmt.Println("Case Suite Stability")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Total: %d Stable: %d Unstable: %d Not Run: %d\n", report.Counts.Total, report.Counts.Stable, report.Counts.Unstable, report.Counts.NotRun)
	for _, item := range report.Items {
		fmt.Printf("- %s latest=%s transitions=%d unstable=%t\n", item.CaseID, item.LatestStatus, item.Transitions, item.Unstable)
		if item.Reason != "" {
			fmt.Printf("  reason: %s\n", item.Reason)
		}
	}
	for _, warning := range report.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}

func runCaseSuitePriority(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case suite priority", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	filter := flags.String("filter", "", "Filter by id, display name, scenario, description, tag, owner, or priority")
	nodeID := flags.String("node", "", "Only include cases attached to this interface node id")
	status := flags.String("status", "active", "Only include cases with this status")
	owner := flags.String("owner", "", "Only include cases owned by this value")
	priority := flags.String("priority", "", "Only include cases with this priority")
	limit := flags.Int("limit", 0, "Maximum ready cases to select; 0 selects all ready cases")
	requestID := flags.String("request-id", "", "Request id for the generated batch request")
	baseURL := flags.String("base-url", "", "Base URL for the generated batch request")
	evidenceDir := flags.String("evidence-dir", "", "Evidence directory for the generated batch request")
	timeoutSeconds := flags.Int("timeout-seconds", 0, "Timeout seconds for the generated batch request")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var tags stringListFlag
	var signals stringListFlag
	var changes stringListFlag
	flags.Var(&tags, "tag", "Only include cases with this tag; repeat for multiple tags")
	flags.Var(&signals, "signal", "Changed path, interface text, workflow text, tag, or case text; repeat for multiple signals")
	flags.Var(&changes, "change", "Alias for --signal; repeat for multiple changes")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *limit < 0 {
		return errors.New("--limit cannot be negative")
	}
	if *timeoutSeconds < 0 {
		return errors.New("--timeout-seconds cannot be negative")
	}
	bundle, sourceStore, _, cleanup, err := loadRequiredInterfaceNodeReportBundleFromStoreFlags(ctx, *profilePath, *profileHome, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	filterValue := caseListFilter{
		Filter:   *filter,
		NodeID:   *nodeID,
		Tags:     tags.Values(),
		Status:   *status,
		Owner:    *owner,
		Priority: *priority,
	}
	cases := selectedCaseSuiteCases(bundle, filterValue)
	prioritySignals := append(signals.Values(), changes.Values()...)
	report, err := casesuite.Priority(ctx, bundle, sourceStore, caseSuiteFilter(filterValue), cases, casesuite.PriorityOptions{
		Signals:        prioritySignals,
		Limit:          *limit,
		RequestID:      *requestID,
		BaseURL:        *baseURL,
		EvidenceDir:    *evidenceDir,
		TimeoutSeconds: *timeoutSeconds,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printCaseSuitePriority(report)
	return nil
}

func printCaseSuitePriority(report casesuite.PriorityReport) {
	fmt.Println("Case Suite Priority")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Total: %d Ready: %d Blocked: %d Selected: %d Skipped: %d\n", report.Counts.Total, report.Counts.Ready, report.Counts.Blocked, report.Counts.Selected, report.Counts.Skipped)
	for _, item := range report.Selected {
		fmt.Printf("- %s score=%d latest=%s\n", item.CaseID, item.Score, item.LatestStatus)
		for _, reason := range item.Reasons {
			fmt.Printf("  reason: %s\n", reason)
		}
	}
	for _, item := range report.Blocked {
		fmt.Printf("- blocked %s score=%d\n", item.CaseID, item.Score)
	}
	for _, warning := range report.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}

func runCaseSuiteBrief(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case suite brief", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	filter := flags.String("filter", "", "Filter by id, display name, scenario, description, tag, owner, or priority")
	nodeID := flags.String("node", "", "Only include cases attached to this interface node id")
	status := flags.String("status", "active", "Only include cases with this status")
	owner := flags.String("owner", "", "Only include cases owned by this value")
	priority := flags.String("priority", "", "Only include cases with this priority")
	limit := flags.Int("limit", 0, "Maximum ready cases to recommend; 0 recommends all ready cases")
	stabilityLimit := flags.Int("stability-limit", 10, "Recent runs per case to analyze")
	requestID := flags.String("request-id", "", "Request id for the generated batch request")
	baseURL := flags.String("base-url", "", "Base URL for the generated batch request")
	evidenceDir := flags.String("evidence-dir", "", "Evidence directory for the generated batch request")
	timeoutSeconds := flags.Int("timeout-seconds", 0, "Timeout seconds for the generated batch request")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var tags stringListFlag
	var signals stringListFlag
	var changes stringListFlag
	flags.Var(&tags, "tag", "Only include cases with this tag; repeat for multiple tags")
	flags.Var(&signals, "signal", "Changed path, interface text, workflow text, tag, or case text; repeat for multiple signals")
	flags.Var(&changes, "change", "Alias for --signal; repeat for multiple changes")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *limit < 0 {
		return errors.New("--limit cannot be negative")
	}
	if *stabilityLimit <= 0 {
		return errors.New("--stability-limit must be greater than zero")
	}
	if *timeoutSeconds < 0 {
		return errors.New("--timeout-seconds cannot be negative")
	}
	bundle, sourceStore, _, cleanup, err := loadRequiredInterfaceNodeReportBundleFromStoreFlags(ctx, *profilePath, *profileHome, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	filterValue := caseListFilter{
		Filter:   *filter,
		NodeID:   *nodeID,
		Tags:     tags.Values(),
		Status:   *status,
		Owner:    *owner,
		Priority: *priority,
	}
	cases := selectedCaseSuiteCases(bundle, filterValue)
	briefSignals := append(signals.Values(), changes.Values()...)
	report, err := casesuite.Brief(ctx, bundle, sourceStore, caseSuiteFilter(filterValue), cases, casesuite.BriefOptions{
		Signals:        briefSignals,
		Limit:          *limit,
		StabilityLimit: *stabilityLimit,
		RequestID:      *requestID,
		BaseURL:        *baseURL,
		EvidenceDir:    *evidenceDir,
		TimeoutSeconds: *timeoutSeconds,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printCaseSuiteBrief(report)
	return nil
}

func printCaseSuiteBrief(report casesuite.BriefReport) {
	fmt.Println("Case Suite Brief")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Total: %d Ready: %d Blocked: %d Passed: %d Failed: %d Not Run: %d Unstable: %d Recommended: %d\n", report.Counts.Total, report.Counts.Ready, report.Counts.Blocked, report.Counts.Passed, report.Counts.Failed, report.Counts.NotRun, report.Counts.Unstable, report.Counts.PrioritySelected)
	for _, item := range report.Recommended {
		fmt.Printf("- %s score=%d latest=%s\n", item.CaseID, item.Score, item.LatestStatus)
		for _, reason := range item.Reasons {
			fmt.Printf("  reason: %s\n", reason)
		}
	}
	for _, item := range report.Blocked {
		fmt.Printf("- blocked %s\n", item.CaseID)
		for _, issue := range item.Issues {
			fmt.Printf("  issue: %s\n", issue)
		}
	}
	for _, warning := range report.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}

func runCaseSuiteQuality(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case suite quality", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	filter := flags.String("filter", "", "Filter by id, display name, scenario, description, tag, owner, or priority")
	nodeID := flags.String("node", "", "Only include cases attached to this interface node id")
	status := flags.String("status", "active", "Only include cases with this status")
	owner := flags.String("owner", "", "Only include cases owned by this value")
	priority := flags.String("priority", "", "Only include cases with this priority")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var tags stringListFlag
	flags.Var(&tags, "tag", "Only include cases with this tag; repeat for multiple tags")
	if err := flags.Parse(args); err != nil {
		return err
	}
	bundle, sourceStore, _, cleanup, err := loadRequiredInterfaceNodeReportBundleFromStoreFlags(ctx, *profilePath, *profileHome, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	filterValue := caseListFilter{
		Filter:   *filter,
		NodeID:   *nodeID,
		Tags:     tags.Values(),
		Status:   *status,
		Owner:    *owner,
		Priority: *priority,
	}
	cases := selectedCaseSuiteCases(bundle, filterValue)
	report, err := casesuite.Quality(ctx, bundle, sourceStore, caseSuiteFilter(filterValue), cases)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printCaseSuiteQuality(report)
	return nil
}

func printCaseSuiteQuality(report casesuite.QualityReport) {
	fmt.Println("Case Suite Quality")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Nodes: %d Without Cases: %d Cases: %d Complete: %d Incomplete: %d\n", report.Counts.Nodes, report.Counts.NodesWithoutCases, report.Counts.Cases, report.Counts.CompleteCases, report.Counts.IncompleteCases)
	if report.Counts.InvalidStatus > 0 || report.Counts.NonExecutableLifecycle > 0 {
		fmt.Printf("Lifecycle: non-executable=%d invalid=%d\n", report.Counts.NonExecutableLifecycle, report.Counts.InvalidStatus)
	}
	for _, item := range report.Nodes {
		fmt.Printf("- node %s\n", item.NodeID)
		for _, issue := range item.Issues {
			fmt.Printf("  issue: %s\n", issue)
		}
	}
	for _, item := range report.Cases {
		if item.Complete {
			continue
		}
		fmt.Printf("- case %s\n", item.CaseID)
		for _, issue := range item.Issues {
			fmt.Printf("  issue: %s\n", issue)
		}
	}
	for _, warning := range report.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}

func runCaseSuiteQualityPlan(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case suite quality-plan", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	filter := flags.String("filter", "", "Filter by id, display name, scenario, description, tag, owner, or priority")
	nodeID := flags.String("node", "", "Only include cases attached to this interface node id")
	status := flags.String("status", "active", "Only include cases with this status")
	owner := flags.String("owner", "", "Only include cases owned by this value")
	priority := flags.String("priority", "", "Only include cases with this priority")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var tags stringListFlag
	flags.Var(&tags, "tag", "Only include cases with this tag; repeat for multiple tags")
	if err := flags.Parse(args); err != nil {
		return err
	}
	bundle, sourceStore, _, cleanup, err := loadRequiredInterfaceNodeReportBundleFromStoreFlags(ctx, *profilePath, *profileHome, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	filterValue := caseListFilter{
		Filter:   *filter,
		NodeID:   *nodeID,
		Tags:     tags.Values(),
		Status:   *status,
		Owner:    *owner,
		Priority: *priority,
	}
	cases := selectedCaseSuiteCases(bundle, filterValue)
	report, err := casesuite.QualityPlan(ctx, bundle, sourceStore, caseSuiteFilter(filterValue), cases)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printCaseSuiteQualityPlan(report)
	return nil
}

func printCaseSuiteQualityPlan(report casesuite.QualityPlanReport) {
	fmt.Println("Case Suite Quality Plan")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Total: %d Draft Case: %d Complete Metadata: %d Review Lifecycle: %d Add Runnable: %d Add Execution: %d\n", report.Counts.Total, report.Counts.DraftCase, report.Counts.CompleteMetadata, report.Counts.ReviewLifecycle, report.Counts.AddRunnable, report.Counts.AddExecution)
	for _, item := range report.Actions {
		switch item.Type {
		case "draft-case":
			fmt.Printf("- draft %s for node %s\n", item.SuggestedCaseID, item.NodeID)
		case "review-case-lifecycle":
			fmt.Printf("- review lifecycle %s\n", item.CaseID)
		default:
			fmt.Printf("- %s %s\n", item.Type, item.CaseID)
		}
		if len(item.Fields) > 0 {
			fmt.Printf("  fields: %s\n", strings.Join(item.Fields, ","))
		}
		if item.Reason != "" {
			fmt.Printf("  reason: %s\n", item.Reason)
		}
	}
	for _, warning := range report.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}

type caseSuiteQualityReport struct {
	OK             bool                        `json:"ok"`
	ProfileID      string                      `json:"profileId"`
	Title          string                      `json:"title"`
	ReportURL      string                      `json:"reportUrl"`
	JSONReportURL  string                      `json:"jsonReportUrl"`
	ElapsedMs      int64                       `json:"elapsedMs"`
	GeneratedAt    time.Time                   `json:"generatedAt"`
	Filters        caseListFilter              `json:"filters"`
	Counts         casesuite.QualityPlanCounts `json:"counts"`
	QualityPlan    casesuite.QualityPlanReport `json:"qualityPlan"`
	Warnings       []string                    `json:"warnings,omitempty"`
	SourceStoreURL string                      `json:"sourceStoreUrl,omitempty"`
}

func runCaseSuiteQualityReport(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case suite quality-report", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	filter := flags.String("filter", "", "Filter by id, display name, scenario, description, tag, owner, or priority")
	nodeID := flags.String("node", "", "Only include cases attached to this interface node id")
	status := flags.String("status", "active", "Only include cases with this status")
	owner := flags.String("owner", "", "Only include cases owned by this value")
	priority := flags.String("priority", "", "Only include cases with this priority")
	outputDir := flags.String("output-dir", "", "Report output directory")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var tags stringListFlag
	flags.Var(&tags, "tag", "Only include cases with this tag; repeat for multiple tags")
	if err := flags.Parse(args); err != nil {
		return err
	}
	bundle, sourceStore, resolvedStoreURL, cleanup, err := loadRequiredInterfaceNodeReportBundleFromStoreFlags(ctx, *profilePath, *profileHome, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	filterValue := caseListFilter{
		Filter:   *filter,
		NodeID:   *nodeID,
		Tags:     tags.Values(),
		Status:   *status,
		Owner:    *owner,
		Priority: *priority,
	}
	cases := selectedCaseSuiteCases(bundle, filterValue)
	if strings.TrimSpace(*outputDir) == "" {
		*outputDir = filepath.Join(".runtime", "reports", "case-suite-quality."+safeReportID(caseSuiteFilterSlug(filterValue))+"."+time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	absOutputDir, err := filepath.Abs(*outputDir)
	if err != nil {
		return err
	}
	report, err := executeCaseSuiteQualityReport(ctx, bundle, sourceStore, resolvedStoreURL, filterValue, cases, absOutputDir)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printCaseSuiteQualityReport(report)
	return nil
}

func writeCaseSuiteQualityReportFiles(outputDir string, report *caseSuiteQualityReport) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	jsonPath := filepath.Join(outputDir, "report.json")
	htmlPath := filepath.Join(outputDir, "report.html")
	report.JSONReportURL = jsonPath
	report.ReportURL = htmlPath
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.WriteFile(htmlPath, []byte(renderCaseSuiteQualityReportHTML(*report)), 0o644)
}

func renderCaseSuiteQualityReportHTML(report caseSuiteQualityReport) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>Case Suite Quality Report</title><style>`)
	b.WriteString(`body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;margin:24px;color:#111827;background:#f8fafc}main{max-width:1280px;margin:auto}h1{font-size:24px;margin:0 0 4px}.meta{color:#4b5563;margin-bottom:16px}.summary{display:flex;gap:8px;flex-wrap:wrap;margin:12px 0}.pill{border:1px solid #d1d5db;background:white;border-radius:6px;padding:6px 10px;font-size:13px}table{width:100%;border-collapse:collapse;background:white;border:1px solid #d1d5db}th,td{border-bottom:1px solid #e5e7eb;text-align:left;vertical-align:top;padding:7px 8px;font-size:13px}th{background:#f3f4f6;color:#374151}.mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px}.wrap{word-break:break-all}.small{font-size:12px;color:#6b7280}.ok{color:#047857}.bad{color:#b91c1c}`)
	b.WriteString(`</style></head><body><main>`)
	b.WriteString(`<h1>Case Suite Quality Report</h1>`)
	b.WriteString(`<div class="meta">` + html.EscapeString(report.ProfileID) + `</div><div class="summary">`)
	b.WriteString(reportPill("status", statusText(report.QualityPlan.Quality.OK)))
	b.WriteString(reportPill("actions", strconv.Itoa(report.Counts.Total)))
	b.WriteString(reportPill("draft", strconv.Itoa(report.Counts.DraftCase)))
	b.WriteString(reportPill("metadata", strconv.Itoa(report.Counts.CompleteMetadata)))
	b.WriteString(reportPill("runnable", strconv.Itoa(report.Counts.AddRunnable)))
	b.WriteString(reportPill("execution", strconv.Itoa(report.Counts.AddExecution)))
	b.WriteString(reportPill("elapsed", fmt.Sprintf("%d ms", report.ElapsedMs)))
	if len(report.Filters.Tags) > 0 {
		b.WriteString(reportPill("tags", strings.Join(report.Filters.Tags, ",")))
	}
	if report.Filters.Owner != "" {
		b.WriteString(reportPill("owner", report.Filters.Owner))
	}
	if report.Filters.Priority != "" {
		b.WriteString(reportPill("priority", report.Filters.Priority))
	}
	b.WriteString(`</div><table><thead><tr><th>#</th><th>Action</th><th>Target</th><th>Fields</th><th>Issues</th><th>Reason</th><th>Command</th></tr></thead><tbody>`)
	for index, item := range report.QualityPlan.Actions {
		target := firstNonEmpty(item.CaseID, item.SuggestedCaseID, item.NodeID)
		b.WriteString(`<tr><td class="mono">` + strconv.Itoa(index+1) + `</td>`)
		b.WriteString(`<td><div>` + html.EscapeString(item.Type) + `</div></td>`)
		b.WriteString(`<td><div class="mono wrap">` + html.EscapeString(target) + `</div>`)
		if item.NodeID != "" {
			b.WriteString(`<div class="small">node: ` + html.EscapeString(item.NodeID) + `</div>`)
		}
		if item.NodeName != "" {
			b.WriteString(`<div class="small">` + html.EscapeString(item.NodeName) + `</div>`)
		}
		b.WriteString(`</td>`)
		b.WriteString(`<td class="wrap">` + html.EscapeString(strings.Join(item.Fields, ", ")) + `</td>`)
		b.WriteString(`<td class="wrap">` + html.EscapeString(strings.Join(item.Issues, ", ")) + `</td>`)
		b.WriteString(`<td class="wrap">` + html.EscapeString(item.Reason) + `</td>`)
		b.WriteString(`<td class="mono wrap">` + html.EscapeString(strings.Join(item.Command, " ")) + `</td></tr>`)
	}
	b.WriteString(`</tbody></table></main></body></html>`)
	return b.String()
}

func printCaseSuiteQualityReport(report caseSuiteQualityReport) {
	fmt.Println("Case Suite Quality Report")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Total Actions: %d Draft Case: %d Complete Metadata: %d Add Runnable: %d Add Execution: %d\n", report.Counts.Total, report.Counts.DraftCase, report.Counts.CompleteMetadata, report.Counts.AddRunnable, report.Counts.AddExecution)
	fmt.Printf("Elapsed: %d ms\n", report.ElapsedMs)
	fmt.Printf("Report: %s\n", report.ReportURL)
	fmt.Printf("JSON: %s\n", report.JSONReportURL)
}

func runCaseSuiteInspect(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case suite inspect", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	filter := flags.String("filter", "", "Filter by id, display name, scenario, description, tag, owner, or priority")
	nodeID := flags.String("node", "", "Only include cases attached to this interface node id")
	status := flags.String("status", "active", "Only include cases with this status")
	owner := flags.String("owner", "", "Only include cases owned by this value")
	priority := flags.String("priority", "", "Only include cases with this priority")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var tags stringListFlag
	flags.Var(&tags, "tag", "Only include cases with this tag; repeat for multiple tags")
	if err := flags.Parse(args); err != nil {
		return err
	}
	bundle, sourceStore, _, cleanup, err := loadRequiredInterfaceNodeReportBundleFromStoreFlags(ctx, *profilePath, *profileHome, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	filterValue := caseListFilter{
		Filter:   *filter,
		NodeID:   *nodeID,
		Tags:     tags.Values(),
		Status:   *status,
		Owner:    *owner,
		Priority: *priority,
	}
	cases := selectedCaseSuiteCases(bundle, filterValue)
	report, err := casesuite.Inspect(ctx, bundle, sourceStore, caseSuiteFilter(filterValue), cases)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printCaseSuiteInspection(report)
	return nil
}

func printCaseSuiteInspection(report casesuite.InspectionReport) {
	fmt.Println("Case Suite Inspection")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Total: %d Ready: %d Blocked: %d Passed: %d Failed: %d Not Run: %d\n", report.Counts.Total, report.Counts.Ready, report.Counts.Blocked, report.Counts.Passed, report.Counts.Failed, report.Counts.NotRun)
	for _, item := range report.Items {
		fmt.Printf("- %s ready=%t latest=%s action=%s\n", item.CaseID, item.Ready, item.LatestStatus, item.SuggestedAction)
		for _, issue := range item.Issues {
			fmt.Printf("  issue: %s\n", issue)
		}
	}
	for _, warning := range report.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}

func runCaseSuitePlan(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case suite plan", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	filter := flags.String("filter", "", "Filter by id, display name, scenario, description, tag, owner, or priority")
	nodeID := flags.String("node", "", "Only include cases attached to this interface node id")
	status := flags.String("status", "active", "Only include cases with this status")
	owner := flags.String("owner", "", "Only include cases owned by this value")
	priority := flags.String("priority", "", "Only include cases with this priority")
	requestID := flags.String("request-id", "", "Request id for the generated batch request")
	baseURL := flags.String("base-url", "", "Base URL for the generated batch request")
	evidenceDir := flags.String("evidence-dir", "", "Evidence directory for the generated batch request")
	timeoutSeconds := flags.Int("timeout-seconds", 0, "Timeout seconds for the generated batch request")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var tags stringListFlag
	var actions stringListFlag
	flags.Var(&tags, "tag", "Only include cases with this tag; repeat for multiple tags")
	flags.Var(&actions, "action", "Only select ready cases with this suggested action; repeat for multiple actions")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *timeoutSeconds < 0 {
		return errors.New("--timeout-seconds cannot be negative")
	}
	bundle, sourceStore, _, cleanup, err := loadRequiredInterfaceNodeReportBundleFromStoreFlags(ctx, *profilePath, *profileHome, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	filterValue := caseListFilter{
		Filter:   *filter,
		NodeID:   *nodeID,
		Tags:     tags.Values(),
		Status:   *status,
		Owner:    *owner,
		Priority: *priority,
	}
	cases := selectedCaseSuiteCases(bundle, filterValue)
	report, err := casesuite.Plan(ctx, bundle, sourceStore, caseSuiteFilter(filterValue), cases, casesuite.PlanOptions{
		RequestID:      *requestID,
		Actions:        actions.Values(),
		BaseURL:        *baseURL,
		EvidenceDir:    *evidenceDir,
		TimeoutSeconds: *timeoutSeconds,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printCaseSuitePlan(report)
	return nil
}

func printCaseSuitePlan(report casesuite.PlanReport) {
	fmt.Println("Case Suite Plan")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Total: %d Ready: %d Blocked: %d Selected: %d Skipped: %d\n", report.Counts.Total, report.Counts.Ready, report.Counts.Blocked, report.Counts.Selected, report.Counts.Skipped)
	for _, item := range report.Selected {
		fmt.Printf("- %s action=%s latest=%s\n", item.CaseID, item.SuggestedAction, item.LatestStatus)
	}
	for _, item := range report.Blocked {
		fmt.Printf("- blocked %s action=%s\n", item.CaseID, item.SuggestedAction)
	}
	for _, warning := range report.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}

func runCaseSuiteImpact(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case suite impact", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	filter := flags.String("filter", "", "Additional case selector filter")
	nodeID := flags.String("node", "", "Only include cases attached to this interface node id")
	status := flags.String("status", "active", "Only include cases with this status")
	owner := flags.String("owner", "", "Only include cases owned by this value")
	priority := flags.String("priority", "", "Only include cases with this priority")
	requestID := flags.String("request-id", "", "Request id for the generated batch request")
	baseURL := flags.String("base-url", "", "Base URL for the generated batch request")
	evidenceDir := flags.String("evidence-dir", "", "Evidence directory for the generated batch request")
	timeoutSeconds := flags.Int("timeout-seconds", 0, "Timeout seconds for the generated batch request")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var tags stringListFlag
	var actions stringListFlag
	var signals stringListFlag
	var changes stringListFlag
	flags.Var(&tags, "tag", "Only include cases with this tag; repeat for multiple tags")
	flags.Var(&actions, "action", "Only select ready cases with this suggested action; repeat for multiple actions")
	flags.Var(&signals, "signal", "Changed path, interface text, workflow text, tag, or case text; repeat for multiple signals")
	flags.Var(&changes, "change", "Alias for --signal; repeat for multiple changes")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *timeoutSeconds < 0 {
		return errors.New("--timeout-seconds cannot be negative")
	}
	bundle, sourceStore, _, cleanup, err := loadRequiredInterfaceNodeReportBundleFromStoreFlags(ctx, *profilePath, *profileHome, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	filterValue := caseListFilter{
		Filter:   *filter,
		NodeID:   *nodeID,
		Tags:     tags.Values(),
		Status:   *status,
		Owner:    *owner,
		Priority: *priority,
	}
	impactSignals := append(signals.Values(), changes.Values()...)
	report, err := casesuite.Impact(ctx, bundle, sourceStore, caseSuiteFilter(filterValue), casesuite.ImpactOptions{
		Signals: impactSignals,
		Plan: casesuite.PlanOptions{
			RequestID:      *requestID,
			Actions:        actions.Values(),
			BaseURL:        *baseURL,
			EvidenceDir:    *evidenceDir,
			TimeoutSeconds: *timeoutSeconds,
		},
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printCaseSuiteImpact(report)
	return nil
}

func printCaseSuiteImpact(report casesuite.ImpactReport) {
	fmt.Println("Case Suite Impact")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Signals: %d Nodes: %d Workflows: %d Cases: %d Selected: %d Blocked: %d\n", report.Counts.Signals, report.Counts.Nodes, report.Counts.Workflows, report.Counts.Cases, report.Counts.Selected, report.Counts.Blocked)
	for _, item := range report.Cases {
		fmt.Printf("- %s action=%s latest=%s\n", item.CaseID, item.SuggestedAction, item.LatestStatus)
		for _, reason := range item.Reasons {
			fmt.Printf("  reason: %s\n", reason)
		}
	}
	for _, warning := range report.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}

type caseSuiteImpactExecutionReport struct {
	OK        bool                   `json:"ok"`
	Impact    casesuite.ImpactReport `json:"impact"`
	Report    caseSuiteReport        `json:"report"`
	ElapsedMs int64                  `json:"elapsedMs"`
}

func runCaseSuiteImpactReport(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case suite impact-report", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	filter := flags.String("filter", "", "Additional case selector filter")
	nodeID := flags.String("node", "", "Only include cases attached to this interface node id")
	status := flags.String("status", "active", "Only include cases with this status")
	owner := flags.String("owner", "", "Only include cases owned by this value")
	priority := flags.String("priority", "", "Only include cases with this priority")
	requestID := flags.String("request-id", "", "Request id for the generated batch request")
	baseURL := flags.String("base-url", "", "Base URL for live request execution")
	outputDir := flags.String("output-dir", "", "Report output directory")
	timeoutSeconds := flags.Int("timeout-seconds", 3, "Timeout per API Case")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var tags stringListFlag
	var actions stringListFlag
	var signals stringListFlag
	var changes stringListFlag
	flags.Var(&tags, "tag", "Only include cases with this tag; repeat for multiple tags")
	flags.Var(&actions, "action", "Only select ready cases with this suggested action; repeat for multiple actions")
	flags.Var(&signals, "signal", "Changed path, interface text, workflow text, tag, or case text; repeat for multiple signals")
	flags.Var(&changes, "change", "Alias for --signal; repeat for multiple changes")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *timeoutSeconds <= 0 {
		return errors.New("--timeout-seconds must be greater than zero")
	}
	started := time.Now()
	bundle, sourceStore, resolvedStoreURL, cleanup, err := loadRequiredInterfaceNodeReportBundleFromStoreFlags(ctx, *profilePath, *profileHome, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	filters := caseListFilter{
		Filter:   *filter,
		NodeID:   *nodeID,
		Tags:     tags.Values(),
		Status:   *status,
		Owner:    *owner,
		Priority: *priority,
	}
	impactSignals := append(signals.Values(), changes.Values()...)
	impact, err := casesuite.Impact(ctx, bundle, sourceStore, caseSuiteFilter(filters), casesuite.ImpactOptions{
		Signals: impactSignals,
		Plan: casesuite.PlanOptions{
			RequestID:      *requestID,
			Actions:        actions.Values(),
			BaseURL:        *baseURL,
			TimeoutSeconds: *timeoutSeconds,
		},
	})
	if err != nil {
		return err
	}
	cases := apiCasesByIDs(bundle.APICases, impact.BatchRequest.CaseIDs)
	if len(cases) == 0 {
		return errors.New("no ready impacted API cases selected for execution")
	}
	derived := deriveCaseSuiteConfigs(bundle, cases)
	bundle.TemplateConfigs = mergeTemplateConfigs(bundle.TemplateConfigs, derived)
	if strings.TrimSpace(*outputDir) == "" {
		*outputDir = filepath.Join(".runtime", "reports", "case-suite-impact."+safeReportID(strings.Join(impact.Signals, "-"))+"."+time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	absOutputDir, err := filepath.Abs(*outputDir)
	if err != nil {
		return err
	}
	report, err := executeCaseSuiteReport(ctx, bundle, cases, derived, sourceStore, resolvedStoreURL, filters, *baseURL, absOutputDir, *timeoutSeconds)
	if err != nil {
		return err
	}
	out := caseSuiteImpactExecutionReport{
		OK:        impact.OK && report.OK,
		Impact:    impact,
		Report:    report,
		ElapsedMs: time.Since(started).Milliseconds(),
	}
	if *jsonOutput {
		return writeIndentedJSON(out)
	}
	printCaseSuiteImpactExecutionReport(out)
	return nil
}

func apiCasesByIDs(cases []profile.APICase, ids []string) []profile.APICase {
	byID := map[string]profile.APICase{}
	for _, item := range cases {
		byID[item.ID] = item
	}
	out := make([]profile.APICase, 0, len(ids))
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			out = append(out, item)
		}
	}
	return out
}

func printCaseSuiteImpactExecutionReport(report caseSuiteImpactExecutionReport) {
	fmt.Println("Case Suite Impact Report")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Selected: %d Passed: %d Failed: %d\n", report.Impact.Counts.Selected, report.Report.Counts.Passed, report.Report.Counts.Failed)
	for _, item := range report.Report.Results {
		fmt.Printf("- %s [%s]", item.CaseID, item.Status)
		if item.CaseRunID != "" {
			fmt.Printf(" %s", item.CaseRunID)
		}
		fmt.Println()
	}
}
