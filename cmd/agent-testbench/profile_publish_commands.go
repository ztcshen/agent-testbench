package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profileaudit"
	"agent-testbench/internal/domain/profilecatalog"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

type profileImportReport struct {
	ProfileID     string               `json:"profileId"`
	BundlePath    string               `json:"bundlePath"`
	BundleDigest  string               `json:"bundleDigest"`
	Counts        profileImportCounts  `json:"counts"`
	Diff          profileImportDiff    `json:"diff"`
	StorePath     string               `json:"storePath"`
	CatalogIndex  profileCatalogIndex  `json:"catalogIndex"`
	ConfigVersion profileConfigVersion `json:"configVersion"`
	ReadModels    []string             `json:"readModels"`
	ImportedAt    time.Time            `json:"importedAt"`
	Audit         *profileaudit.Report `json:"audit,omitempty"`
}

type profileImportCounts struct {
	Services         int `json:"services"`
	Workflows        int `json:"workflows"`
	InterfaceNodes   int `json:"interfaceNodes"`
	APICases         int `json:"apiCases"`
	RequestTemplates int `json:"requestTemplates"`
	CaseDependencies int `json:"caseDependencies"`
	WorkflowBindings int `json:"workflowBindings"`
	Fixtures         int `json:"fixtures"`
}

type profileImportDiff struct {
	HasPreviousCatalog bool                         `json:"hasPreviousCatalog"`
	Before             profileImportCounts          `json:"before"`
	After              profileImportCounts          `json:"after"`
	APICases           profileImportCaseDiff        `json:"apiCases"`
	NodeCaseDeltas     []profileImportNodeCaseDelta `json:"nodeCaseDeltas,omitempty"`
}

type profileImportCaseDiff struct {
	Before  int      `json:"before"`
	After   int      `json:"after"`
	Added   []string `json:"added,omitempty"`
	Removed []string `json:"removed,omitempty"`
}

type profileImportNodeCaseDelta struct {
	NodeID string `json:"nodeId"`
	Before int    `json:"before"`
	After  int    `json:"after"`
	Delta  int    `json:"delta"`
}

type profileCatalogIndex struct {
	ProfileID string                    `json:"profileId"`
	IndexedAt time.Time                 `json:"indexedAt"`
	Counts    profileCatalogIndexCounts `json:"counts"`
}

type profileCatalogIndexReport struct {
	ProfileID     string                `json:"profileId"`
	IndexedAt     time.Time             `json:"indexedAt"`
	Counts        profileImportCounts   `json:"counts"`
	ConfigVersion *profileConfigVersion `json:"configVersion,omitempty"`
}

type profileCatalogIndexCounts struct {
	Services         int `json:"services"`
	Workflows        int `json:"workflows"`
	InterfaceNodes   int `json:"interfaceNodes"`
	APICases         int `json:"apiCases"`
	RequestTemplates int `json:"requestTemplates"`
	CaseDependencies int `json:"caseDependencies"`
	WorkflowBindings int `json:"workflowBindings"`
	Fixtures         int `json:"fixtures"`
	Templates        int `json:"templates"`
	TemplateConfigs  int `json:"templateConfigs"`
}

type profileConfigVersion struct {
	ID           string    `json:"id"`
	ProfileID    string    `json:"profileId"`
	SourcePath   string    `json:"sourcePath"`
	BundleDigest string    `json:"bundleDigest"`
	Active       bool      `json:"active"`
	PublishedAt  time.Time `json:"publishedAt"`
	CreatedAt    time.Time `json:"createdAt"`
}

type profileVerifyReport struct {
	OK        bool                 `json:"ok"`
	Error     string               `json:"error,omitempty"`
	ProfileID string               `json:"profileId"`
	Audit     profileaudit.Report  `json:"audit"`
	Publish   profileImportReport  `json:"publish"`
	Summary   profileVerifySummary `json:"summary"`
	Checks    []profileVerifyCheck `json:"checks"`
}

type profileVerifySummary struct {
	TotalChecks          int    `json:"totalChecks"`
	PassedChecks         int    `json:"passedChecks"`
	FailedChecks         int    `json:"failedChecks"`
	RequiredCaseRuns     bool   `json:"requiredCaseRuns"`
	RequiredWorkflowRuns bool   `json:"requiredWorkflowRuns"`
	FirstFailed          string `json:"firstFailed,omitempty"`
}

type profileVerifyCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

type profileVerifyOptions struct {
	RequireCaseRuns     bool
	RequireWorkflowRuns bool
}

func runProfileCatalogIndex(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("profile catalog-index", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	report, err := readProfileCatalogIndex(ctx, resolvedStoreURL)
	if err != nil {
		return err
	}
	if *jsonOutput {
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
	s, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return err
	}
	defer s.Close()
	resolvedProfilePath, err := materializeProfileReference(templatePackageReference(*templatePackagePath, *profilePath), *profileHome, *force)
	if err != nil {
		return err
	}
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
	s, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return err
	}
	defer s.Close()

	resolvedFrom, err := materializeProfileReference(*from, *profileHome, *force)
	if err != nil {
		return err
	}
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
	bundle, err := profile.Load(from)
	if err != nil {
		return profileImportReport{}, err
	}
	if requireAuditOK {
		auditReport, err := profileaudit.Audit(ctx, profileaudit.Options{
			Bundle:     bundle,
			BundlePath: from,
		})
		if err != nil {
			return profileImportReport{}, err
		}
		if !auditReport.OK {
			return profileImportReport{}, fmt.Errorf("profile audit failed for profile %q: %s", bundle.ID, profileaudit.FailureSummary(auditReport))
		}
	}
	digest, err := profile.BundleDigest(from)
	if err != nil {
		return profileImportReport{}, err
	}
	summary, err := json.Marshal(bundle.Counts())
	if err != nil {
		return profileImportReport{}, err
	}
	importedAt := time.Now().UTC()
	if _, err := s.UpsertProfileIndex(ctx, store.ProfileIndex{
		ProfileID:    bundle.ID,
		BundlePath:   from,
		BundleDigest: digest,
		SummaryJSON:  string(summary),
		ImportedAt:   importedAt,
	}); err != nil {
		return profileImportReport{}, err
	}
	catalog := profilecatalog.FromBundle(bundle, importedAt)
	previousCatalog, hasPreviousCatalog, err := readCurrentProfileCatalog(ctx, s)
	if err != nil {
		return profileImportReport{}, err
	}
	if err := s.ReplaceProfileCatalog(ctx, catalog); err != nil {
		return profileImportReport{}, err
	}
	configVersion, err := s.UpsertConfigVersion(ctx, store.ConfigVersion{
		ID:           configVersionID(bundle.ID, importedAt),
		ProfileID:    bundle.ID,
		SourcePath:   from,
		BundleDigest: digest,
		SummaryJSON:  string(summary),
		Active:       true,
		PublishedAt:  importedAt,
		CreatedAt:    importedAt,
	})
	if err != nil {
		return profileImportReport{}, err
	}
	readModelKeys, err := controlplane.UpsertProfileReadModels(ctx, s, catalog, configVersion.ID, importedAt)
	if err != nil {
		return profileImportReport{}, err
	}
	catalogIndex, err := s.GetProfileCatalogIndex(ctx)
	if err != nil {
		return profileImportReport{}, err
	}
	report := profileImportReport{
		ProfileID:     bundle.ID,
		BundlePath:    from,
		BundleDigest:  digest,
		Counts:        profileImportAssetCounts(bundle.Counts()),
		Diff:          profileImportDiffFromCatalogs(previousCatalog, catalog, hasPreviousCatalog),
		StorePath:     storePath,
		CatalogIndex:  profileCatalogIndexFromStore(catalogIndex),
		ConfigVersion: profileConfigVersionFromStore(configVersion),
		ReadModels:    readModelKeys,
		ImportedAt:    importedAt,
	}
	if auditOutput {
		auditReport, err := profileaudit.Audit(ctx, profileaudit.Options{
			Bundle:     bundle,
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
	checks, err := verifyPublishedProfile(ctx, s, bundle, publishReport, options)
	if err != nil {
		return profileVerifyReport{}, err
	}
	report := profileVerifyReport{
		OK:        profileChecksOK(checks),
		ProfileID: bundle.ID,
		Audit:     *publishReport.Audit,
		Publish:   publishReport,
		Summary:   summarizeProfileVerification(checks, options),
		Checks:    checks,
	}
	if !report.OK {
		report.Error = fmt.Sprintf("profile verification failed for profile %q: %s", bundle.ID, firstFailedProfileCheck(checks))
		return report, fmt.Errorf("profile verification failed for profile %q: %s", bundle.ID, firstFailedProfileCheck(checks))
	}
	return report, nil
}

func summarizeProfileVerification(checks []profileVerifyCheck, options profileVerifyOptions) profileVerifySummary {
	summary := profileVerifySummary{
		TotalChecks:          len(checks),
		RequiredCaseRuns:     options.RequireCaseRuns,
		RequiredWorkflowRuns: options.RequireWorkflowRuns,
	}
	for _, check := range checks {
		if check.OK {
			summary.PassedChecks++
			continue
		}
		summary.FailedChecks++
		if summary.FirstFailed == "" {
			summary.FirstFailed = check.Name
		}
	}
	return summary
}

func verifyPublishedProfile(ctx context.Context, s store.Store, bundle profile.Bundle, report profileImportReport, options profileVerifyOptions) ([]profileVerifyCheck, error) {
	checks := make([]profileVerifyCheck, 0, 6)
	index, err := s.GetProfileIndex(ctx, report.ProfileID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			checks = appendProfileCheck(checks, "profile-index", false, "profile index was not written")
			return checks, nil
		}
		return nil, err
	}
	checks = appendProfileCheck(checks, "profile-index", index.BundleDigest == report.BundleDigest, "profile index digest matches published bundle")

	catalogIndex, err := s.GetProfileCatalogIndex(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			checks = appendProfileCheck(checks, "catalog-index", false, "catalog index was not written")
		} else {
			return nil, err
		}
	} else {
		checks = appendProfileCheck(checks, "catalog-index", catalogIndex.ProfileID == report.ProfileID, "catalog index points to active profile")
	}

	activeConfig, err := s.GetActiveConfigVersion(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			checks = appendProfileCheck(checks, "active-config", false, "active config version was not written")
		} else {
			return nil, err
		}
	} else {
		ok := activeConfig.ID == report.ConfigVersion.ID && activeConfig.ProfileID == report.ProfileID && activeConfig.BundleDigest == report.BundleDigest
		checks = appendProfileCheck(checks, "active-config", ok, "active config version matches published bundle")
	}

	for _, key := range []string{profilecatalog.ReadModelInterfaceNodes, controlplane.ReadModelCatalog, controlplane.ReadModelDashboard} {
		model, err := s.GetReadModel(ctx, report.ProfileID, key)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				checks = appendProfileCheck(checks, "read-model:"+key, false, "read model was not written")
				continue
			}
			return nil, err
		}
		ok := model.ConfigVersionID == report.ConfigVersion.ID && strings.TrimSpace(model.PayloadJSON) != ""
		checks = appendProfileCheck(checks, "read-model:"+key, ok, "read model exists for published config version")
	}
	if options.RequireCaseRuns {
		caseRunChecks, err := verifyProfileAPICaseRuns(ctx, s, bundle)
		if err != nil {
			return nil, err
		}
		checks = append(checks, caseRunChecks...)
	}
	if options.RequireWorkflowRuns {
		workflowChecks, err := verifyProfileWorkflowRuns(ctx, s, bundle)
		if err != nil {
			return nil, err
		}
		checks = append(checks, workflowChecks...)
	}
	return checks, nil
}

func verifyProfileWorkflowRuns(ctx context.Context, s store.Store, bundle profile.Bundle) ([]profileVerifyCheck, error) {
	if len(bundle.Workflows) == 0 {
		return []profileVerifyCheck{{Name: "workflow-runs", OK: true, Detail: "profile declares no workflows"}}, nil
	}
	runs, err := s.ListRuns(ctx)
	if err != nil {
		return nil, err
	}
	latestByWorkflow := map[string]store.Run{}
	for _, item := range runs {
		if item.WorkflowID == "" {
			continue
		}
		current, ok := latestByWorkflow[item.WorkflowID]
		if !ok || item.CreatedAt.After(current.CreatedAt) || (item.CreatedAt.Equal(current.CreatedAt) && item.ID > current.ID) {
			latestByWorkflow[item.WorkflowID] = item
		}
	}
	checks := make([]profileVerifyCheck, 0, len(bundle.Workflows))
	for _, item := range bundle.Workflows {
		run, ok := latestByWorkflow[item.ID]
		if !ok || !isPassedStatus(run.Status) {
			checks = appendProfileCheck(checks, "workflow-run:"+item.ID, false, "no passed run recorded in Store")
			continue
		}
		checks = appendProfileCheck(checks, "workflow-run:"+item.ID, true, "latest Workflow run passed")
	}
	return checks, nil
}

func verifyProfileAPICaseRuns(ctx context.Context, s store.Store, bundle profile.Bundle) ([]profileVerifyCheck, error) {
	if len(bundle.APICases) == 0 {
		return []profileVerifyCheck{{Name: "api-case-runs", OK: true, Detail: "profile declares no API cases"}}, nil
	}
	latestStore, ok := s.(interface {
		ListLatestAPICaseRuns(context.Context) ([]store.APICaseRun, error)
	})
	if !ok {
		return nil, errors.New("runtime store does not support latest API case run lookup")
	}
	latestRuns, err := latestStore.ListLatestAPICaseRuns(ctx)
	if err != nil {
		return nil, err
	}
	latestByCase := map[string]store.APICaseRun{}
	for _, item := range latestRuns {
		latestByCase[item.CaseID] = item
	}
	checks := make([]profileVerifyCheck, 0, len(bundle.APICases))
	for _, item := range bundle.APICases {
		run, ok := latestByCase[item.ID]
		if !ok || !isPassedStatus(run.Status) {
			checks = appendProfileCheck(checks, "api-case-run:"+item.ID, false, "no passed run recorded in Store")
			continue
		}
		checks = appendProfileCheck(checks, "api-case-run:"+item.ID, true, "latest API case run passed")
	}
	return checks, nil
}

func isPassedStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pass", "passed", "success", "ok":
		return true
	default:
		return false
	}
}

func appendProfileCheck(checks []profileVerifyCheck, name string, ok bool, detail string) []profileVerifyCheck {
	return append(checks, profileVerifyCheck{Name: name, OK: ok, Detail: detail})
}

func profileChecksOK(checks []profileVerifyCheck) bool {
	if len(checks) == 0 {
		return false
	}
	for _, check := range checks {
		if !check.OK {
			return false
		}
	}
	return true
}

func firstFailedProfileCheck(checks []profileVerifyCheck) string {
	for _, check := range checks {
		if !check.OK {
			return check.Name + ": " + check.Detail
		}
	}
	return "no checks passed"
}

func profileImportAssetCounts(counts profile.Counts) profileImportCounts {
	return profileImportCounts{
		Services:         counts.Services,
		Workflows:        counts.Workflows,
		InterfaceNodes:   counts.InterfaceNodes,
		APICases:         counts.APICases,
		RequestTemplates: counts.RequestTemplates,
		CaseDependencies: counts.CaseDependencies,
		WorkflowBindings: counts.WorkflowBindings,
		Fixtures:         counts.Fixtures,
	}
}

func readCurrentProfileCatalog(ctx context.Context, s store.Store) (store.ProfileCatalog, bool, error) {
	catalog, err := s.GetProfileCatalog(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return store.ProfileCatalog{}, false, nil
		}
		return store.ProfileCatalog{}, false, err
	}
	return catalog, true, nil
}

func profileImportDiffFromCatalogs(before store.ProfileCatalog, after store.ProfileCatalog, hasBefore bool) profileImportDiff {
	diff := profileImportDiff{
		HasPreviousCatalog: hasBefore,
		Before:             profileImportCountsFromCatalog(before),
		After:              profileImportCountsFromCatalog(after),
		APICases: profileImportCaseDiff{
			Before: len(before.APICases),
			After:  len(after.APICases),
		},
	}
	if !hasBefore {
		diff.APICases.Before = 0
		diff.Before = profileImportCounts{}
	}
	beforeIDs := map[string]bool{}
	for _, item := range before.APICases {
		beforeIDs[item.ID] = true
	}
	afterIDs := map[string]bool{}
	for _, item := range after.APICases {
		afterIDs[item.ID] = true
		if hasBefore && !beforeIDs[item.ID] {
			diff.APICases.Added = append(diff.APICases.Added, item.ID)
		}
	}
	if hasBefore {
		for _, item := range before.APICases {
			if !afterIDs[item.ID] {
				diff.APICases.Removed = append(diff.APICases.Removed, item.ID)
			}
		}
	}
	sort.Strings(diff.APICases.Added)
	sort.Strings(diff.APICases.Removed)
	diff.NodeCaseDeltas = profileImportNodeCaseDeltas(before.APICases, after.APICases, hasBefore)
	return diff
}

func profileImportCountsFromCatalog(catalog store.ProfileCatalog) profileImportCounts {
	return profileImportCounts{
		Services:         len(catalog.Services),
		Workflows:        len(catalog.Workflows),
		InterfaceNodes:   len(catalog.InterfaceNodes),
		APICases:         len(catalog.APICases),
		RequestTemplates: len(catalog.RequestTemplates),
		CaseDependencies: len(catalog.CaseDependencies),
		WorkflowBindings: len(catalog.WorkflowBindings),
		Fixtures:         len(catalog.Fixtures),
	}
}

func profileImportNodeCaseDeltas(before []store.CatalogAPICase, after []store.CatalogAPICase, hasBefore bool) []profileImportNodeCaseDelta {
	beforeCounts := map[string]int{}
	if hasBefore {
		for _, item := range before {
			beforeCounts[firstNonEmpty(item.NodeID, "(none)")]++
		}
	}
	afterCounts := map[string]int{}
	for _, item := range after {
		afterCounts[firstNonEmpty(item.NodeID, "(none)")]++
	}
	nodeIDs := map[string]bool{}
	for nodeID := range beforeCounts {
		nodeIDs[nodeID] = true
	}
	for nodeID := range afterCounts {
		nodeIDs[nodeID] = true
	}
	out := make([]profileImportNodeCaseDelta, 0, len(nodeIDs))
	for _, nodeID := range sortedBoolMapKeys(nodeIDs) {
		beforeCount := beforeCounts[nodeID]
		afterCount := afterCounts[nodeID]
		if hasBefore && beforeCount == afterCount {
			continue
		}
		out = append(out, profileImportNodeCaseDelta{
			NodeID: nodeID,
			Before: beforeCount,
			After:  afterCount,
			Delta:  afterCount - beforeCount,
		})
	}
	return out
}

func sortedBoolMapKeys(items map[string]bool) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func printProfileImportDiff(diff profileImportDiff) {
	if !diff.HasPreviousCatalog {
		fmt.Printf("API Cases: %d\n", diff.APICases.After)
		return
	}
	fmt.Printf("API Cases: %d -> %d\n", diff.APICases.Before, diff.APICases.After)
	for _, item := range diff.NodeCaseDeltas {
		if item.Delta == 0 {
			continue
		}
		fmt.Printf("- %s: %d -> %d (%+d)\n", item.NodeID, item.Before, item.After, item.Delta)
	}
	if len(diff.APICases.Added) > 0 {
		fmt.Printf("Added Cases: %d\n", len(diff.APICases.Added))
	}
	if len(diff.APICases.Removed) > 0 {
		fmt.Printf("Removed Cases: %d\n", len(diff.APICases.Removed))
	}
}

func profileCatalogIndexFromStore(index store.ProfileCatalogIndex) profileCatalogIndex {
	return profileCatalogIndex{
		ProfileID: index.ProfileID,
		IndexedAt: index.IndexedAt,
		Counts: profileCatalogIndexCounts{
			Services:         index.Counts.Services,
			Workflows:        index.Counts.Workflows,
			InterfaceNodes:   index.Counts.InterfaceNodes,
			APICases:         index.Counts.APICases,
			RequestTemplates: index.Counts.RequestTemplates,
			CaseDependencies: index.Counts.CaseDependencies,
			WorkflowBindings: index.Counts.WorkflowBindings,
			Fixtures:         index.Counts.Fixtures,
			Templates:        index.Counts.Templates,
			TemplateConfigs:  index.Counts.TemplateConfigs,
		},
	}
}

func profileConfigVersionFromStore(item store.ConfigVersion) profileConfigVersion {
	return profileConfigVersion{
		ID:           item.ID,
		ProfileID:    item.ProfileID,
		SourcePath:   item.SourcePath,
		BundleDigest: item.BundleDigest,
		Active:       item.Active,
		PublishedAt:  item.PublishedAt,
		CreatedAt:    item.CreatedAt,
	}
}

func configVersionID(profileID string, publishedAt time.Time) string {
	safeProfileID := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-").Replace(strings.TrimSpace(profileID))
	if safeProfileID == "" {
		safeProfileID = "profile"
	}
	return "config." + safeProfileID + "." + publishedAt.UTC().Format("20060102T150405.000000000Z")
}

func printProfileImportAudit(report profileaudit.Report) {
	fmt.Printf("Audit OK: %t\n", report.OK)
	fmt.Printf("Audit Issues: %d\n", report.IssueCount)
	for _, item := range report.Issues {
		fmt.Printf("- [%s] %s %s %s: %s\n", item.Severity, item.Code, item.SubjectType, item.SubjectID, item.Message)
	}
}

func readProfileCatalogIndex(ctx context.Context, storeURL string) (profileCatalogIndexReport, error) {
	runtime, err := openStore(ctx, storeURL)
	if err != nil {
		return profileCatalogIndexReport{}, err
	}
	defer runtime.Close()
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

func printProfileCatalogIndex(report profileCatalogIndexReport) {
	fmt.Printf("Template Package Catalog Index: %s\n", report.ProfileID)
	fmt.Printf("Indexed At: %s\n", report.IndexedAt.Format(time.RFC3339))
	fmt.Printf("Services: %d\n", report.Counts.Services)
	fmt.Printf("Workflows: %d\n", report.Counts.Workflows)
	fmt.Printf("Interface Nodes: %d\n", report.Counts.InterfaceNodes)
	fmt.Printf("API Cases: %d\n", report.Counts.APICases)
	fmt.Printf("Request Templates: %d\n", report.Counts.RequestTemplates)
	if report.ConfigVersion != nil {
		fmt.Printf("Config Version: %s\n", report.ConfigVersion.ID)
	}
}

func printProfileVerify(report profileVerifyReport) {
	fmt.Printf("Profile Verification: %s\n", report.ProfileID)
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Audit OK: %t\n", report.Audit.OK)
	fmt.Printf("Published Config: %s\n", report.Publish.ConfigVersion.ID)
	fmt.Printf("Read Models: %s\n", strings.Join(report.Publish.ReadModels, ", "))
	fmt.Printf("Checks: %d passed / %d total", report.Summary.PassedChecks, report.Summary.TotalChecks)
	if report.Summary.FailedChecks > 0 {
		fmt.Printf(" (%d failed", report.Summary.FailedChecks)
		if report.Summary.FirstFailed != "" {
			fmt.Printf(", first failed: %s", report.Summary.FirstFailed)
		}
		fmt.Print(")")
	}
	fmt.Println()
	fmt.Printf("Runtime Gates: api-cases=%t workflows=%t\n", report.Summary.RequiredCaseRuns, report.Summary.RequiredWorkflowRuns)
	fmt.Println("Checks:")
	for _, check := range report.Checks {
		fmt.Printf("- %s: %t (%s)\n", check.Name, check.OK, check.Detail)
	}
}

func printProfile(bundle profile.Bundle) {
	counts := bundle.Counts()
	fmt.Printf("Profile: %s\n", bundle.ID)
	fmt.Printf("Display Name: %s\n", bundle.DisplayName)
	fmt.Printf("Services: %d\n", counts.Services)
	fmt.Printf("Workflows: %d\n", counts.Workflows)
	fmt.Printf("Interface Nodes: %d\n", counts.InterfaceNodes)
	fmt.Printf("API Cases: %d\n", counts.APICases)
	fmt.Printf("Request Templates: %d\n", counts.RequestTemplates)
	fmt.Printf("Case Dependencies: %d\n", counts.CaseDependencies)
	fmt.Printf("Workflow Bindings: %d\n", counts.WorkflowBindings)
	fmt.Printf("Fixtures: %d\n", counts.Fixtures)
}

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
		defer s.Close()
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
		defer s.Close()
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
