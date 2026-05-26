package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type catalogCompatibilityAssets struct {
	APICases           []APICase           `json:"-"`
	NodeConfigServices []Service           `json:"-"`
	TemplateConfigs    []TemplateConfig    `json:"templateConfigs"`
	InterfaceNodeCases []json.RawMessage   `json:"interfaceNodeCases"`
	NodeConfigs        []catalogNodeConfig `json:"nodeConfigs"`
}

type catalogNodeConfig struct {
	Service
	Role string `json:"role,omitempty"`
}

func loadCatalogCompatibilityAssets(baseDir string) (catalogCompatibilityAssets, error) {
	payload, err := loadCatalogAsset[catalogCompatibilityAssets](baseDir)
	if err != nil {
		return catalogCompatibilityAssets{}, err
	}
	apiCases, err := decodeCatalogAPICases(baseDir, payload.InterfaceNodeCases)
	if err != nil {
		return catalogCompatibilityAssets{}, err
	}
	payload.APICases = apiCases
	payload.NodeConfigServices = catalogNodeConfigServices(payload.NodeConfigs)
	return payload, nil
}

func decodeCatalogAPICases(baseDir string, rawCases []json.RawMessage) ([]APICase, error) {
	path := filepath.Join(baseDir, catalogName)
	out := make([]APICase, 0, len(rawCases))
	for _, raw := range rawCases {
		apiCase, err := decodeCatalogAPICase(path, raw)
		if err != nil {
			return nil, err
		}
		out = append(out, apiCase)
	}
	return out, nil
}

func decodeCatalogAPICase(path string, raw json.RawMessage) (APICase, error) {
	var apiCase APICase
	if err := json.Unmarshal(raw, &apiCase); err != nil {
		return APICase{}, fmt.Errorf("decode profile catalog api case %s: %w", path, err)
	}
	var titleCarrier struct {
		Title string `json:"title,omitempty"`
	}
	if err := json.Unmarshal(raw, &titleCarrier); err != nil {
		return APICase{}, fmt.Errorf("decode profile catalog api case title %s: %w", path, err)
	}
	if strings.TrimSpace(apiCase.DisplayName) == "" {
		apiCase.DisplayName = titleCarrier.Title
	}
	return apiCase, nil
}

func catalogNodeConfigServices(items []catalogNodeConfig) []Service {
	out := make([]Service, 0, len(items))
	for _, item := range items {
		service := item.Service
		if service.Kind == "" {
			service.Kind = item.Role
		}
		out = append(out, service)
	}
	return out
}

func loadCatalogAsset[T any](baseDir string) (T, error) {
	var out T
	path := filepath.Join(baseDir, catalogName)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("read profile catalog asset %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("decode profile catalog asset %s: %w", path, err)
	}
	return out, nil
}
