package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profileaudit"
	"agent-testbench/internal/domain/profilecatalog"
	"agent-testbench/internal/profilepublish"
	"agent-testbench/internal/profileverify"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func runProfileCatalogIndex(ctx context.Context, args []string) error {
	options, err := parseProfileCatalogReadOptions("profile catalog-index", args)
	if err != nil {
		return err
	}
	report, err := readProfileCatalogIndex(ctx, options.StoreURL)
	if err != nil {
		return err
	}
	if options.JSONOutput {
		return writeIndentedJSON(report)
	}
	printProfileCatalogIndex(report)
	return nil
}

func runProfileVerify(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("profile verify", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path")
	templatePackagePath := flags.String("template-package", "", "Template package path or installed template package id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	requireCaseRuns := flags.Bool("require-case-runs", false, "Require every API Case in the profile to have a latest passed Store run")
	requireWorkflowRuns := flags.Bool("require-workflow-runs", false, "Require every Workflow in the profile to have a latest passed Store run")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	force := flags.Bool("force", false, "Replace an installed profile when --profile points to a packed archive")
	if err := flags.Parse(args); err != nil {
		return err
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	resolvedProfilePath, err := materializeProfileReference(templatePackageReference(*templatePackagePath, *profilePath), *profileHome, *force)
	if err != nil {
		return err
	}
	if err := guardProfilePublishTarget(resolvedProfilePath, resolvedStoreURL); err != nil {
		return err
	}
	s, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return err
	}
	defer closeCLIStore(s)
	report, err := verifyProfileBundle(ctx, s, resolvedProfilePath, maskStoreURL(resolvedStoreURL), profileVerifyOptions{
		RequireCaseRuns:     *requireCaseRuns,
		RequireWorkflowRuns: *requireWorkflowRuns,
	})
	if err != nil {
		if *jsonOutput && report.ProfileID != "" {
			if report.Error == "" {
				report.Error = err.Error()
			}
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			if encodeErr := encoder.Encode(report); encodeErr != nil {
				return encodeErr
			}
		}
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	printProfileVerify(report)
	return nil
}

func runProfileImport(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("profile import", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	return runConfigPublishWithFlags(ctx, flags, args, "Imported profile")
}

func runConfigPublish(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("config publish", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	return runConfigPublishWithFlags(ctx, flags, args, "Published config")
}

func runConfigPublishWithFlags(ctx context.Context, flags *flag.FlagSet, args []string, textPrefix string) error {
	from := flags.String("from", "", "Profile bundle path")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	auditOutput := flags.Bool("audit", false, "Run profile audit after import")
	requireAuditOK := flags.Bool("require-audit-ok", false, "Fail before writing the Store unless profile audit has no issues")
	force := flags.Bool("force", false, "Replace an installed profile when --from points to a packed archive")
	if err := flags.Parse(args); err != nil {
		return err
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	resolvedFrom, err := materializeProfileReference(*from, *profileHome, *force)
	if err != nil {
		return err
	}
	if err := guardProfilePublishTarget(resolvedFrom, resolvedStoreURL); err != nil {
		return err
	}
	s, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return err
	}
	defer closeCLIStore(s)
	report, err := publishProfileBundleToStore(ctx, s, resolvedFrom, maskStoreURL(resolvedStoreURL), *auditOutput, *requireAuditOK)
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	fmt.Printf("%s: %s\n", textPrefix, report.ProfileID)
	fmt.Printf("Digest: %s\n", report.BundleDigest)
	printProfileImportDiff(report.Diff)
	if report.Audit != nil {
		printProfileImportAudit(*report.Audit)
	}
	return nil
}

func publishProfileBundleToStore(ctx context.Context, s store.Store, from string, storePath string, auditOutput bool, requireAuditOK bool) (profileImportReport, error) {
	result, err := profilepublish.Publish(ctx, s, profilepublish.Options{
		Path:             from,
		RequireAuditOK:   requireAuditOK,
		UpsertReadModels: controlplane.UpsertProfileReadModels,
	})
	if err != nil {
		return profileImportReport{}, err
	}
	report := profileImportReport{
		ProfileID:     result.Bundle.ID,
		BundlePath:    from,
		BundleDigest:  result.Digest,
		Counts:        profileImportAssetCounts(result.Counts),
		Diff:          profileImportDiffFromCatalogs(result.PreviousCatalog, result.Catalog, result.HasPreviousCatalog),
		StorePath:     storePath,
		CatalogIndex:  profileCatalogIndexFromStore(result.CatalogIndex),
		ConfigVersion: profileConfigVersionFromStore(result.ConfigVersion),
		ReadModels:    result.ReadModels,
		ImportedAt:    result.ImportedAt,
	}
	if auditOutput {
		auditReport, err := profileaudit.Audit(ctx, profileaudit.Options{
			Bundle:     result.Bundle,
			BundlePath: from,
			Store:      s,
		})
		if err != nil {
			return profileImportReport{}, err
		}
		report.Audit = &auditReport
	}
	return report, nil
}

func verifyProfileBundle(ctx context.Context, s store.Store, profilePath string, storePath string, options profileVerifyOptions) (profileVerifyReport, error) {
	bundle, err := profile.Load(profilePath)
	if err != nil {
		return profileVerifyReport{}, err
	}
	auditReport, err := profileaudit.Audit(ctx, profileaudit.Options{
		Bundle:     bundle,
		BundlePath: profilePath,
	})
	if err != nil {
		return profileVerifyReport{}, err
	}
	if !auditReport.OK {
		return profileVerifyReport{}, fmt.Errorf("profile audit failed for profile %q: %s", bundle.ID, profileaudit.FailureSummary(auditReport))
	}
	publishReport, err := publishProfileBundleToStore(ctx, s, profilePath, storePath, true, true)
	if err != nil {
		return profileVerifyReport{}, err
	}
	verifyOptions := profileVerifyOptionsWithReadModels(options)
	checks, err := profileverify.PublishedChecks(ctx, s, bundle, profileverify.PublishedProfile{
		ProfileID:       publishReport.ProfileID,
		BundleDigest:    publishReport.BundleDigest,
		ConfigVersionID: publishReport.ConfigVersion.ID,
	}, verifyOptions)
	if err != nil {
		return profileVerifyReport{}, err
	}
	report := profileVerifyReport{
		OK:        profileverify.ChecksOK(checks),
		ProfileID: bundle.ID,
		Audit:     *publishReport.Audit,
		Publish:   publishReport,
		Summary:   profileverify.Summarize(checks, verifyOptions),
		Checks:    checks,
	}
	if !report.OK {
		report.Error = fmt.Sprintf("profile verification failed for profile %q: %s", bundle.ID, profileverify.FirstFailed(checks))
		return report, fmt.Errorf("profile verification failed for profile %q: %s", bundle.ID, profileverify.FirstFailed(checks))
	}
	return report, nil
}

func profileVerifyOptionsWithReadModels(options profileVerifyOptions) profileVerifyOptions {
	if len(options.ReadModelKeys) == 0 {
		options.ReadModelKeys = []string{profilecatalog.ReadModelInterfaceNodes, controlplane.ReadModelCatalog, controlplane.ReadModelDashboard}
	}
	return options
}

func readProfileCatalogIndex(ctx context.Context, storeURL string) (profileCatalogIndexReport, error) {
	runtime, err := openStore(ctx, storeURL)
	if err != nil {
		return profileCatalogIndexReport{}, err
	}
	defer closeCLIStore(runtime)
	index, err := runtime.GetProfileCatalogIndex(ctx)
	if err != nil {
		return profileCatalogIndexReport{}, err
	}
	report := profileCatalogIndexReport{
		ProfileID: index.ProfileID,
		IndexedAt: index.IndexedAt,
		Counts: profileImportCounts{
			Services:         index.Counts.Services,
			Workflows:        index.Counts.Workflows,
			InterfaceNodes:   index.Counts.InterfaceNodes,
			APICases:         index.Counts.APICases,
			RequestTemplates: index.Counts.RequestTemplates,
			CaseDependencies: index.Counts.CaseDependencies,
			WorkflowBindings: index.Counts.WorkflowBindings,
			Fixtures:         index.Counts.Fixtures,
		},
	}
	if version, err := runtime.GetActiveConfigVersion(ctx); err == nil {
		value := profileConfigVersionFromStore(version)
		report.ConfigVersion = &value
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return profileCatalogIndexReport{}, err
	}
	return report, nil
}
