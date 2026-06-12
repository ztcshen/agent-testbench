package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"agent-testbench/internal/domain/apicasecommand"
	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profilecatalog"
	"agent-testbench/internal/store"
)

func runCaseIncompleteBatches(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case incomplete-batches", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Print JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	s, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return err
	}
	defer closeCLIStore(s)

	bundle, err := incompleteCaseBundle(ctx, strings.TrimSpace(*profilePath), s)
	if err != nil {
		return err
	}
	report, err := incompleteCaseReportForStore(ctx, bundle, s)
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	printIncompleteCaseReport(report)
	return nil
}

func incompleteCaseBundle(ctx context.Context, profilePath string, runtime store.Store) (profile.Bundle, error) {
	if profilePath != "" {
		return profile.Load(profilePath)
	}
	catalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil {
		return profile.Bundle{}, err
	}
	return profilecatalog.ToBundle(catalog), nil
}

type incompleteCaseReport struct {
	OK       bool                 `json:"ok"`
	Count    int                  `json:"count"`
	Items    []incompleteCaseItem `json:"items"`
	Warnings []string             `json:"warnings"`
}

type incompleteCaseItem struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	Reason           string `json:"reason"`
	Source           string `json:"source"`
	Message          string `json:"message"`
	SuggestedCommand string `json:"suggestedCommand"`
}

func incompleteCaseReportForStore(ctx context.Context, bundle profile.Bundle, s store.Store) (incompleteCaseReport, error) {
	passed, latest, err := apiCaseRunStatusByCase(ctx, s)
	if err != nil {
		return incompleteCaseReport{}, err
	}
	items := make([]incompleteCaseItem, 0)
	for _, item := range bundle.APICases {
		if strings.TrimSpace(item.ID) == "" || passed[item.ID] {
			continue
		}
		reason := "not-run"
		if status := latest[item.ID]; status != "" {
			reason = "latest-" + status
		}
		items = append(items, incompleteCaseItem{
			ID:               item.ID,
			Title:            firstNonEmpty(item.DisplayName, item.ID),
			Reason:           reason,
			Source:           "profile:" + bundle.ID,
			Message:          "no passed Store run found for this API Case",
			SuggestedCommand: apicasecommand.SuggestedRunCommand(item),
		})
	}
	return incompleteCaseReport{
		OK:       true,
		Count:    len(items),
		Items:    items,
		Warnings: []string{},
	}, nil
}

func apiCaseRunStatusByCase(ctx context.Context, s store.Store) (map[string]bool, map[string]string, error) {
	runs, err := s.ListRuns(ctx)
	if err != nil {
		return nil, nil, err
	}
	passed := map[string]bool{}
	latest := map[string]string{}
	for i := len(runs) - 1; i >= 0; i-- {
		caseRuns, err := s.ListAPICaseRuns(ctx, runs[i].ID)
		if err != nil {
			return nil, nil, err
		}
		for _, item := range caseRuns {
			if latest[item.CaseID] == "" {
				latest[item.CaseID] = item.Status
			}
			if strings.EqualFold(item.Status, store.StatusPassed) {
				passed[item.CaseID] = true
			}
		}
	}
	return passed, latest, nil
}

func printIncompleteCaseReport(report incompleteCaseReport) {
	fmt.Printf("Incomplete API Cases: %d\n", report.Count)
	for _, item := range report.Items {
		fmt.Printf("- %s [%s]\n", item.ID, item.Reason)
		if item.SuggestedCommand != "" {
			fmt.Printf("  %s\n", item.SuggestedCommand)
		}
	}
	for _, warning := range report.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}

func quoteCommandValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return `''`
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
