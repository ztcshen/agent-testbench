package store

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Environment struct {
	ID                     string
	DisplayName            string
	Description            string
	Status                 string
	Verified               bool
	ServicesJSON           string
	ReposJSON              string
	ComposeJSON            string
	HealthChecksJSON       string
	VerificationWorkflowID string
	LastVerificationRunID  string
	LastVerificationStatus string
	EvidenceComplete       bool
	TopologyComplete       bool
	LastVerifiedAt         time.Time
	SummaryJSON            string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

const (
	StoreMetadataMaxBytes         = 1024 * 1024
	EnvironmentDefinitionMaxBytes = StoreMetadataMaxBytes
	EnvironmentSummaryMaxBytes    = StoreMetadataMaxBytes
	ComponentAssetInlineMaxBytes  = StoreMetadataMaxBytes
	ComponentGraphMaxBytes        = StoreMetadataMaxBytes
	EnvironmentFileInlineMaxBytes = StoreMetadataMaxBytes
	EnvironmentFilesMaxBytes      = StoreMetadataMaxBytes
	EnvironmentServicesMaxBytes   = StoreMetadataMaxBytes
	EnvironmentHealthMaxBytes     = StoreMetadataMaxBytes
)

const (
	EnvironmentFileKindComposeFile    = "compose-file"
	EnvironmentFileKindComposeEnvFile = "compose-env-file"
	EnvironmentFileKindStartupFile    = "startup-file"
)

func PrepareEnvironmentForUpsert(e Environment, now time.Time) (Environment, error) {
	if err := ValidateEnvironmentDefinitionSize(e); err != nil {
		return Environment{}, err
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = now
	}
	return e, nil
}

func PrepareEnvironmentForStructuredUpsert(ctx context.Context, lister EnvironmentStructuredStateLister, e Environment, now time.Time) (Environment, error) {
	e, err := EnvironmentWithoutStructuredState(ctx, lister, e)
	if err != nil {
		return Environment{}, err
	}
	return PrepareEnvironmentForUpsert(e, now)
}

func ValidateEnvironmentDefinitionSize(e Environment) error {
	definitionFields := []namedSize{
		{name: "id", size: len(e.ID)},
		{name: "display_name", size: len(e.DisplayName)},
		{name: "description", size: len(e.Description)},
		{name: "status", size: len(e.Status)},
		{name: "services_json", size: len(e.ServicesJSON)},
		{name: "repos_json", size: len(e.ReposJSON)},
		{name: "compose_json", size: len(e.ComposeJSON)},
		{name: "health_checks_json", size: len(e.HealthChecksJSON)},
		{name: "verification_workflow_id", size: len(e.VerificationWorkflowID)},
	}
	total := 0
	for _, field := range definitionFields {
		total += field.size
	}
	if total > EnvironmentDefinitionMaxBytes {
		largest := largestNamedSize(definitionFields)
		return fmt.Errorf("environment definition metadata is %d bytes; 1 MB safety boundary is %d bytes; write blocked. Reason: largest contributor %s contributes %d bytes in environment %q; below this boundary the Store accepts deterministic restore metadata, startup configuration, DDL, seed SQL, certificates, keys, and launch scripts without per-kind limits", total, EnvironmentDefinitionMaxBytes, largest.name, largest.size, e.ID)
	}
	if len(e.SummaryJSON) > EnvironmentSummaryMaxBytes {
		return fmt.Errorf("environment summary metadata is %d bytes; 1 MB safety boundary is %d bytes; write blocked. Reason: largest contributor summary_json contributes %d bytes in environment %q; below this boundary the Store accepts acceptance summaries, restore status, and indexes without per-kind limits", len(e.SummaryJSON), EnvironmentSummaryMaxBytes, len(e.SummaryJSON), e.ID)
	}
	return nil
}

type namedSize struct {
	name string
	size int
}

func largestNamedSize(items []namedSize) namedSize {
	largest := namedSize{}
	for _, item := range items {
		if item.size > largest.size {
			largest = item
		}
	}
	return largest
}

type EnvironmentComponentGraph struct {
	Components   []EnvironmentComponent `json:"components"`
	Dependencies []ComponentDependency  `json:"dependencies"`
	Assets       []ComponentConfigAsset `json:"assets"`
}

type EnvironmentFile struct {
	EnvID         string    `json:"envId,omitempty"`
	Path          string    `json:"path"`
	Kind          string    `json:"kind"`
	ContentInline string    `json:"contentInline,omitempty"`
	Required      bool      `json:"required"`
	ApplyOrder    int       `json:"applyOrder,omitempty"`
	SummaryJSON   string    `json:"summaryJson,omitempty"`
	CreatedAt     time.Time `json:"createdAt,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt,omitempty"`
}

type EnvironmentService struct {
	EnvID       string    `json:"envId,omitempty"`
	ServiceID   string    `json:"serviceId"`
	RepoURL     string    `json:"repoUrl,omitempty"`
	Branch      string    `json:"branch,omitempty"`
	Ref         string    `json:"ref,omitempty"`
	Checkout    string    `json:"checkout,omitempty"`
	SummaryJSON string    `json:"summaryJson,omitempty"`
	CreatedAt   time.Time `json:"createdAt,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty"`
}

type EnvironmentHealthCheck struct {
	EnvID          string    `json:"envId,omitempty"`
	CheckID        string    `json:"checkId"`
	Kind           string    `json:"kind"`
	URL            string    `json:"url,omitempty"`
	Address        string    `json:"address,omitempty"`
	Command        string    `json:"command,omitempty"`
	ComposeService string    `json:"composeService,omitempty"`
	Expect         string    `json:"expect,omitempty"`
	ApplyOrder     int       `json:"applyOrder,omitempty"`
	SummaryJSON    string    `json:"summaryJson,omitempty"`
	CreatedAt      time.Time `json:"createdAt,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt,omitempty"`
}

type EnvironmentComponent struct {
	EnvID           string    `json:"envId,omitempty"`
	ComponentID     string    `json:"componentId"`
	DisplayName     string    `json:"displayName,omitempty"`
	Kind            string    `json:"kind,omitempty"`
	Role            string    `json:"role,omitempty"`
	ComposeService  string    `json:"composeService,omitempty"`
	Image           string    `json:"image,omitempty"`
	Required        bool      `json:"required"`
	RuntimeJSON     string    `json:"runtimeJson,omitempty"`
	HealthCheckJSON string    `json:"healthCheckJson,omitempty"`
	SummaryJSON     string    `json:"summaryJson,omitempty"`
	CreatedAt       time.Time `json:"createdAt,omitempty"`
	UpdatedAt       time.Time `json:"updatedAt,omitempty"`
}

type ComponentDependency struct {
	EnvID               string    `json:"envId,omitempty"`
	ConsumerComponentID string    `json:"consumerComponentId"`
	ProviderComponentID string    `json:"providerComponentId"`
	Phase               string    `json:"phase,omitempty"`
	Capability          string    `json:"capability,omitempty"`
	Required            bool      `json:"required"`
	ProfileJSON         string    `json:"profileJson,omitempty"`
	CreatedAt           time.Time `json:"createdAt,omitempty"`
	UpdatedAt           time.Time `json:"updatedAt,omitempty"`
}

type ComponentConfigAsset struct {
	EnvID             string    `json:"envId,omitempty"`
	OwnerComponentID  string    `json:"ownerComponentId"`
	AssetID           string    `json:"assetId"`
	AssetKind         string    `json:"assetKind,omitempty"`
	TargetComponentID string    `json:"targetComponentId,omitempty"`
	TargetPath        string    `json:"targetPath,omitempty"`
	ContentInline     string    `json:"contentInline,omitempty"`
	RemoteRefJSON     string    `json:"remoteRefJson,omitempty"`
	SHA256            string    `json:"sha256,omitempty"`
	SizeBytes         int64     `json:"sizeBytes,omitempty"`
	ApplyOrder        int       `json:"applyOrder,omitempty"`
	Sensitive         bool      `json:"sensitive"`
	SummaryJSON       string    `json:"summaryJson,omitempty"`
	CreatedAt         time.Time `json:"createdAt,omitempty"`
	UpdatedAt         time.Time `json:"updatedAt,omitempty"`
}

func ValidateEnvironmentComponentGraph(envID string, g EnvironmentComponentGraph) error {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return fmt.Errorf("environment id is required for component graph")
	}
	total := 0
	componentIDs := map[string]bool{}
	contributors := make([]namedSize, 0, len(g.Components)+len(g.Dependencies)+len(g.Assets))
	for _, component := range g.Components {
		id := strings.TrimSpace(component.ComponentID)
		if id == "" {
			return fmt.Errorf("component id is required")
		}
		componentIDs[id] = true
		size := len(id) + len(component.DisplayName) + len(component.Kind) + len(component.Role) +
			len(component.ComposeService) + len(component.Image) + len(component.RuntimeJSON) +
			len(component.HealthCheckJSON) + len(component.SummaryJSON)
		total += size
		contributors = append(contributors, namedSize{name: fmt.Sprintf("component %q", id), size: size})
	}
	for _, dep := range g.Dependencies {
		consumer := strings.TrimSpace(dep.ConsumerComponentID)
		provider := strings.TrimSpace(dep.ProviderComponentID)
		if consumer == "" || provider == "" {
			return fmt.Errorf("component dependency requires consumer and provider component ids")
		}
		if !componentIDs[consumer] {
			return fmt.Errorf("component dependency consumer %q is not registered in environment %s", consumer, envID)
		}
		if !componentIDs[provider] {
			return fmt.Errorf("component dependency provider %q is not registered in environment %s", provider, envID)
		}
		size := len(consumer) + len(provider) + len(dep.Phase) + len(dep.Capability) + len(dep.ProfileJSON)
		total += size
		contributors = append(contributors, namedSize{name: fmt.Sprintf("dependency %q->%q", consumer, provider), size: size})
	}
	for _, asset := range g.Assets {
		owner := strings.TrimSpace(asset.OwnerComponentID)
		if owner == "" || strings.TrimSpace(asset.AssetID) == "" {
			return fmt.Errorf("component config asset requires owner component id and asset id")
		}
		if !componentIDs[owner] {
			return fmt.Errorf("component config asset owner %q is not registered in environment %s", owner, envID)
		}
		target := strings.TrimSpace(asset.TargetComponentID)
		if target != "" && !componentIDs[target] {
			return fmt.Errorf("component config asset target %q is not registered in environment %s", target, envID)
		}
		if len(asset.ContentInline) > ComponentAssetInlineMaxBytes {
			return fmt.Errorf("component config asset %q inline content is %d bytes; 1 MB safety boundary is %d bytes; write blocked. Reason: owner=%q kind=%q target=%q path=%q is the single inline content contributor over the boundary; below this boundary the Store accepts deterministic text configuration, DDL, seed SQL, certificates, keys, and launch scripts without per-kind limits", asset.AssetID, len(asset.ContentInline), ComponentAssetInlineMaxBytes, owner, asset.AssetKind, target, asset.TargetPath)
		}
		size := len(owner) + len(asset.AssetID) + len(asset.AssetKind) + len(target) + len(asset.TargetPath) +
			len(asset.ContentInline) + len(asset.RemoteRefJSON) + len(asset.SHA256) + len(asset.SummaryJSON)
		total += size
		contributors = append(contributors, namedSize{name: fmt.Sprintf("asset %q owner=%q kind=%q target=%q path=%q", asset.AssetID, owner, asset.AssetKind, target, asset.TargetPath), size: size})
	}
	if total > ComponentGraphMaxBytes {
		largest := largestNamedSize(contributors)
		return fmt.Errorf("environment component graph metadata is %d bytes; 1 MB safety boundary is %d bytes; write blocked. Reason: largest contributor %s contributes %d bytes in environment %q, and the combined component graph is over the boundary; below this boundary the Store accepts deterministic restore metadata and startup text without per-kind limits", total, ComponentGraphMaxBytes, largest.name, largest.size, envID)
	}
	return nil
}

func ValidateEnvironmentFiles(envID string, files []EnvironmentFile) error {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return fmt.Errorf("environment id is required for environment files")
	}
	seen := map[string]bool{}
	total := 0
	contributors := make([]namedSize, 0, len(files))
	for _, file := range files {
		path := cleanEnvironmentFilePath(file.Path)
		kind := strings.TrimSpace(file.Kind)
		if path == "" {
			return fmt.Errorf("environment file path is required")
		}
		if kind == "" {
			return fmt.Errorf("environment file kind is required for %s", path)
		}
		if !validEnvironmentFileKind(kind) {
			return fmt.Errorf("environment file %s has unsupported kind %q", path, kind)
		}
		if len(file.ContentInline) > EnvironmentFileInlineMaxBytes {
			return fmt.Errorf("environment file %q inline content is %d bytes; 1 MB safety boundary is %d bytes; write blocked", path, len(file.ContentInline), EnvironmentFileInlineMaxBytes)
		}
		key := kind + "\x00" + path
		if seen[key] {
			return fmt.Errorf("duplicate environment file %s kind=%s", path, kind)
		}
		seen[key] = true
		size := len(path) + len(kind) + len(file.ContentInline) + len(file.SummaryJSON)
		total += size
		contributors = append(contributors, namedSize{name: fmt.Sprintf("environment file %q kind=%q", path, kind), size: size})
	}
	if total > EnvironmentFilesMaxBytes {
		largest := largestNamedSize(contributors)
		return fmt.Errorf("environment file metadata is %d bytes; 1 MB safety boundary is %d bytes; write blocked. Reason: largest contributor %s contributes %d bytes in environment %q", total, EnvironmentFilesMaxBytes, largest.name, largest.size, envID)
	}
	return nil
}

func NormalizeEnvironmentFiles(files []EnvironmentFile) []EnvironmentFile {
	out := make([]EnvironmentFile, 0, len(files))
	for _, file := range files {
		file.Path = cleanEnvironmentFilePath(file.Path)
		file.Kind = strings.TrimSpace(file.Kind)
		if strings.TrimSpace(file.SummaryJSON) == "" {
			file.SummaryJSON = "{}"
		}
		out = append(out, file)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ApplyOrder != out[j].ApplyOrder {
			return out[i].ApplyOrder < out[j].ApplyOrder
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func MergeEnvironmentFilesIntoComposeJSON(env Environment, files []EnvironmentFile) (Environment, error) {
	files = NormalizeEnvironmentFiles(files)
	if len(files) == 0 {
		return env, nil
	}
	compose := map[string]any{}
	if strings.TrimSpace(env.ComposeJSON) != "" {
		if err := json.Unmarshal([]byte(env.ComposeJSON), &compose); err != nil {
			return Environment{}, fmt.Errorf("decode environment compose_json: %w", err)
		}
	}
	generated := stringMapFromJSONAny(compose["generatedFiles"])
	composeFiles := make([]string, 0)
	envFiles := make([]string, 0)
	for _, file := range files {
		switch file.Kind {
		case EnvironmentFileKindComposeFile:
			composeFiles = append(composeFiles, file.Path)
		case EnvironmentFileKindComposeEnvFile:
			envFiles = append(envFiles, file.Path)
		}
		if EnvironmentFileHasInlineContent(file) {
			generated[file.Path] = file.ContentInline
		}
	}
	if len(composeFiles) > 0 {
		compose["composeFile"] = composeFiles[0]
		compose["composeFiles"] = composeFiles
	}
	if len(envFiles) > 0 {
		compose["envFiles"] = envFiles
	}
	if len(generated) > 0 {
		compose["generatedFiles"] = generated
	}
	raw, err := json.Marshal(compose)
	if err != nil {
		return Environment{}, fmt.Errorf("encode structured environment compose files: %w", err)
	}
	env.ComposeJSON = string(raw)
	return env, nil
}

func EnvironmentWithoutStructuredState(ctx context.Context, lister EnvironmentStructuredStateLister, env Environment) (Environment, error) {
	files, err := lister.ListEnvironmentFiles(ctx, env.ID)
	if err != nil {
		return Environment{}, err
	}
	services, err := lister.ListEnvironmentServices(ctx, env.ID)
	if err != nil {
		return Environment{}, err
	}
	checks, err := lister.ListEnvironmentHealthChecks(ctx, env.ID)
	if err != nil {
		return Environment{}, err
	}
	env = EnvironmentWithoutStructuredFiles(env, files)
	return EnvironmentWithoutStructuredRuntimeMetadata(env, services, checks), nil
}

func EnvironmentWithoutStructuredFiles(env Environment, files []EnvironmentFile) Environment {
	files = NormalizeEnvironmentFiles(files)
	if len(files) == 0 {
		return env
	}
	compose := map[string]any{}
	if strings.TrimSpace(env.ComposeJSON) != "" {
		if err := json.Unmarshal([]byte(env.ComposeJSON), &compose); err != nil {
			return env
		}
	}
	generated := stringMapFromJSONAny(compose["generatedFiles"])
	structuredComposeFiles := map[string]bool{}
	structuredEnvFiles := map[string]bool{}
	for _, file := range files {
		path := cleanEnvironmentFilePath(file.Path)
		if path == "" {
			continue
		}
		switch file.Kind {
		case EnvironmentFileKindComposeFile:
			structuredComposeFiles[path] = true
		case EnvironmentFileKindComposeEnvFile:
			structuredEnvFiles[path] = true
		}
		if !EnvironmentFileHasInlineContent(file) {
			continue
		}
		delete(generated, path)
		delete(generated, file.Path)
	}
	composeFiles := stringSliceWithoutPaths(stringSliceFromJSONAny(compose["composeFiles"]), structuredComposeFiles)
	envFiles := stringSliceWithoutPaths(stringSliceFromJSONAny(compose["envFiles"]), structuredEnvFiles)
	if len(composeFiles) > 0 {
		compose["composeFiles"] = composeFiles
	} else {
		delete(compose, "composeFiles")
	}
	composeFile := cleanEnvironmentFilePath(valueStringFromAny(compose["composeFile"]))
	if composeFile != "" && structuredComposeFiles[composeFile] {
		if len(composeFiles) > 0 {
			compose["composeFile"] = composeFiles[0]
		} else {
			delete(compose, "composeFile")
		}
	}
	if len(envFiles) > 0 {
		compose["envFiles"] = envFiles
	} else {
		delete(compose, "envFiles")
	}
	if len(generated) > 0 {
		compose["generatedFiles"] = generated
	} else {
		delete(compose, "generatedFiles")
	}
	env.ComposeJSON = mustJSON(compose, "{}")
	return env
}

func stringSliceWithoutPaths(values []string, paths map[string]bool) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		clean := cleanEnvironmentFilePath(value)
		if clean == "" || paths[clean] {
			continue
		}
		out = append(out, clean)
	}
	return out
}

func ValidateEnvironmentServices(envID string, services []EnvironmentService) error {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return fmt.Errorf("environment id is required for environment services")
	}
	seen := map[string]bool{}
	total := 0
	contributors := make([]namedSize, 0, len(services))
	for _, service := range services {
		id := strings.TrimSpace(service.ServiceID)
		if id == "" {
			return fmt.Errorf("environment service id is required")
		}
		if seen[id] {
			return fmt.Errorf("duplicate environment service %s", id)
		}
		seen[id] = true
		size := len(id) + len(service.RepoURL) + len(service.Branch) + len(service.Ref) + len(service.Checkout) + len(service.SummaryJSON)
		total += size
		contributors = append(contributors, namedSize{name: fmt.Sprintf("environment service %q", id), size: size})
	}
	if total > EnvironmentServicesMaxBytes {
		largest := largestNamedSize(contributors)
		return fmt.Errorf("environment service metadata is %d bytes; 1 MB safety boundary is %d bytes; write blocked. Reason: largest contributor %s contributes %d bytes in environment %q", total, EnvironmentServicesMaxBytes, largest.name, largest.size, envID)
	}
	return nil
}

func NormalizeEnvironmentServices(services []EnvironmentService) []EnvironmentService {
	out := make([]EnvironmentService, 0, len(services))
	for _, service := range services {
		service.ServiceID = strings.TrimSpace(service.ServiceID)
		service.RepoURL = strings.TrimSpace(service.RepoURL)
		service.Branch = strings.TrimSpace(service.Branch)
		service.Ref = strings.TrimSpace(service.Ref)
		service.Checkout = strings.TrimSpace(service.Checkout)
		if strings.TrimSpace(service.SummaryJSON) == "" {
			service.SummaryJSON = "{}"
		}
		out = append(out, service)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ServiceID < out[j].ServiceID
	})
	return out
}

func ValidateEnvironmentHealthChecks(envID string, checks []EnvironmentHealthCheck) error {
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return fmt.Errorf("environment id is required for environment health checks")
	}
	seen := map[string]bool{}
	total := 0
	contributors := make([]namedSize, 0, len(checks))
	for _, check := range checks {
		id := strings.TrimSpace(check.CheckID)
		kind := strings.TrimSpace(check.Kind)
		if id == "" || kind == "" {
			return fmt.Errorf("environment health check requires id and kind")
		}
		if seen[id] {
			return fmt.Errorf("duplicate environment health check %s", id)
		}
		seen[id] = true
		size := len(id) + len(kind) + len(check.URL) + len(check.Address) + len(check.Command) + len(check.ComposeService) + len(check.Expect) + len(check.SummaryJSON)
		total += size
		contributors = append(contributors, namedSize{name: fmt.Sprintf("environment health check %q", id), size: size})
	}
	if total > EnvironmentHealthMaxBytes {
		largest := largestNamedSize(contributors)
		return fmt.Errorf("environment health check metadata is %d bytes; 1 MB safety boundary is %d bytes; write blocked. Reason: largest contributor %s contributes %d bytes in environment %q", total, EnvironmentHealthMaxBytes, largest.name, largest.size, envID)
	}
	return nil
}

func NormalizeEnvironmentHealthChecks(checks []EnvironmentHealthCheck) []EnvironmentHealthCheck {
	out := make([]EnvironmentHealthCheck, 0, len(checks))
	for _, check := range checks {
		check.CheckID = strings.TrimSpace(check.CheckID)
		check.Kind = strings.TrimSpace(check.Kind)
		check.URL = strings.TrimSpace(check.URL)
		check.Address = strings.TrimSpace(check.Address)
		check.Command = strings.TrimSpace(check.Command)
		check.ComposeService = strings.TrimSpace(check.ComposeService)
		check.Expect = strings.TrimSpace(check.Expect)
		if strings.TrimSpace(check.SummaryJSON) == "" {
			check.SummaryJSON = "{}"
		}
		out = append(out, check)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ApplyOrder != out[j].ApplyOrder {
			return out[i].ApplyOrder < out[j].ApplyOrder
		}
		return out[i].CheckID < out[j].CheckID
	})
	return out
}

func HydrateEnvironmentStructuredState(ctx context.Context, lister EnvironmentStructuredStateLister, env Environment) (Environment, error) {
	files, err := lister.ListEnvironmentFiles(ctx, env.ID)
	if err != nil {
		return Environment{}, err
	}
	env, err = MergeEnvironmentFilesIntoComposeJSON(env, files)
	if err != nil {
		return Environment{}, err
	}
	services, err := lister.ListEnvironmentServices(ctx, env.ID)
	if err != nil {
		return Environment{}, err
	}
	checks, err := lister.ListEnvironmentHealthChecks(ctx, env.ID)
	if err != nil {
		return Environment{}, err
	}
	return MergeEnvironmentRuntimeMetadataIntoJSON(env, services, checks)
}

func validEnvironmentFileKind(kind string) bool {
	switch kind {
	case EnvironmentFileKindComposeFile, EnvironmentFileKindComposeEnvFile, EnvironmentFileKindStartupFile:
		return true
	default:
		return false
	}
}

func cleanEnvironmentFilePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return ""
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	return clean
}

func stringMapFromJSONAny(value any) map[string]string {
	out := map[string]string{}
	if raw, ok := value.(map[string]string); ok {
		for key, item := range raw {
			out[key] = item
		}
		return out
	}
	if raw, ok := value.(map[string]any); ok {
		for key, item := range raw {
			if text, ok := item.(string); ok {
				out[key] = text
			}
		}
	}
	return out
}
