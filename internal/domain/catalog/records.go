// Package catalog defines Store-first catalog records shared across boundaries.
package catalog

import "time"

type ConfigVersion struct {
	ID           string
	ProfileID    string
	SourcePath   string
	BundleDigest string
	SummaryJSON  string
	Active       bool
	PublishedAt  time.Time
	CreatedAt    time.Time
}

type ReadModel struct {
	ProfileID       string
	Key             string
	ConfigVersionID string
	PayloadJSON     string
	GeneratedAt     time.Time
	UpdatedAt       time.Time
}

type ProfileIndex struct {
	ProfileID    string
	BundlePath   string
	BundleDigest string
	SummaryJSON  string
	ImportedAt   time.Time
	UpdatedAt    time.Time
}

type ProfileCatalog struct {
	ProfileID        string
	IndexedAt        time.Time
	Services         []Service
	Workflows        []Workflow
	InterfaceNodes   []InterfaceNode
	InterfaceFields  []InterfaceNodeField
	APICases         []APICase
	RequestTemplates []RequestTemplate
	WorkflowBindings []WorkflowBinding
	CaseDependencies []CaseDependency
	Fixtures         []Fixture
	TemplateConfigs  []TemplateConfig
}

type ProfileCatalogIndex struct {
	ProfileID string
	IndexedAt time.Time
	Counts    ProfileCatalogCounts
}

type ProfileCatalogCounts struct {
	Services         int
	Workflows        int
	InterfaceNodes   int
	APICases         int
	RequestTemplates int
	WorkflowBindings int
	CaseDependencies int
	Fixtures         int
	Templates        int
	TemplateConfigs  int
}

type Service struct {
	ID                  string
	DisplayName         string
	Kind                string
	AttachedTemplateIDs []string
	GitURL              string
	GitBranch           string
	RepoEnv             string
	SourcePath          string
	ContainerName       string
	Image               string
	DockerService       string
	ServicePort         int
	ManagementPort      int
	MemoryMb            int
	CPUMilli            int
	StartupCommand      string
	HealthURL           string
	LogPath             string
	Status              string
	SortOrder           int
}

type Workflow struct {
	ID                string
	DisplayName       string
	Description       string
	BaseStepTimeoutMs int
	TimeoutOffsetMs   int
}

type InterfaceNode struct {
	ID          string
	DisplayName string
	ServiceID   string
	Operation   string
	Method      string
	Path        string
	TemplateID  string
	Version     string
	Status      string
	Tags        []string
	Description string
	TimeoutMs   int
	SortOrder   int
	CreatedAt   string
	UpdatedAt   string
}

type InterfaceNodeField struct {
	ID          string
	NodeID      string
	Direction   string
	FieldPath   string
	DisplayName string
	DataType    string
	Required    bool
	Bindable    bool
	PortType    string
	Status      string
	SortOrder   int
}

type APICase struct {
	ID                   string
	DisplayName          string
	Description          string
	NodeID               string
	CaseType             string
	Scenario             string
	Tags                 []string
	Priority             string
	Owner                string
	PayloadTemplateJSON  string
	RequestTemplateID    string
	PatchJSON            string
	RenderMode           string
	ExpectedJSON         string
	RequiredForAdmission bool
	Status               string
	SortOrder            int
	CasePath             string
	SourceKind           string
	SourcePath           string
	ExecutorID           string
	BaseURL              string
	EvidenceDir          string
	TimeoutSeconds       int
	DefaultOverridesJSON string
}

type RequestTemplate struct {
	ID           string
	DisplayName  string
	NodeID       string
	Method       string
	Path         string
	TemplateJSON string
	Version      string
	Status       string
	SortOrder    int
}

type WorkflowBinding struct {
	WorkflowID string
	StepID     string
	NodeID     string
	CaseID     string
	Required   bool
	SortOrder  int
}

type CaseDependency struct {
	ID           string
	CaseID       string
	FixtureID    string
	MappingsJSON string
	Required     bool
	Status       string
	SortOrder    int
}

type Fixture struct {
	ID               string
	DisplayName      string
	Kind             string
	DataJSON         string
	SourceWorkflowID string
	SourceUntilStep  string
	TTLSeconds       int
	Status           string
	SortOrder        int
}

type TemplateConfig struct {
	ID          string
	TemplateID  string
	NodeID      string
	WorkflowID  string
	ScopeType   string
	ScopeID     string
	Title       string
	Description string
	ConfigJSON  string
	Status      string
	SortOrder   int
}
