package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
	context, err := newProfileRepairContext(manifestPath, profileRef, profileHome, apply)
	if err != nil {
		return profileRepairReport{}, err
	}
	report := profileRepairReport{OK: true, Applied: apply, ProfilePath: context.profilePath, ManifestPath: context.manifestPath}
	cases := profileRepairCatalogCases(context.catalog, context.manifest, &report)
	if err := profileRepairCaseFileItems(context, &report); err != nil {
		return profileRepairReport{}, err
	}
	if err := profileRepairWriteCatalog(context, cases, &report); err != nil {
		return profileRepairReport{}, err
	}
	profileRepairFinalizeSummary(&report)
	return report, nil
}

type profileRepairContext struct {
	manifestPath string
	profilePath  string
	catalogPath  string
	apply        bool
	manifest     profileRepairManifest
	catalog      map[string]any
}

func newProfileRepairContext(manifestPath string, profileRef string, profileHome string, apply bool) (profileRepairContext, error) {
	manifestPath = strings.TrimSpace(manifestPath)
	if manifestPath == "" {
		return profileRepairContext{}, errors.New("--from-manifest is required")
	}
	manifest, err := readProfileRepairManifest(manifestPath)
	if err != nil {
		return profileRepairContext{}, err
	}
	resolvedProfilePath, err := resolveProfileRepairPath(profileRef, profileHome, manifest)
	if err != nil {
		return profileRepairContext{}, err
	}
	catalogPath := profileRepairCatalogPath(resolvedProfilePath, manifest)
	catalog, err := readProfileRepairCatalog(catalogPath)
	if err != nil {
		return profileRepairContext{}, err
	}
	return profileRepairContext{
		manifestPath: manifestPath,
		profilePath:  resolvedProfilePath,
		catalogPath:  catalogPath,
		apply:        apply,
		manifest:     manifest,
		catalog:      catalog,
	}, nil
}

func readProfileRepairManifest(manifestPath string) (profileRepairManifest, error) {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return profileRepairManifest{}, fmt.Errorf("read repair manifest: %w", err)
	}
	var manifest profileRepairManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return profileRepairManifest{}, fmt.Errorf("decode repair manifest: %w", err)
	}
	return manifest, nil
}

func resolveProfileRepairPath(profileRef string, profileHome string, manifest profileRepairManifest) (string, error) {
	resolvedProfilePath := strings.TrimSpace(profileRef)
	if resolvedProfilePath != "" {
		var err error
		resolvedProfilePath, err = resolveProfileReference(resolvedProfilePath, profileHome)
		if err != nil {
			return "", err
		}
	} else {
		resolvedProfilePath = strings.TrimSpace(manifest.ProfilePath)
	}
	if resolvedProfilePath == "" {
		return "", errors.New("profile repair needs --profile or manifest profilePath")
	}
	return resolvedProfilePath, nil
}

func readProfileRepairCatalog(catalogPath string) (map[string]any, error) {
	catalogRaw, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("read profile catalog: %w", err)
	}
	var catalog map[string]any
	if err := json.Unmarshal(catalogRaw, &catalog); err != nil {
		return nil, fmt.Errorf("decode profile catalog: %w", err)
	}
	return catalog, nil
}

func profileRepairCatalogCases(catalog map[string]any, manifest profileRepairManifest, report *profileRepairReport) []json.RawMessage {
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
	return cases
}

func profileRepairCaseFileItems(context profileRepairContext, report *profileRepairReport) error {
	fileContents := profileRepairCaseFiles(context.manifest)
	for sourcePath, content := range fileContents {
		targetPath := profileRepairCaseFilePath(context.profilePath, context.manifest.ProfilePath, sourcePath)
		action := "already-present"
		current, err := os.ReadFile(targetPath)
		if err != nil || string(current) != content {
			action = "restore"
			report.Summary.CaseFilesRestored++
			if context.apply {
				if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(targetPath, []byte(ensureTrailingNewline(content)), 0o644); err != nil {
					return err
				}
			}
		} else {
			report.Summary.AlreadyPresent++
		}
		report.Items = append(report.Items, profileRepairItem{Kind: "case-file", Path: targetPath, Action: action})
	}
	return nil
}

func profileRepairWriteCatalog(context profileRepairContext, cases []json.RawMessage, report *profileRepairReport) error {
	if context.apply && report.Summary.CatalogCasesRestored > 0 {
		nextCases := make([]any, 0, len(cases))
		for _, rawCase := range cases {
			var value any
			if err := json.Unmarshal(rawCase, &value); err != nil {
				return err
			}
			nextCases = append(nextCases, value)
		}
		context.catalog["interfaceNodeCases"] = nextCases
		out, err := json.MarshalIndent(context.catalog, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(context.catalogPath, append(out, '\n'), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func profileRepairFinalizeSummary(report *profileRepairReport) {
	if report.Summary.CatalogCasesRestored > 0 && report.Applied {
		report.Summary.ChangedFiles++
	}
	if report.Summary.CaseFilesRestored > 0 && report.Applied {
		report.Summary.ChangedFiles += report.Summary.CaseFilesRestored
	}
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
