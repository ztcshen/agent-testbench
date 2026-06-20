package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/runner/apicase"
)

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

type interfaceNodeCaseApplyResult struct {
	Profile string `json:"profile"`
	File    string `json:"file"`
	Applied int    `json:"applied"`
	Cases   int    `json:"cases"`
	Files   int    `json:"files"`
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

func runInterfaceNodeCaseApply(args []string) error {
	flags := flag.NewFlagSet("interface-node case apply", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Template package path")
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
		return "", fmt.Errorf("case file path %q must stay inside the template package", value)
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
