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
