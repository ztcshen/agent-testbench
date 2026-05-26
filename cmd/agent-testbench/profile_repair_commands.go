package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-testbench/internal/domain/profile"
)

type profileDoctorReport struct {
	OK          bool                 `json:"ok"`
	ProfileID   string               `json:"profileId,omitempty"`
	ProfilePath string               `json:"profilePath"`
	CaseID      string               `json:"caseId"`
	Checks      []profileDoctorCheck `json:"checks"`
}

type profileDoctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func runProfileDoctor(args []string) error {
	flags := flag.NewFlagSet("profile doctor", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	templatePackagePath := flags.String("template-package", "", "Template package path or installed template package id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	caseID := flags.String("case-id", "", "API case id to inspect")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	resolvedProfilePath, err := resolveProfileReference(templatePackageReference(*templatePackagePath, *profilePath), *profileHome)
	if err != nil {
		return err
	}
	report := profileDoctor(resolvedProfilePath, *caseID)
	if *jsonOutput {
		if err := writeIndentedJSON(report); err != nil {
			return err
		}
		if !report.OK {
			return errors.New("profile doctor found issues")
		}
		return nil
	}
	printProfileDoctor(report)
	if !report.OK {
		return errors.New("profile doctor found issues")
	}
	return nil
}

func profileDoctor(profilePath string, caseID string) profileDoctorReport {
	report := profileDoctorReport{
		ProfilePath: profilePath,
		CaseID:      strings.TrimSpace(caseID),
		OK:          true,
	}
	if report.CaseID == "" {
		return appendProfileDoctorCheck(report, "case-id", false, "--case-id is required")
	}
	bundle, err := profile.Load(profilePath)
	if err != nil {
		return appendProfileDoctorCheck(report, "profile-load", false, err.Error())
	}
	report.ProfileID = bundle.ID
	report = appendProfileDoctorCheck(report, "profile-load", true, "profile loaded")
	apiCase, foundCase := findProfileAPICase(bundle.APICases, report.CaseID)
	report = appendProfileDoctorCheck(report, "case-catalog", foundCase, "case is present in loaded profile catalog")
	rawCatalogIDs := loadRawCatalogCaseIDs(bundle.BaseDir)
	if len(rawCatalogIDs) > 0 {
		report = appendProfileDoctorCheck(report, "catalog-json-entry", rawCatalogIDs[report.CaseID], "case is present in catalog.json interfaceNodeCases")
	}
	caseFile := profileCaseFilePath(bundle.BaseDir, apiCase)
	if report.CaseID != "" {
		_, err := os.Stat(caseFile)
		report = appendProfileDoctorCheck(report, "case-file", err == nil, caseFile)
	}
	if !foundCase {
		return report
	}
	if strings.TrimSpace(apiCase.NodeID) != "" {
		_, foundNode := findProfileInterfaceNode(bundle.InterfaceNodes, apiCase.NodeID)
		report = appendProfileDoctorCheck(report, "interface-node", foundNode, "node "+apiCase.NodeID+" exists")
	}
	if strings.TrimSpace(apiCase.RequestTemplateID) != "" {
		_, foundTemplate := findProfileRequestTemplate(bundle.RequestTemplates, apiCase.RequestTemplateID)
		report = appendProfileDoctorCheck(report, "request-template", foundTemplate, "template "+apiCase.RequestTemplateID+" exists")
	}
	for _, item := range bundle.CaseDependencies {
		if item.CaseID != apiCase.ID {
			continue
		}
		_, foundFixture := findProfileFixture(bundle.Fixtures, item.FixtureID)
		report = appendProfileDoctorCheck(report, "fixture:"+item.FixtureID, foundFixture, "dependency "+item.ID+" fixture exists")
	}
	if strings.TrimSpace(apiCase.PatchJSON) != "" {
		report = appendProfileDoctorCheck(report, "patch-json", validJSONObjectOrArray(apiCase.PatchJSON), "patchJson parses as JSON")
	}
	if strings.TrimSpace(apiCase.ExpectedJSON) != "" {
		report = appendProfileDoctorCheck(report, "expected-json", validJSONObjectOrArray(apiCase.ExpectedJSON), "expectedJson parses as JSON")
	}
	return report
}

func appendProfileDoctorCheck(report profileDoctorReport, name string, ok bool, detail string) profileDoctorReport {
	if !ok {
		report.OK = false
	}
	report.Checks = append(report.Checks, profileDoctorCheck{Name: name, OK: ok, Detail: detail})
	return report
}

func printProfileDoctor(report profileDoctorReport) {
	fmt.Println("Profile Doctor")
	fmt.Printf("Profile: %s\n", firstNonEmpty(report.ProfileID, report.ProfilePath))
	fmt.Printf("Case: %s\n", report.CaseID)
	fmt.Printf("OK: %t\n", report.OK)
	for _, check := range report.Checks {
		status := "ok"
		if !check.OK {
			status = "issue"
		}
		fmt.Printf("- %s [%s] %s\n", check.Name, status, check.Detail)
	}
}

type profileRepairReport struct {
	OK           bool                 `json:"ok"`
	Applied      bool                 `json:"applied"`
	ProfilePath  string               `json:"profilePath"`
	ManifestPath string               `json:"manifestPath"`
	Summary      profileRepairSummary `json:"summary"`
	Items        []profileRepairItem  `json:"items"`
	Warnings     []string             `json:"warnings,omitempty"`
}

type profileRepairSummary struct {
	CatalogCasesRestored int `json:"catalogCasesRestored"`
	CaseFilesRestored    int `json:"caseFilesRestored"`
	AlreadyPresent       int `json:"alreadyPresent"`
	ChangedFiles         int `json:"changedFiles"`
}

type profileRepairItem struct {
	Kind   string `json:"kind"`
	ID     string `json:"id,omitempty"`
	Path   string `json:"path,omitempty"`
	Action string `json:"action"`
}

type profileRepairManifest struct {
	ProfilePath  string                     `json:"profilePath"`
	CatalogPath  string                     `json:"catalogPath"`
	CaseIDs      []string                   `json:"caseIds"`
	CatalogCases []json.RawMessage          `json:"catalogCases"`
	CaseFiles    map[string]string          `json:"caseFiles"`
	CaseFileJSON map[string]json.RawMessage `json:"caseFileJson"`
}

func runProfileRepair(args []string) error {
	flags := flag.NewFlagSet("profile repair", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	manifestPath := flags.String("from-manifest", "", "Repair manifest path")
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	apply := flags.Bool("apply", false, "Write repaired profile files")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	report, err := profileRepair(*manifestPath, *profilePath, *profileHome, *apply)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printProfileRepair(report)
	return nil
}

func profileRepair(manifestPath string, profileRef string, profileHome string, apply bool) (profileRepairReport, error) {
	manifestPath = strings.TrimSpace(manifestPath)
	if manifestPath == "" {
		return profileRepairReport{}, errors.New("--from-manifest is required")
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return profileRepairReport{}, fmt.Errorf("read repair manifest: %w", err)
	}
	var manifest profileRepairManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return profileRepairReport{}, fmt.Errorf("decode repair manifest: %w", err)
	}
	resolvedProfilePath := strings.TrimSpace(profileRef)
	if resolvedProfilePath != "" {
		resolvedProfilePath, err = resolveProfileReference(resolvedProfilePath, profileHome)
		if err != nil {
			return profileRepairReport{}, err
		}
	} else {
		resolvedProfilePath = strings.TrimSpace(manifest.ProfilePath)
	}
	if resolvedProfilePath == "" {
		return profileRepairReport{}, errors.New("profile repair needs --profile or manifest profilePath")
	}
	catalogPath := profileRepairCatalogPath(resolvedProfilePath, manifest)
	report := profileRepairReport{OK: true, Applied: apply, ProfilePath: resolvedProfilePath, ManifestPath: manifestPath}
	catalogRaw, err := os.ReadFile(catalogPath)
	if err != nil {
		return profileRepairReport{}, fmt.Errorf("read profile catalog: %w", err)
	}
	var catalog map[string]any
	if err := json.Unmarshal(catalogRaw, &catalog); err != nil {
		return profileRepairReport{}, fmt.Errorf("decode profile catalog: %w", err)
	}
	cases := rawJSONListFromAny(catalog["interfaceNodeCases"])
	byID := map[string]json.RawMessage{}
	for _, rawCase := range cases {
		id := jsonID(rawCase)
		if id != "" {
			byID[id] = rawCase
		}
	}
	for _, rawCase := range manifest.CatalogCases {
		id := jsonID(rawCase)
		if id == "" {
			report.Warnings = append(report.Warnings, "skipped catalog case without id")
			continue
		}
		action := "already-present"
		if _, ok := byID[id]; !ok {
			cases = append(cases, rawCase)
			byID[id] = rawCase
			action = "restore"
			report.Summary.CatalogCasesRestored++
		} else {
			report.Summary.AlreadyPresent++
		}
		report.Items = append(report.Items, profileRepairItem{Kind: "catalog-case", ID: id, Action: action})
	}
	fileContents := profileRepairCaseFiles(manifest)
	for sourcePath, content := range fileContents {
		targetPath := profileRepairCaseFilePath(resolvedProfilePath, manifest.ProfilePath, sourcePath)
		action := "already-present"
		current, err := os.ReadFile(targetPath)
		if err != nil || string(current) != content {
			action = "restore"
			report.Summary.CaseFilesRestored++
			if apply {
				if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
					return profileRepairReport{}, err
				}
				if err := os.WriteFile(targetPath, []byte(ensureTrailingNewline(content)), 0o644); err != nil {
					return profileRepairReport{}, err
				}
			}
		} else {
			report.Summary.AlreadyPresent++
		}
		report.Items = append(report.Items, profileRepairItem{Kind: "case-file", Path: targetPath, Action: action})
	}
	if apply && report.Summary.CatalogCasesRestored > 0 {
		nextCases := make([]any, 0, len(cases))
		for _, rawCase := range cases {
			var value any
			if err := json.Unmarshal(rawCase, &value); err != nil {
				return profileRepairReport{}, err
			}
			nextCases = append(nextCases, value)
		}
		catalog["interfaceNodeCases"] = nextCases
		out, err := json.MarshalIndent(catalog, "", "  ")
		if err != nil {
			return profileRepairReport{}, err
		}
		if err := os.WriteFile(catalogPath, append(out, '\n'), 0o644); err != nil {
			return profileRepairReport{}, err
		}
	}
	if report.Summary.CatalogCasesRestored > 0 && apply {
		report.Summary.ChangedFiles++
	}
	if report.Summary.CaseFilesRestored > 0 && apply {
		report.Summary.ChangedFiles += report.Summary.CaseFilesRestored
	}
	return report, nil
}

func printProfileRepair(report profileRepairReport) {
	fmt.Println("Profile Repair")
	fmt.Printf("Profile: %s\n", report.ProfilePath)
	fmt.Printf("Applied: %t\n", report.Applied)
	fmt.Printf("Catalog Cases Restored: %d\n", report.Summary.CatalogCasesRestored)
	fmt.Printf("Case Files Restored: %d\n", report.Summary.CaseFilesRestored)
	for _, item := range report.Items {
		target := firstNonEmpty(item.ID, item.Path)
		fmt.Printf("- %s %s: %s\n", item.Kind, target, item.Action)
	}
}

func findProfileAPICase(items []profile.APICase, id string) (profile.APICase, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return profile.APICase{}, false
}

func findProfileInterfaceNode(items []profile.InterfaceNode, id string) (profile.InterfaceNode, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return profile.InterfaceNode{}, false
}

func findProfileRequestTemplate(items []profile.RequestTemplate, id string) (profile.RequestTemplate, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return profile.RequestTemplate{}, false
}

func findProfileFixture(items []profile.Fixture, id string) (profile.Fixture, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return profile.Fixture{}, false
}

func profileCaseFilePath(baseDir string, apiCase profile.APICase) string {
	if strings.TrimSpace(apiCase.CasePath) != "" {
		if filepath.IsAbs(apiCase.CasePath) {
			return apiCase.CasePath
		}
		return filepath.Join(baseDir, apiCase.CasePath)
	}
	return filepath.Join(baseDir, "cases", apiCase.ID+".json")
}

func loadRawCatalogCaseIDs(baseDir string) map[string]bool {
	out := map[string]bool{}
	raw, err := os.ReadFile(filepath.Join(baseDir, "catalog.json"))
	if err != nil {
		return out
	}
	var payload struct {
		InterfaceNodeCases []json.RawMessage `json:"interfaceNodeCases"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return out
	}
	for _, item := range payload.InterfaceNodeCases {
		if id := jsonID(item); id != "" {
			out[id] = true
		}
	}
	return out
}

func validJSONObjectOrArray(value string) bool {
	var parsed any
	if json.Unmarshal([]byte(value), &parsed) != nil {
		return false
	}
	switch parsed.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}

func profileRepairCatalogPath(profilePath string, manifest profileRepairManifest) string {
	if strings.TrimSpace(manifest.CatalogPath) != "" {
		if filepath.IsAbs(manifest.CatalogPath) {
			return manifest.CatalogPath
		}
		if strings.TrimSpace(manifest.ProfilePath) != "" {
			if rel, err := filepath.Rel(manifest.ProfilePath, manifest.CatalogPath); err == nil && !strings.HasPrefix(rel, "..") {
				return filepath.Join(profilePath, rel)
			}
		}
		return manifest.CatalogPath
	}
	return filepath.Join(profilePath, "catalog.json")
}

func jsonID(raw json.RawMessage) string {
	var payload struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return ""
	}
	return strings.TrimSpace(payload.ID)
}

func profileRepairCaseFiles(manifest profileRepairManifest) map[string]string {
	out := map[string]string{}
	for path, content := range manifest.CaseFiles {
		out[path] = content
	}
	for path, raw := range manifest.CaseFileJSON {
		out[path] = string(raw)
	}
	return out
}

func profileRepairCaseFilePath(profilePath string, manifestProfilePath string, sourcePath string) string {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return filepath.Join(profilePath, "cases", "case.json")
	}
	if strings.TrimSpace(manifestProfilePath) != "" {
		if rel, err := filepath.Rel(manifestProfilePath, sourcePath); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return filepath.Join(profilePath, rel)
		}
	}
	if filepath.IsAbs(sourcePath) {
		return sourcePath
	}
	return filepath.Join(profilePath, sourcePath)
}

func ensureTrailingNewline(value string) string {
	if strings.HasSuffix(value, "\n") {
		return value
	}
	return value + "\n"
}
