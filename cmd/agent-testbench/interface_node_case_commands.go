package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"agent-testbench/internal/domain/casesuite"
	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/runner/apicase"
)

type interfaceNodeCaseAuditReport struct {
	OK         bool                          `json:"ok"`
	ProfileID  string                        `json:"profileId"`
	NodeID     string                        `json:"nodeId"`
	Counts     interfaceNodeCaseAuditCounts  `json:"counts"`
	Configured []interfaceNodeCaseConfigured `json:"configured"`
	Missing    []interfaceNodeCaseMissing    `json:"missing"`
}

type interfaceNodeCaseAuditCounts struct {
	Cases      int `json:"cases"`
	Configured int `json:"configured"`
	Missing    int `json:"missing"`
}

type interfaceNodeCaseConfigured struct {
	CaseID   string `json:"caseId"`
	ConfigID string `json:"configId"`
}

type interfaceNodeCaseMissing struct {
	CaseID string `json:"caseId"`
	Title  string `json:"title,omitempty"`
}

type interfaceNodeCaseApplyRequest struct {
	APICases           []profile.APICase     `json:"apiCases,omitempty"`
	InterfaceNodeCases []profile.APICase     `json:"interfaceNodeCases,omitempty"`
	TemplateConfigs    []templateConfigInput `json:"templateConfigs,omitempty"`
	CaseFiles          []caseFileInput       `json:"caseFiles,omitempty"`
}

type templateConfigInput struct {
	profile.TemplateConfig
	Config json.RawMessage `json:"config,omitempty"`
}

type caseFileInput struct {
	Path string       `json:"path"`
	Case apicase.Case `json:"case"`
}

type interfaceNodeCaseDraftReport struct {
	OK             bool                          `json:"ok"`
	ProfileID      string                        `json:"profileId"`
	NodeID         string                        `json:"nodeId"`
	CaseID         string                        `json:"caseId"`
	CasePath       string                        `json:"casePath"`
	BundlePath     string                        `json:"bundlePath,omitempty"`
	APICase        profile.APICase               `json:"apiCase"`
	TemplateConfig profile.TemplateConfig        `json:"templateConfig"`
	CaseFile       caseFileInput                 `json:"caseFile"`
	ApplyBundle    interfaceNodeCaseApplyRequest `json:"applyBundle"`
}

type interfaceNodeCaseApplyResult struct {
	Profile string `json:"profile"`
	File    string `json:"file"`
	Applied int    `json:"applied"`
	Cases   int    `json:"cases"`
	Files   int    `json:"files"`
}

func auditInterfaceNodeCaseExecutionConfigs(bundle profile.Bundle, nodeID string) interfaceNodeCaseAuditReport {
	configs := caseExecutionConfigIDs(bundle.TemplateConfigs)
	report := interfaceNodeCaseAuditReport{ProfileID: bundle.ID, NodeID: nodeID}
	for _, item := range bundle.APICases {
		if item.NodeID != nodeID {
			continue
		}
		report.Counts.Cases++
		if configID := configs[item.ID]; configID != "" {
			report.Counts.Configured++
			report.Configured = append(report.Configured, interfaceNodeCaseConfigured{CaseID: item.ID, ConfigID: configID})
			continue
		}
		report.Counts.Missing++
		report.Missing = append(report.Missing, interfaceNodeCaseMissing{CaseID: item.ID, Title: firstNonEmpty(item.DisplayName, item.ID)})
	}
	report.OK = report.Counts.Cases > 0 && report.Counts.Missing == 0
	return report
}

func printInterfaceNodeCaseAudit(report interfaceNodeCaseAuditReport) {
	fmt.Printf("Profile: %s\n", report.ProfileID)
	fmt.Printf("Interface Node: %s\n", report.NodeID)
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Cases: %d\n", report.Counts.Cases)
	fmt.Printf("Configured: %d\n", report.Counts.Configured)
	fmt.Printf("Missing: %d\n", report.Counts.Missing)
	for _, item := range report.Missing {
		fmt.Printf("- missing case execution: %s\n", item.CaseID)
	}
}

func runInterfaceNodeCaseAudit(args []string) error {
	flags := flag.NewFlagSet("interface-node case audit", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path")
	nodeID := flags.String("node", "", "Interface node id")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*profilePath) == "" {
		return errors.New("--profile is required")
	}
	if strings.TrimSpace(*nodeID) == "" {
		return errors.New("--node is required")
	}
	bundle, err := profile.Load(*profilePath)
	if err != nil {
		return err
	}
	report := auditInterfaceNodeCaseExecutionConfigs(bundle, *nodeID)
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printInterfaceNodeCaseAudit(report)
	return nil
}

func runInterfaceNodeCaseDraft(args []string) error {
	flags := flag.NewFlagSet("interface-node case draft", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path")
	nodeID := flags.String("node", "", "Interface node id")
	caseID := flags.String("case-id", "", "Case id to create")
	title := flags.String("title", "", "Case title")
	casePath := flags.String("case-path", "", "Runnable case path inside the profile bundle")
	method := flags.String("method", "", "HTTP method; defaults to the interface node method")
	requestPath := flags.String("path", "", "Request path; defaults to the interface node path")
	priority := flags.String("priority", "", "Case priority metadata")
	owner := flags.String("owner", "", "Case owner metadata")
	outputPath := flags.String("output", "", "Write an apply-ready case config bundle to this path")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var tags stringListFlag
	flags.Var(&tags, "tag", "Case tag metadata; repeat for multiple tags")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*profilePath) == "" {
		return errors.New("--profile is required")
	}
	if strings.TrimSpace(*nodeID) == "" {
		return errors.New("--node is required")
	}
	if strings.TrimSpace(*caseID) == "" {
		return errors.New("--case-id is required")
	}
	bundle, err := profile.Load(*profilePath)
	if err != nil {
		return err
	}
	report, err := draftInterfaceNodeCase(bundle, *nodeID, *caseID, *title, *casePath, *method, *requestPath, tags.Values(), *priority, *owner)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*outputPath) != "" {
		if err := writeCaseApplyBundle(*outputPath, report.ApplyBundle); err != nil {
			return err
		}
		report.BundlePath = *outputPath
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	fmt.Printf("Case Draft: %s\n", report.CaseID)
	fmt.Printf("Node: %s\n", report.NodeID)
	fmt.Printf("Case Path: %s\n", report.CasePath)
	if report.BundlePath != "" {
		fmt.Printf("Bundle: %s\n", report.BundlePath)
	}
	return nil
}

func draftInterfaceNodeCase(bundle profile.Bundle, nodeID string, caseID string, title string, casePath string, method string, requestPath string, tags []string, priority string, owner string) (interfaceNodeCaseDraftReport, error) {
	node, ok := findInterfaceNode(bundle.InterfaceNodes, nodeID)
	if !ok {
		return interfaceNodeCaseDraftReport{}, fmt.Errorf("interface node %q not found", nodeID)
	}
	caseID = strings.TrimSpace(caseID)
	if caseExists(bundle.APICases, caseID) {
		return interfaceNodeCaseDraftReport{}, fmt.Errorf("api case %q already exists", caseID)
	}
	method = strings.ToUpper(strings.TrimSpace(firstNonEmpty(method, node.Method, "GET")))
	requestPath = strings.TrimSpace(firstNonEmpty(requestPath, node.Path, "/"))
	if !strings.HasPrefix(requestPath, "/") {
		requestPath = "/" + requestPath
	}
	title = strings.TrimSpace(firstNonEmpty(title, node.DisplayName, caseID))
	if strings.TrimSpace(casePath) == "" {
		casePath = filepath.ToSlash(filepath.Join("api-cases", safeCaseFileName(caseID)+".json"))
	}
	apiCase := profile.APICase{
		ID:          caseID,
		DisplayName: title,
		Description: "Generated draft for " + firstNonEmpty(node.DisplayName, node.ID) + ".",
		NodeID:      node.ID,
		Tags:        casesuite.NormalizeStringList(tags),
		Priority:    strings.TrimSpace(priority),
		Owner:       strings.TrimSpace(owner),
		Status:      "active",
		SortOrder:   nextCaseSortOrder(bundle.APICases),
		CasePath:    filepath.ToSlash(casePath),
	}
	caseFile := caseFileInput{
		Path: apiCase.CasePath,
		Case: apicase.Case{
			ID:    caseID,
			Title: title,
			Request: apicase.Request{
				Method:  method,
				Path:    requestPath,
				Headers: draftCaseHeaders(method),
				Body:    draftCaseBody(method),
			},
			Assertions: apicase.Assertions{ExpectedStatusCodes: []int{http.StatusOK}},
		},
	}
	configJSON, err := compactJSONValue(map[string]any{
		"caseId": caseID,
		"caseExecution": map[string]any{
			"method":            method,
			"nodeId":            node.ID,
			"path":              requestPath,
			"expectedHttpCodes": []int{http.StatusOK},
		},
	})
	if err != nil {
		return interfaceNodeCaseDraftReport{}, err
	}
	config := profile.TemplateConfig{
		ID:          "cfg." + caseID,
		TemplateID:  "case-execution",
		NodeID:      node.ID,
		ScopeType:   "case",
		ScopeID:     caseID,
		Title:       title + " execution",
		Description: "Generated draft execution config.",
		ConfigJSON:  configJSON,
		Status:      "active",
		SortOrder:   apiCase.SortOrder,
	}
	applyBundle := interfaceNodeCaseApplyRequest{
		APICases:        []profile.APICase{apiCase},
		TemplateConfigs: []templateConfigInput{{TemplateConfig: config}},
		CaseFiles:       []caseFileInput{caseFile},
	}
	return interfaceNodeCaseDraftReport{
		OK:             true,
		ProfileID:      bundle.ID,
		NodeID:         node.ID,
		CaseID:         caseID,
		CasePath:       apiCase.CasePath,
		APICase:        apiCase,
		TemplateConfig: config,
		CaseFile:       caseFile,
		ApplyBundle:    applyBundle,
	}, nil
}

func writeCaseApplyBundle(path string, bundle interfaceNodeCaseApplyRequest) error {
	raw, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create case draft output directory: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write case draft bundle %s: %w", path, err)
	}
	return nil
}

func findInterfaceNode(nodes []profile.InterfaceNode, id string) (profile.InterfaceNode, bool) {
	id = strings.TrimSpace(id)
	for _, node := range nodes {
		if node.ID == id {
			return node, true
		}
	}
	return profile.InterfaceNode{}, false
}

func caseExists(cases []profile.APICase, id string) bool {
	for _, item := range cases {
		if item.ID == id {
			return true
		}
	}
	return false
}

func nextCaseSortOrder(cases []profile.APICase) int {
	maxOrder := 0
	for _, item := range cases {
		if item.SortOrder > maxOrder {
			maxOrder = item.SortOrder
		}
	}
	return maxOrder + 1
}

func safeCaseFileName(caseID string) string {
	return safeProfileAssetFileName(caseID, "case")
}

func safeProfileAssetFileName(value string, defaultName string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultName
	}
	var builder strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('-')
	}
	if builder.Len() == 0 {
		return defaultName
	}
	return builder.String()
}

func draftCaseHeaders(method string) map[string]string {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodGet, http.MethodHead:
		return nil
	default:
		return map[string]string{"Content-Type": "application/json"}
	}
}

func draftCaseBody(method string) map[string]any {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodGet, http.MethodHead:
		return nil
	default:
		return map[string]any{"sample": true}
	}
}

func runInterfaceNodeCaseApply(args []string) error {
	flags := flag.NewFlagSet("interface-node case apply", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path")
	requestPath := flags.String("file", "", "Case execution config bundle")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*profilePath) == "" {
		return errors.New("--profile is required")
	}
	if strings.TrimSpace(*requestPath) == "" {
		return errors.New("--file is required")
	}
	result, err := applyInterfaceNodeCaseConfigs(*profilePath, *requestPath)
	if err != nil {
		return err
	}
	result.Profile = *profilePath
	result.File = *requestPath
	if *jsonOutput {
		return writeIndentedJSON(result)
	}
	fmt.Printf("Applied interface node case configs: %d\n", result.Applied)
	if result.Cases > 0 {
		fmt.Printf("Applied API cases: %d\n", result.Cases)
	}
	if result.Files > 0 {
		fmt.Printf("Applied case files: %d\n", result.Files)
	}
	fmt.Printf("Profile: %s\n", *profilePath)
	return nil
}

func applyInterfaceNodeCaseConfigs(profilePath string, requestPath string) (interfaceNodeCaseApplyResult, error) {
	raw, err := os.ReadFile(requestPath)
	if err != nil {
		return interfaceNodeCaseApplyResult{}, fmt.Errorf("read case config bundle %s: %w", requestPath, err)
	}
	var request interfaceNodeCaseApplyRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return interfaceNodeCaseApplyResult{}, fmt.Errorf("decode case config bundle %s: %w", requestPath, err)
	}
	request.APICases = append(request.APICases, request.InterfaceNodeCases...)
	if len(request.TemplateConfigs) == 0 && len(request.APICases) == 0 && len(request.CaseFiles) == 0 {
		return interfaceNodeCaseApplyResult{}, errors.New("case config bundle must include apiCases, templateConfigs, or caseFiles")
	}
	configs := make([]profile.TemplateConfig, 0, len(request.TemplateConfigs))
	for _, item := range request.TemplateConfigs {
		config, err := normalizeTemplateConfigInput(item)
		if err != nil {
			return interfaceNodeCaseApplyResult{}, err
		}
		configs = append(configs, config)
	}
	apiCases := make([]profile.APICase, 0, len(request.APICases))
	for _, item := range request.APICases {
		apiCase, err := normalizeAPICaseInput(item)
		if err != nil {
			return interfaceNodeCaseApplyResult{}, err
		}
		apiCases = append(apiCases, apiCase)
	}
	if err := writeCaseFiles(profilePath, request.CaseFiles); err != nil {
		return interfaceNodeCaseApplyResult{}, err
	}
	catalogPath := filepath.Join(profilePath, "catalog.json")
	payload, existingConfigs, existingCases, err := readCatalogCaseAssets(catalogPath)
	if err != nil {
		return interfaceNodeCaseApplyResult{}, err
	}
	if len(configs) > 0 {
		merged := mergeTemplateConfigs(existingConfigs, configs)
		configRaw, err := json.Marshal(merged)
		if err != nil {
			return interfaceNodeCaseApplyResult{}, err
		}
		payload["templateConfigs"] = configRaw
	}
	if len(apiCases) > 0 {
		merged := mergeProfileAPICases(existingCases, apiCases)
		casesRaw, err := json.Marshal(merged)
		if err != nil {
			return interfaceNodeCaseApplyResult{}, err
		}
		payload["interfaceNodeCases"] = casesRaw
		delete(payload, "apiCases")
	}
	if _, ok := payload["schemaVersion"]; !ok {
		payload["schemaVersion"] = json.RawMessage(`"1"`)
	}
	next, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return interfaceNodeCaseApplyResult{}, err
	}
	next = append(next, '\n')
	if err := os.WriteFile(catalogPath, next, 0o644); err != nil {
		return interfaceNodeCaseApplyResult{}, fmt.Errorf("write profile catalog %s: %w", catalogPath, err)
	}
	if _, err := profile.Load(profilePath); err != nil {
		return interfaceNodeCaseApplyResult{}, fmt.Errorf("profile catalog is invalid after apply: %w", err)
	}
	return interfaceNodeCaseApplyResult{Applied: len(configs), Cases: len(apiCases), Files: len(request.CaseFiles)}, nil
}

func normalizeTemplateConfigInput(input templateConfigInput) (profile.TemplateConfig, error) {
	config := input.TemplateConfig
	if len(input.Config) > 0 {
		compact, err := compactRawJSON(input.Config)
		if err != nil {
			return profile.TemplateConfig{}, fmt.Errorf("template config %q config is invalid: %w", config.ID, err)
		}
		config.ConfigJSON = compact
	}
	if strings.TrimSpace(config.ID) == "" {
		return profile.TemplateConfig{}, errors.New("template config id is required")
	}
	if strings.TrimSpace(config.ConfigJSON) == "" {
		return profile.TemplateConfig{}, fmt.Errorf("template config %q configJson is required", config.ID)
	}
	if caseID, ok := caseExecutionConfigCaseID(config.ConfigJSON); !ok {
		return profile.TemplateConfig{}, fmt.Errorf("template config %q must contain caseId and caseExecution", config.ID)
	} else if strings.TrimSpace(config.ScopeID) == "" {
		config.ScopeID = caseID
	}
	if strings.TrimSpace(config.ScopeType) == "" {
		config.ScopeType = "case"
	}
	if strings.TrimSpace(config.Status) == "" {
		config.Status = "active"
	}
	return config, nil
}

func normalizeAPICaseInput(item profile.APICase) (profile.APICase, error) {
	item.ID = strings.TrimSpace(item.ID)
	item.NodeID = strings.TrimSpace(item.NodeID)
	item.CasePath = filepath.ToSlash(strings.TrimSpace(item.CasePath))
	if item.ID == "" {
		return profile.APICase{}, errors.New("api case id is required")
	}
	if item.NodeID == "" {
		return profile.APICase{}, fmt.Errorf("api case %q nodeId is required", item.ID)
	}
	if item.Status == "" {
		item.Status = "active"
	}
	if item.DisplayName == "" {
		item.DisplayName = item.ID
	}
	return item, nil
}

func writeCaseFiles(profilePath string, files []caseFileInput) error {
	for _, item := range files {
		relative, err := safeBundleRelativePath(item.Path)
		if err != nil {
			return err
		}
		if strings.TrimSpace(item.Case.ID) == "" {
			return fmt.Errorf("case file %q case id is required", item.Path)
		}
		target := filepath.Join(profilePath, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create case file directory %s: %w", filepath.Dir(target), err)
		}
		raw, err := json.MarshalIndent(item.Case, "", "  ")
		if err != nil {
			return fmt.Errorf("encode case file %s: %w", item.Path, err)
		}
		raw = append(raw, '\n')
		if err := os.WriteFile(target, raw, 0o644); err != nil {
			return fmt.Errorf("write case file %s: %w", target, err)
		}
	}
	return nil
}

func safeBundleRelativePath(value string) (string, error) {
	value = filepath.ToSlash(strings.TrimSpace(value))
	if value == "" {
		return "", errors.New("case file path is required")
	}
	if filepath.IsAbs(value) || strings.HasPrefix(value, "../") || strings.Contains(value, "/../") || value == ".." {
		return "", fmt.Errorf("case file path %q must stay inside the profile bundle", value)
	}
	return filepath.FromSlash(value), nil
}

func readCatalogCaseAssets(path string) (map[string]json.RawMessage, []profile.TemplateConfig, []profile.APICase, error) {
	payload := map[string]json.RawMessage{}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return payload, nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read profile catalog %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, nil, fmt.Errorf("decode profile catalog %s: %w", path, err)
	}
	var configs []profile.TemplateConfig
	if rawConfigs, ok := payload["templateConfigs"]; ok {
		if err := json.Unmarshal(rawConfigs, &configs); err != nil {
			return nil, nil, nil, fmt.Errorf("decode profile catalog templateConfigs %s: %w", path, err)
		}
	}
	var cases []profile.APICase
	for _, key := range []string{"interfaceNodeCases", "apiCases"} {
		rawCases, ok := payload[key]
		if !ok {
			continue
		}
		if err := json.Unmarshal(rawCases, &cases); err != nil {
			return nil, nil, nil, fmt.Errorf("decode profile catalog %s %s: %w", key, path, err)
		}
		break
	}
	return payload, configs, cases, nil
}

func mergeTemplateConfigs(existing []profile.TemplateConfig, updates []profile.TemplateConfig) []profile.TemplateConfig {
	return mergeProfileCatalogItems(existing, updates, func(item profile.TemplateConfig) string {
		return item.ID
	}, func(item profile.TemplateConfig) int {
		return item.SortOrder
	})
}

func mergeProfileAPICases(existing []profile.APICase, updates []profile.APICase) []profile.APICase {
	return mergeProfileCatalogItems(existing, updates, func(item profile.APICase) string {
		return item.ID
	}, func(item profile.APICase) int {
		return item.SortOrder
	})
}

func mergeProfileCatalogItems[T any](existing []T, updates []T, itemID func(T) string, itemSortOrder func(T) int) []T {
	positions := map[string]int{}
	out := make([]T, 0, len(existing)+len(updates))
	for _, item := range existing {
		id := itemID(item)
		positions[id] = len(out)
		out = append(out, item)
	}
	for _, item := range updates {
		id := itemID(item)
		if index, ok := positions[id]; ok {
			out[index] = item
			continue
		}
		positions[id] = len(out)
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		leftOrder, rightOrder := itemSortOrder(out[i]), itemSortOrder(out[j])
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return itemID(out[i]) < itemID(out[j])
	})
	return out
}
