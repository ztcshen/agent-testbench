package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

type caseRunsCLIReport struct {
	OK       bool              `json:"ok"`
	CaseRuns []caseRunsCLIItem `json:"caseRuns"`
	Warnings []string          `json:"warnings"`
}

type caseRunsCLIItem struct {
	ID            string    `json:"id"`
	RunID         string    `json:"runId"`
	CaseID        string    `json:"caseId"`
	Status        string    `json:"status"`
	Operation     string    `json:"operation"`
	EvidencePath  string    `json:"evidencePath"`
	EvidenceCount int       `json:"evidenceCount"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func runCaseRuns(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case runs", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	runFilter := flags.String("run", "", "Only list case runs for one run id")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	report, err := listCaseRunsFromStore(ctx, runtime, *runFilter)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printCaseRuns(report)
	return nil
}

func listCaseRunsFromStore(ctx context.Context, runtime store.Store, runFilter string) (caseRunsCLIReport, error) {
	runs, err := runtime.ListRuns(ctx)
	if err != nil {
		return caseRunsCLIReport{}, err
	}
	filter := strings.TrimSpace(runFilter)
	report := caseRunsCLIReport{OK: true, Warnings: []string{}}
	for i := len(runs) - 1; i >= 0; i-- {
		run := runs[i]
		if filter != "" && run.ID != filter {
			continue
		}
		caseRuns, err := runtime.ListAPICaseRuns(ctx, run.ID)
		if err != nil {
			return caseRunsCLIReport{}, err
		}
		evidence, err := runtime.ListEvidence(ctx, run.ID)
		if err != nil {
			return caseRunsCLIReport{}, err
		}
		for j := len(caseRuns) - 1; j >= 0; j-- {
			report.CaseRuns = append(report.CaseRuns, caseRunsCLIItemFrom(run, caseRuns[j], evidence))
		}
	}
	return report, nil
}

func caseRunsCLIItemFrom(run store.Run, item store.APICaseRun, evidence []store.EvidenceRecord) caseRunsCLIItem {
	evidenceCount := 0
	for _, record := range evidence {
		if record.CaseRunID == item.ID {
			evidenceCount++
		}
	}
	request := rawJSONObject(item.RequestSummaryJSON)
	return caseRunsCLIItem{
		ID:            item.ID,
		RunID:         run.ID,
		CaseID:        item.CaseID,
		Status:        item.Status,
		Operation:     caseRunOperationFromRequest(request, item.CaseID),
		EvidencePath:  run.EvidenceRoot,
		EvidenceCount: evidenceCount,
		UpdatedAt:     firstNonZeroTime(item.CreatedAt, run.UpdatedAt, run.CreatedAt),
	}
}

func caseRunOperationFromRequest(request map[string]any, defaultValue string) string {
	method := strings.ToUpper(strings.TrimSpace(valueString(request["method"])))
	path := strings.TrimSpace(valueString(request["path"]))
	if method != "" && path != "" {
		return method + " " + path
	}
	if method != "" {
		return method
	}
	if path != "" {
		return path
	}
	return defaultValue
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func printCaseRuns(report caseRunsCLIReport) {
	fmt.Println("Case Runs")
	fmt.Printf("Total: %d\n", len(report.CaseRuns))
	for _, item := range report.CaseRuns {
		fmt.Printf("- %s [%s] %s %s evidence=%d\n", item.ID, item.Status, item.CaseID, item.Operation, item.EvidenceCount)
	}
}
