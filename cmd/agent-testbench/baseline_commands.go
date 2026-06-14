package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"agent-testbench/internal/store"
)

func runBaseline(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing baseline command")
	}
	switch args[0] {
	case "get":
		return runBaselineGet(ctx, args[1:])
	case "set":
		return runBaselineSet(ctx, args[1:])
	default:
		return fmt.Errorf("unknown baseline command: %s", args[0])
	}
}

func runBaselineGet(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("baseline get", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	profileID := flags.String("profile", "", "Profile id")
	subjectID := flags.String("subject", "", "Subject id")
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

	gate, err := s.GetBaselineGate(ctx, *profileID, *subjectID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("baseline gate not found: %s %s", *profileID, *subjectID)
		}
		return err
	}
	printBaselineGate(gate)
	return nil
}

func runBaselineSet(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("baseline set", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	profileID := flags.String("profile", "", "Profile id")
	subjectID := flags.String("subject", "", "Subject id")
	status := flags.String("status", "", "Gate status")
	required := flags.Bool("required", false, "Mark the gate as required")
	summaryJSON := flags.String("summary-json", "{}", "Gate summary JSON")
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

	now := time.Now().UTC()
	gate, err := s.UpsertBaselineGate(ctx, store.BaselineGate{
		ProfileID:   *profileID,
		SubjectID:   *subjectID,
		Status:      *status,
		Required:    *required,
		SummaryJSON: *summaryJSON,
		CheckedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return err
	}
	printBaselineGate(gate)
	return nil
}

func printBaselineGate(gate store.BaselineGate) {
	fmt.Printf("Baseline Gate: %s %s\n", gate.ProfileID, gate.SubjectID)
	fmt.Printf("Status: %s\n", gate.Status)
	fmt.Printf("Required: %t\n", gate.Required)
}
