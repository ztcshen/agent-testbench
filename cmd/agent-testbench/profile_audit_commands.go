package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profileaudit"
)

func runProfileAudit(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("profile audit", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path")
	templatePackagePath := flags.String("template-package", "", "Template package path or installed template package id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	offlineTemplatePackage := flags.Bool("offline-template-package", false, "Read the template package directly for offline review")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	force := flags.Bool("force", false, "Replace an installed profile when --profile points to a packed archive")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !*offlineTemplatePackage {
		return errors.New("--profile audit reads template packages only for offline review; add --offline-template-package")
	}
	resolvedProfilePath, err := materializeProfileReference(templatePackageReference(*templatePackagePath, *profilePath), *profileHome, *force)
	if err != nil {
		return err
	}
	bundle, err := profile.Load(resolvedProfilePath)
	if err != nil {
		return err
	}

	options := profileaudit.Options{
		Bundle:     bundle,
		BundlePath: resolvedProfilePath,
	}
	resolvedStoreURL, err := resolveStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	if strings.TrimSpace(resolvedStoreURL) != "" {
		s, err := openStore(ctx, resolvedStoreURL)
		if err != nil {
			return err
		}
		defer closeCLIStore(s)
		options.Store = s
	}

	report, err := profileaudit.Audit(ctx, options)
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	printProfileAudit(report)
	return nil
}

func runProfileAuditPlan(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("profile audit-plan", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path")
	templatePackagePath := flags.String("template-package", "", "Template package path or installed template package id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	offlineTemplatePackage := flags.Bool("offline-template-package", false, "Read the template package directly for offline review")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	force := flags.Bool("force", false, "Replace an installed profile when --profile points to a packed archive")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !*offlineTemplatePackage {
		return errors.New("--profile audit-plan reads template packages only for offline review; add --offline-template-package")
	}
	resolvedStoreURL, err := resolveStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	report, err := profileAuditRepairPlan(ctx, templatePackageReference(*templatePackagePath, *profilePath), *profileHome, resolvedStoreURL, *force)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printProfileAuditRepairPlan(report)
	return nil
}

func profileAuditRepairPlan(ctx context.Context, profilePath string, profileHome string, storeURL string, force bool) (profileaudit.RepairPlanReport, error) {
	resolvedProfilePath, err := materializeProfileReference(profilePath, profileHome, force)
	if err != nil {
		return profileaudit.RepairPlanReport{}, err
	}
	bundle, err := profile.Load(resolvedProfilePath)
	if err != nil {
		return profileaudit.RepairPlanReport{}, err
	}
	options := profileaudit.Options{
		Bundle:     bundle,
		BundlePath: resolvedProfilePath,
	}
	if strings.TrimSpace(storeURL) != "" {
		s, err := openStore(ctx, storeURL)
		if err != nil {
			return profileaudit.RepairPlanReport{}, err
		}
		defer closeCLIStore(s)
		options.Store = s
	}
	audit, err := profileaudit.Audit(ctx, options)
	if err != nil {
		return profileaudit.RepairPlanReport{}, err
	}
	return profileaudit.RepairPlan(audit), nil
}

func printProfileAudit(report profileaudit.Report) {
	fmt.Printf("Profile Audit: %s\n", report.ProfileID)
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Issues: %d\n", report.IssueCount)
	for _, item := range report.Issues {
		fmt.Printf("- [%s] %s %s %s: %s\n", item.Severity, item.Code, item.SubjectType, item.SubjectID, item.Message)
	}
	if report.Store == nil {
		return
	}
	fmt.Printf("Store Profile Indexed: %t\n", report.Store.ProfileIndexed)
	if report.Store.BundleDigest != "" || report.Store.IndexedDigest != "" {
		fmt.Printf("Store Digest Matches: %t\n", report.Store.DigestMatches)
	}
	for _, item := range report.Store.APICases {
		status := item.LatestStatus
		if status == "" {
			status = "not-run"
		}
		fmt.Printf("API Case: %s Status: %s Passed: %t\n", item.CaseID, status, item.HasPassed)
	}
}

func printProfileAuditRepairPlan(report profileaudit.RepairPlanReport) {
	fmt.Printf("Profile Audit Repair Plan: %s\n", report.ProfileID)
	fmt.Printf("Issues: %d\n", report.IssueCount)
	fmt.Printf("Actions: %d\n", report.ActionCount)
	for _, item := range report.Actions {
		fmt.Printf("- %s %s %s %s: %s\n", item.Type, item.IssueCode, item.SubjectType, item.SubjectID, item.SuggestedChange)
	}
	for _, warning := range report.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}
