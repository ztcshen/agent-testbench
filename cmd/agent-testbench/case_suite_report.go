package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"agent-testbench/internal/domain/casesuite"
	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profilecatalog"
	"agent-testbench/internal/runner/junit"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

type caseSuiteReport struct {
	OK             bool                          `json:"ok"`
	ProfileID      string                        `json:"profileId"`
	Title          string                        `json:"title"`
	ReportURL      string                        `json:"reportUrl"`
	JSONReportURL  string                        `json:"jsonReportUrl"`
	JUnitReportURL string                        `json:"junitReportUrl,omitempty"`
	ElapsedMs      int64                         `json:"elapsedMs"`
	GeneratedAt    time.Time                     `json:"generatedAt"`
	Filters        caseListFilter                `json:"filters"`
	Counts         interfaceNodeCaseReportCounts `json:"counts"`
	Results        []caseSuiteReportItem         `json:"results"`
	Warnings       []string                      `json:"warnings,omitempty"`
	SourceStoreURL string                        `json:"sourceStoreUrl,omitempty"`
}

type caseSuiteReportItem struct {
	CaseID      string   `json:"caseId"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	NodeID      string   `json:"nodeId,omitempty"`
	NodeName    string   `json:"nodeName,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	Owner       string   `json:"owner,omitempty"`
	RunID       string   `json:"runId,omitempty"`
	CaseRunID   string   `json:"caseRunId,omitempty"`
	ViewerURL   string   `json:"viewerUrl,omitempty"`
	DetailURL   string   `json:"detailUrl,omitempty"`
	Status      string   `json:"status"`
	HTTPCode    int      `json:"httpCode,omitempty"`
	ElapsedMs   int64    `json:"elapsedMs"`
	Method      string   `json:"method,omitempty"`
	Path        string   `json:"path,omitempty"`
	FullURL     string   `json:"fullUrl,omitempty"`
	BaseURL     string   `json:"baseUrl,omitempty"`
	Error       string   `json:"error,omitempty"`
	BodyPreview string   `json:"bodyPreview,omitempty"`
}

func runCaseSuiteReport(ctx context.Context, args []string) error {
	selection := newCaseSelectionCLIFlags("case suite report", "active")
	baseURL := selection.flags.String("base-url", "", "Base URL for live request execution")
	outputDir := selection.flags.String("output-dir", "", "Report output directory")
	timeoutSeconds := selection.flags.Int("timeout-seconds", 3, "Timeout per API Case")
	jsonOutput := selection.flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := selection.parse(args); err != nil {
		return err
	}
	if *timeoutSeconds <= 0 {
		return errors.New("--timeout-seconds must be greater than zero")
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*selection.storeRef, *selection.storeURL)
	if err != nil {
		return err
	}
	bundle, sourceStore, cleanup, err := loadInterfaceNodeReportBundle(ctx, *selection.profilePath, *selection.profileHome, resolvedStoreURL)
	if err != nil {
		return err
	}
	defer cleanup()
	filters := selection.caseListFilter()
	cases := selectedCaseSuiteCases(bundle, filters)
	if len(cases) == 0 {
		return errors.New("no API cases matched selector")
	}
	derived := deriveCaseSuiteConfigs(bundle, cases)
	bundle.TemplateConfigs = mergeTemplateConfigs(bundle.TemplateConfigs, derived)
	if strings.TrimSpace(*outputDir) == "" {
		*outputDir = filepath.Join(".runtime", "reports", "case-suite."+safeReportID(caseSuiteFilterSlug(filters))+"."+time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	absOutputDir, err := filepath.Abs(*outputDir)
	if err != nil {
		return err
	}
	report, err := executeCaseSuiteReport(ctx, bundle, cases, derived, sourceStore, resolvedStoreURL, filters, *baseURL, absOutputDir, *timeoutSeconds)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printCaseSuiteReport(report)
	return nil
}

func selectedCaseSuiteCases(bundle profile.Bundle, filters caseListFilter) []profile.APICase {
	return casesuite.SelectCases(bundle, caseSuiteFilter(filters))
}

func deriveCaseSuiteConfigs(bundle profile.Bundle, cases []profile.APICase) []profile.TemplateConfig {
	nodesByID := make(map[string]profile.InterfaceNode, len(bundle.InterfaceNodes))
	for _, node := range bundle.InterfaceNodes {
		nodesByID[node.ID] = node
	}
	casesByNode := map[string][]profile.APICase{}
	for _, item := range cases {
		casesByNode[item.NodeID] = append(casesByNode[item.NodeID], item)
	}
	nodeIDs := make([]string, 0, len(casesByNode))
	for nodeID := range casesByNode {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)
	out := make([]profile.TemplateConfig, 0)
	selected := map[string]bool{}
	for _, item := range cases {
		selected[item.ID] = true
	}
	for _, nodeID := range nodeIDs {
		node, ok := nodesByID[nodeID]
		if !ok {
			continue
		}
		for _, config := range deriveInterfaceNodeCaseConfigs(bundle, node, interfaceNodeReportCases(bundle.APICases, nodeID)) {
			if selected[config.ScopeID] {
				out = append(out, config)
			}
		}
	}
	return out
}

func executeCaseSuiteReport(ctx context.Context, bundle profile.Bundle, cases []profile.APICase, derived []profile.TemplateConfig, sourceStore store.Store, sourceStoreURL string, filters caseListFilter, baseURL string, outputDir string, timeoutSeconds int) (caseSuiteReport, error) {
	started := time.Now()
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return caseSuiteReport{}, err
	}
	runtime, err := requiredReportStore(sourceStore)
	if err != nil {
		return caseSuiteReport{}, err
	}
	if err := runtime.ReplaceProfileCatalog(ctx, profilecatalog.FromBundle(bundle, time.Now().UTC())); err != nil {
		return caseSuiteReport{}, err
	}
	handler := controlplane.NewWithOptions(bundle, controlplane.Options{Runtime: runtime})
	server := httptest.NewServer(handler)
	defer server.Close()
	caseIDs := make([]string, 0, len(cases))
	for _, item := range cases {
		caseIDs = append(caseIDs, item.ID)
	}
	requestPayload := map[string]any{"caseIds": caseIDs, "baseUrl": baseURL, "timeoutSeconds": timeoutSeconds}
	rawRequest, _ := json.Marshal(requestPayload)
	response, err := http.Post(server.URL+"/api/test-kit/run-batch", "application/json", strings.NewReader(string(rawRequest)))
	if err != nil {
		return caseSuiteReport{}, err
	}
	defer response.Body.Close()
	var rawBatch map[string]any
	if err := json.NewDecoder(response.Body).Decode(&rawBatch); err != nil {
		return caseSuiteReport{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return caseSuiteReport{}, fmt.Errorf("case suite batch failed with http status %d", response.StatusCode)
	}
	report := caseSuiteReport{
		OK:             boolFromReportAny(rawBatch["ok"]),
		ProfileID:      bundle.ID,
		Title:          "Case Suite Report",
		ElapsedMs:      time.Since(started).Milliseconds(),
		GeneratedAt:    time.Now().UTC(),
		Filters:        normalizeCaseListFilter(filters),
		SourceStoreURL: sourceStoreURL,
		Counts: interfaceNodeCaseReportCounts{
			Total:          len(cases),
			DerivedConfigs: len(derived),
		},
	}
	report.Results = caseSuiteReportItems(interfaceNodeCaseReportItems(rawBatch["results"]), cases, bundle.InterfaceNodes)
	for _, item := range report.Results {
		if item.Status == store.StatusPassed {
			report.Counts.Passed++
		} else {
			report.Counts.Failed++
		}
	}
	report.OK = report.Counts.Total > 0 && report.Counts.Failed == 0
	if err := writeCaseSuiteReportFiles(outputDir, &report); err != nil {
		return caseSuiteReport{}, err
	}
	return report, nil
}

func caseSuiteReportItems(results []interfaceNodeCaseReportItem, cases []profile.APICase, nodes []profile.InterfaceNode) []caseSuiteReportItem {
	casesByID := make(map[string]profile.APICase, len(cases))
	for _, item := range cases {
		casesByID[item.ID] = item
	}
	nodesByID := make(map[string]profile.InterfaceNode, len(nodes))
	for _, node := range nodes {
		nodesByID[node.ID] = node
	}
	out := make([]caseSuiteReportItem, 0, len(results))
	for _, result := range results {
		apiCase := casesByID[result.CaseID]
		node := nodesByID[apiCase.NodeID]
		out = append(out, caseSuiteReportItem{
			CaseID:      result.CaseID,
			Title:       result.Title,
			Description: apiCase.Description,
			NodeID:      apiCase.NodeID,
			NodeName:    firstNonEmpty(node.DisplayName, apiCase.NodeID),
			Tags:        append([]string(nil), apiCase.Tags...),
			Priority:    apiCase.Priority,
			Owner:       apiCase.Owner,
			RunID:       result.RunID,
			CaseRunID:   result.CaseRunID,
			ViewerURL:   result.ViewerURL,
			DetailURL:   result.DetailURL,
			Status:      result.Status,
			HTTPCode:    result.HTTPCode,
			ElapsedMs:   result.ElapsedMs,
			Method:      result.Method,
			Path:        result.Path,
			FullURL:     result.FullURL,
			BaseURL:     result.BaseURL,
			Error:       result.Error,
			BodyPreview: result.BodyPreview,
		})
	}
	return out
}

func writeCaseSuiteReportFiles(outputDir string, report *caseSuiteReport) error {
	jsonPath := filepath.Join(outputDir, "report.json")
	htmlPath := filepath.Join(outputDir, "report.html")
	junitPath := filepath.Join(outputDir, "report.junit.xml")
	report.JSONReportURL = jsonPath
	report.ReportURL = htmlPath
	report.JUnitReportURL = junitPath
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(htmlPath, []byte(renderCaseSuiteReportHTML(*report)), 0o644); err != nil {
		return err
	}
	junitRaw, err := renderCaseSuiteJUnit(*report)
	if err != nil {
		return err
	}
	return os.WriteFile(junitPath, junitRaw, 0o644)
}

func renderCaseSuiteJUnit(report caseSuiteReport) ([]byte, error) {
	cases := make([]junit.Case, 0, len(report.Results))
	for _, item := range report.Results {
		cases = append(cases, junit.Case{
			Name:           firstNonEmpty(item.CaseID, item.Title),
			ClassName:      firstNonEmpty(item.NodeID, item.NodeName),
			Status:         item.Status,
			TimeSeconds:    float64(item.ElapsedMs) / 1000,
			FailureMessage: item.Error,
			Output:         firstNonEmpty(item.Error, item.BodyPreview),
		})
	}
	return junit.Render(junit.Suite{Name: firstNonEmpty(report.Title, "Case Suite Report"), Cases: cases})
}

func renderCaseSuiteReportHTML(report caseSuiteReport) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>Case Suite Report</title><style>`)
	b.WriteString(`body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;margin:24px;color:#111827;background:#f8fafc}main{max-width:1320px;margin:auto}h1{font-size:24px;margin:0 0 4px}.meta{color:#4b5563;margin-bottom:16px}.summary{display:flex;gap:8px;flex-wrap:wrap;margin:12px 0}.pill{border:1px solid #d1d5db;background:white;border-radius:6px;padding:6px 10px;font-size:13px}.ok{color:#047857}.bad{color:#b91c1c}table{width:100%;border-collapse:collapse;background:white;border:1px solid #d1d5db}th,td{border-bottom:1px solid #e5e7eb;text-align:left;vertical-align:top;padding:7px 8px;font-size:13px}th{background:#f3f4f6;color:#374151}.mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px}.wrap{word-break:break-all}.small{font-size:12px;color:#6b7280}`)
	b.WriteString(`</style></head><body><main>`)
	b.WriteString(`<h1>Case Suite Report</h1>`)
	b.WriteString(`<div class="meta">` + html.EscapeString(report.ProfileID) + `</div><div class="summary">`)
	b.WriteString(reportPill("status", statusText(report.OK)))
	b.WriteString(reportPill("total", strconv.Itoa(report.Counts.Total)))
	b.WriteString(reportPill("passed", strconv.Itoa(report.Counts.Passed)))
	b.WriteString(reportPill("failed", strconv.Itoa(report.Counts.Failed)))
	b.WriteString(reportPill("derived configs", strconv.Itoa(report.Counts.DerivedConfigs)))
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
	b.WriteString(`</div><table><thead><tr><th>#</th><th>Case</th><th>Node</th><th>Maintainer</th><th>Status</th><th>HTTP</th><th>Elapsed</th><th>Evidence</th><th>Request</th><th>Response</th><th>Error</th></tr></thead><tbody>`)
	for index, item := range report.Results {
		statusClass := "bad"
		if item.Status == store.StatusPassed {
			statusClass = "ok"
		}
		b.WriteString(`<tr><td class="mono">` + strconv.Itoa(index+1) + `</td>`)
		b.WriteString(`<td><div>` + html.EscapeString(item.Title) + `</div><div class="mono small wrap">` + html.EscapeString(item.CaseID) + `</div>`)
		if item.Description != "" {
			b.WriteString(`<div class="small">` + html.EscapeString(item.Description) + `</div>`)
		}
		b.WriteString(`</td>`)
		b.WriteString(`<td><div>` + html.EscapeString(item.NodeName) + `</div><div class="mono small wrap">` + html.EscapeString(item.NodeID) + `</div></td>`)
		b.WriteString(`<td><div>` + html.EscapeString(item.Owner) + `</div><div class="small">` + html.EscapeString(item.Priority) + `</div><div class="small">` + html.EscapeString(strings.Join(item.Tags, ", ")) + `</div></td>`)
		b.WriteString(`<td class="` + statusClass + `">` + html.EscapeString(item.Status) + `</td>`)
		b.WriteString(`<td class="mono">` + strconv.Itoa(item.HTTPCode) + `</td>`)
		b.WriteString(`<td class="mono">` + fmt.Sprintf("%d ms", item.ElapsedMs) + `</td>`)
		b.WriteString(`<td class="mono wrap">`)
		if item.DetailURL != "" {
			b.WriteString(`<a href="` + html.EscapeString(item.DetailURL) + `">caseRunId</a><br>`)
		}
		b.WriteString(html.EscapeString(item.CaseRunID))
		b.WriteString(`</td>`)
		b.WriteString(`<td class="mono wrap">` + html.EscapeString(strings.TrimSpace(item.Method+" "+item.FullURL)) + `</td>`)
		b.WriteString(`<td class="mono wrap">` + html.EscapeString(item.BodyPreview) + `</td>`)
		b.WriteString(`<td class="wrap">` + html.EscapeString(item.Error) + `</td></tr>`)
	}
	b.WriteString(`</tbody></table></main></body></html>`)
	return b.String()
}

func printCaseSuiteReport(report caseSuiteReport) {
	fmt.Println("Case Suite Report")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Total: %d Passed: %d Failed: %d\n", report.Counts.Total, report.Counts.Passed, report.Counts.Failed)
	fmt.Printf("Derived Configs: %d\n", report.Counts.DerivedConfigs)
	fmt.Printf("Elapsed: %d ms\n", report.ElapsedMs)
	fmt.Printf("Report: %s\n", report.ReportURL)
}

func caseSuiteFilterSlug(filters caseListFilter) string {
	filters = normalizeCaseListFilter(filters)
	parts := []string{filters.Filter, filters.NodeID, filters.Status, filters.Owner, filters.Priority}
	parts = append(parts, filters.Tags...)
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			return part
		}
	}
	return "all"
}
