package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const manifestName = "profile.json"
const catalogName = "catalog.json"
const agentTestProfilesName = "agent-test-profiles.json"
const configAuthoringName = "config-authoring.json"

func Load(path string) (Bundle, error) {
	manifestPath, err := resolveManifestPath(path)
	if err != nil {
		return Bundle{}, err
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return Bundle{}, fmt.Errorf("read profile manifest %s: %w", manifestPath, err)
	}

	var bundle Bundle
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil {
		return Bundle{}, fmt.Errorf("decode profile manifest %s: %w", manifestPath, err)
	}
	baseDir := filepath.Dir(manifestPath)
	bundle.BaseDir = baseDir
	if err := loadAssets(baseDir, &bundle); err != nil {
		return Bundle{}, err
	}
	if err := validate(bundle); err != nil {
		return Bundle{}, fmt.Errorf("validate profile manifest %s: %w", manifestPath, err)
	}
	return bundle, nil
}

func resolveManifestPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("profile path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat profile path %s: %w", path, err)
	}
	if info.IsDir() {
		return filepath.Join(path, manifestName), nil
	}
	return path, nil
}

func validate(bundle Bundle) error {
	if strings.TrimSpace(bundle.ID) == "" {
		return errors.New("profile id is required")
	}
	if strings.TrimSpace(bundle.DisplayName) == "" {
		return errors.New("profile displayName is required")
	}
	return nil
}

func BundleDigest(path string) (string, error) {
	manifestPath, err := resolveManifestPath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat profile path %s: %w", path, err)
	}
	var files []string
	if !info.IsDir() {
		files = append(files, manifestPath)
	} else {
		baseDir := filepath.Dir(manifestPath)
		if err := filepath.WalkDir(baseDir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			files = append(files, path)
			return nil
		}); err != nil {
			return "", fmt.Errorf("walk profile bundle %s: %w", baseDir, err)
		}
	}
	sort.Strings(files)

	hash := sha256.New()
	for _, file := range files {
		rel := file
		if info.IsDir() {
			if relative, err := filepath.Rel(filepath.Dir(manifestPath), file); err == nil {
				rel = relative
			}
		}
		_, _ = io.WriteString(hash, rel)
		_, _ = hash.Write([]byte{0})
		raw, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read profile bundle file %s: %w", file, err)
		}
		_, _ = hash.Write(raw)
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func loadAssets(baseDir string, bundle *Bundle) error {
	catalogAssets, err := loadCatalogCompatibilityAssets(baseDir)
	if err != nil {
		return err
	}
	if err := loadProfileCatalogAssets(baseDir, bundle, catalogAssets); err != nil {
		return err
	}
	if err := loadProfileExecutionAssets(baseDir, bundle); err != nil {
		return err
	}
	return loadProfileAuthoringAssets(baseDir, bundle, catalogAssets)
}

func loadProfileCatalogAssets(baseDir string, bundle *Bundle, catalog catalogCompatibilityAssets) error {
	services, err := loadAssetDir[Service](baseDir, "services")
	if err != nil {
		return err
	}
	bundle.Services = append(bundle.Services, services...)
	bundle.Services = mergeServices(bundle.Services, catalog.NodeConfigServices)

	workflows, err := loadAssetDir[Workflow](baseDir, "workflows")
	if err != nil {
		return err
	}
	bundle.Workflows = append(bundle.Workflows, workflows...)

	nodes, err := loadAssetDir[InterfaceNode](baseDir, "interface-nodes")
	if err != nil {
		return err
	}
	bundle.InterfaceNodes = append(bundle.InterfaceNodes, nodes...)

	cases, err := loadAPICaseAssets(baseDir)
	if err != nil {
		return err
	}
	bundle.APICases = mergeAPICases(bundle.APICases, catalog.APICases, cases)
	return nil
}

func loadProfileExecutionAssets(baseDir string, bundle *Bundle) error {
	executors, err := loadAssetDir[ExecutorDescriptor](baseDir, "executors")
	if err != nil {
		return err
	}
	bundle.Executors = append(bundle.Executors, executors...)

	requestTemplates, err := loadAssetDir[RequestTemplate](baseDir, "request-templates")
	if err != nil {
		return err
	}
	bundle.RequestTemplates = append(bundle.RequestTemplates, requestTemplates...)

	caseDependencies, err := loadAssetDir[CaseDependency](baseDir, "case-dependencies")
	if err != nil {
		return err
	}
	bundle.CaseDependencies = append(bundle.CaseDependencies, caseDependencies...)

	workflowBindings, err := loadAssetDir[WorkflowBinding](baseDir, "workflow-bindings")
	if err != nil {
		return err
	}
	bundle.WorkflowBindings = append(bundle.WorkflowBindings, workflowBindings...)

	fixtures, err := loadAssetDir[Fixture](baseDir, "fixtures")
	if err != nil {
		return err
	}
	bundle.Fixtures = append(bundle.Fixtures, fixtures...)
	return nil
}

func loadProfileAuthoringAssets(baseDir string, bundle *Bundle, catalog catalogCompatibilityAssets) error {
	bundle.TemplateConfigs = append(bundle.TemplateConfigs, catalog.TemplateConfigs...)

	agentTestProfiles, err := loadAgentTestProfiles(baseDir)
	if err != nil {
		return err
	}
	bundle.AgentTestProfiles = append(bundle.AgentTestProfiles, agentTestProfiles...)

	configAuthoring, err := loadOptionalJSON[ConfigAuthoring](baseDir, configAuthoringName)
	if err != nil {
		return err
	}
	bundle.ConfigAuthoring = configAuthoring
	return nil
}

func loadAgentTestProfiles(baseDir string) ([]AgentTestProfile, error) {
	type fileShape struct {
		SchemaVersion string             `json:"schemaVersion,omitempty"`
		Profiles      []AgentTestProfile `json:"profiles"`
	}
	payload, err := loadOptionalJSON[fileShape](baseDir, agentTestProfilesName)
	if err != nil {
		return nil, err
	}
	return payload.Profiles, nil
}

func loadOptionalJSON[T any](baseDir string, name string) (T, error) {
	var out T
	path := filepath.Join(baseDir, name)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("read profile asset %s: %w", path, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&out); err != nil {
		return out, fmt.Errorf("decode profile asset %s: %w", path, err)
	}
	return out, nil
}

func loadAssetDir[T any](baseDir string, name string) ([]T, error) {
	paths, err := profileAssetJSONPaths(baseDir, name)
	if err != nil {
		return nil, err
	}
	assets := make([]T, 0, len(paths))
	for _, path := range paths {
		asset, err := decodeProfileAsset[T](path)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	return assets, nil
}

func loadAPICaseAssets(baseDir string) ([]APICase, error) {
	paths, err := profileAssetJSONPaths(baseDir, "cases")
	if err != nil {
		return nil, err
	}
	cases := make([]APICase, 0, len(paths))
	for _, path := range paths {
		item, err := decodeProfileAsset[APICase](path)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(item.CasePath) == "" {
			item.CasePath = relativeBundlePath(baseDir, path)
		}
		cases = append(cases, item)
	}
	return cases, nil
}

func profileAssetJSONPaths(baseDir string, name string) ([]string, error) {
	dir := filepath.Join(baseDir, name)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read profile asset directory %s: %w", dir, err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}

func decodeProfileAsset[T any](path string) (T, error) {
	var out T
	raw, err := os.ReadFile(path)
	if err != nil {
		return out, fmt.Errorf("read profile asset %s: %w", path, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&out); err != nil {
		return out, fmt.Errorf("decode profile asset %s: %w", path, err)
	}
	return out, nil
}

func relativeBundlePath(baseDir string, path string) string {
	relative, err := filepath.Rel(baseDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}
