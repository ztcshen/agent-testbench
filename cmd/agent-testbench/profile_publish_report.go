package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profileaudit"
	"agent-testbench/internal/profileverify"
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

type profileCatalogListReport struct {
	OK    bool                  `json:"ok"`
	Count int                   `json:"count"`
	Items []profileCatalogIndex `json:"items"`
}

type profileCatalogRestoreReport struct {
	OK            bool                  `json:"ok"`
	ProfileID     string                `json:"profileId"`
	RestoredAt    time.Time             `json:"restoredAt"`
	Before        profileCatalogIndex   `json:"before"`
	After         profileCatalogIndex   `json:"after"`
	ConfigVersion *profileConfigVersion `json:"configVersion,omitempty"`
	Notes         []string              `json:"notes,omitempty"`
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

type profileVerifySummary = profileverify.Summary
type profileVerifyCheck = profileverify.Check
type profileVerifyOptions = profileverify.Options

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

func printProfileImportAudit(report profileaudit.Report) {
	fmt.Printf("Audit OK: %t\n", report.OK)
	fmt.Printf("Audit Issues: %d\n", report.IssueCount)
	for _, item := range report.Issues {
		fmt.Printf("- [%s] %s %s %s: %s\n", item.Severity, item.Code, item.SubjectType, item.SubjectID, item.Message)
	}
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

func printProfileCatalogList(report profileCatalogListReport) {
	fmt.Printf("Profile Catalogs: %d\n", report.Count)
	for _, item := range report.Items {
		fmt.Printf("%s\t%s\tworkflows=%d\tapiCases=%d\n", item.ProfileID, item.IndexedAt.Format(time.RFC3339), item.Counts.Workflows, item.Counts.APICases)
	}
}

func printProfileCatalogRestore(report profileCatalogRestoreReport) {
	fmt.Printf("Restored Profile Catalog: %s\n", report.ProfileID)
	fmt.Printf("Before: %s\n", report.Before.ProfileID)
	fmt.Printf("After: %s\n", report.After.ProfileID)
	if report.ConfigVersion != nil {
		fmt.Printf("Config Version: %s\n", report.ConfigVersion.ID)
	}
	for _, note := range report.Notes {
		fmt.Printf("Note: %s\n", note)
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
