package controlplane_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

type runtimeCatalogWorkflowFixture struct {
	Store        store.Store
	Services     []store.CatalogService
	StepServices []string
	Nodes        []store.CatalogInterfaceNode
	Cases        []store.CatalogAPICase
	Bindings     []store.CatalogWorkflowBinding
}

type runtimeCatalogPayload struct {
	OK          bool              `json:"ok"`
	GeneratedAt string            `json:"generatedAt"`
	Navigation  map[string]any    `json:"navigation"`
	Warnings    []string          `json:"warnings"`
	Source      map[string]string `json:"source"`
	Services    []struct {
		ID string `json:"id"`
	} `json:"services"`
	Workflows []runtimeCatalogWorkflow `json:"workflows"`
	APICases  []struct {
		ID     string `json:"id"`
		NodeID string `json:"nodeId"`
	} `json:"apiCases"`
}

type runtimeCatalogWorkflow struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	Description       string `json:"description"`
	Entrypoint        string `json:"entrypoint"`
	StepCount         int    `json:"stepCount"`
	CaseCount         int    `json:"caseCount"`
	ServiceCount      int    `json:"serviceCount"`
	BaseStepTimeoutMs int    `json:"baseStepTimeoutMs"`
	TimeoutOffsetMs   int    `json:"timeoutOffsetMs"`
	TimeoutMs         int    `json:"timeoutMs"`
	Graph             struct {
		Nodes []string `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"edges"`
	} `json:"graph"`
	Observability struct {
		Panels []struct {
			ID string `json:"id"`
		} `json:"panels"`
	} `json:"observability"`
	Presentation runtimeCatalogWorkflowPresentation `json:"presentation"`
	RunCount     int                                `json:"runCount"`
	LatestRun    struct {
		ID      string `json:"id"`
		Summary struct {
			Steps []struct {
				StepID    string `json:"stepId"`
				ElapsedMs int    `json:"elapsedMs"`
			} `json:"steps"`
		} `json:"summary"`
	} `json:"latestRun"`
	Steps []runtimeCatalogWorkflowStep `json:"steps"`
}

type runtimeCatalogWorkflowPresentation struct {
	Template string `json:"template"`
	Title    string `json:"title"`
	Copy     struct {
		RunButton     string `json:"runButton"`
		CoverageTitle string `json:"coverageTitle"`
		CoverageEmpty string `json:"coverageEmpty"`
	} `json:"copy"`
	Stages []struct {
		ID    string `json:"id"`
		Steps []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"steps"`
	} `json:"stages"`
}

type runtimeCatalogWorkflowStep struct {
	ID                 string           `json:"id"`
	DisplayName        string           `json:"displayName"`
	ServiceID          string           `json:"serviceId"`
	CaseID             string           `json:"caseId"`
	Required           bool             `json:"required"`
	Executable         bool             `json:"executable"`
	EvidenceKinds      []string         `json:"evidenceKinds"`
	RelatedMockTargets []string         `json:"relatedMockTargets"`
	Inputs             []map[string]any `json:"inputs"`
	Exports            []map[string]any `json:"exports"`
	TimeoutMs          int              `json:"timeoutMs"`
	Presentation       struct {
		Copy struct {
			TopologyTitle string `json:"topologyTitle"`
			RequestTitle  string `json:"requestTitle"`
			ResponseTitle string `json:"responseTitle"`
			LogsTitle     string `json:"logsTitle"`
			EmptyRun      string `json:"emptyRun"`
		} `json:"copy"`
	} `json:"presentation"`
}

func newRuntimeCatalogWorkflowFixture(t *testing.T) runtimeCatalogWorkflowFixture {
	t.Helper()
	ctx := context.Background()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(t.TempDir(), "sandbox.sqlite")})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close sqlite store: %v", err)
		}
	})
	fixture := runtimeCatalogWorkflowRows()
	fixture.Store = s
	if err := s.ReplaceProfileCatalog(ctx, runtimeCatalogProfile(fixture)); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
	if _, err := s.CreateRun(ctx, store.Run{
		ID:          "run.primary",
		ProfileID:   "sample",
		WorkflowID:  "workflow.primary",
		Status:      store.StatusPassed,
		SummaryJSON: `{"status":"passed","steps":[{"stepId":"step-01","elapsedMs":123},{"stepId":"step-02","elapsedMs":456}],"summary":{"stepCount":2,"elapsedMs":579}}`,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	return fixture
}

func runtimeCatalogWorkflowRows() runtimeCatalogWorkflowFixture {
	stepServices := []string{"entry-service", "worker-service", "entry-service"}
	nodes := make([]store.CatalogInterfaceNode, 0, len(stepServices))
	cases := make([]store.CatalogAPICase, 0, len(stepServices))
	bindings := make([]store.CatalogWorkflowBinding, 0, len(stepServices))
	for i, serviceID := range stepServices {
		sortOrder := i + 1
		nodeID := fmt.Sprintf("interface.step.%02d", sortOrder)
		caseID := fmt.Sprintf("case.step.%02d", sortOrder)
		nodes = append(nodes, store.CatalogInterfaceNode{
			ID:          nodeID,
			DisplayName: fmt.Sprintf("Step %02d Interface", sortOrder),
			ServiceID:   serviceID,
			Operation:   fmt.Sprintf("step.%02d", sortOrder),
			Method:      "POST",
			Path:        fmt.Sprintf("/steps/%02d", sortOrder),
			Status:      "active",
			TimeoutMs:   sortOrder * 100,
			SortOrder:   sortOrder,
		})
		cases = append(cases, store.CatalogAPICase{
			ID: caseID, DisplayName: fmt.Sprintf("Step %02d Case", sortOrder), NodeID: nodeID,
			CaseType: "happy_path", RequiredForAdmission: true, Status: "active", SortOrder: sortOrder,
		})
		bindings = append(bindings, store.CatalogWorkflowBinding{
			WorkflowID: "workflow.primary", StepID: fmt.Sprintf("step-%02d", sortOrder),
			NodeID: nodeID, CaseID: caseID, Required: true, SortOrder: sortOrder,
		})
	}
	return runtimeCatalogWorkflowFixture{
		Services: []store.CatalogService{
			{ID: "entry-service", DisplayName: "Entry", Kind: "app"},
			{ID: "worker-service", DisplayName: "Worker", Kind: "app"},
		},
		StepServices: stepServices,
		Nodes:        nodes,
		Cases:        cases,
		Bindings:     bindings,
	}
}

func runtimeCatalogProfile(fixture runtimeCatalogWorkflowFixture) store.ProfileCatalog {
	return store.ProfileCatalog{
		ProfileID: "sample",
		IndexedAt: time.Now().UTC(),
		Services:  fixture.Services,
		Workflows: []store.CatalogWorkflow{
			{ID: "workflow.primary", DisplayName: "Primary Workflow", Description: "Runs the primary workflow.", BaseStepTimeoutMs: 300, TimeoutOffsetMs: 50},
		},
		InterfaceNodes:   fixture.Nodes,
		APICases:         fixture.Cases,
		WorkflowBindings: fixture.Bindings,
		TemplateConfigs:  runtimeCatalogTemplateConfigs(),
	}
}

func runtimeCatalogTemplateConfigs() []store.CatalogTemplateConfig {
	return []store.CatalogTemplateConfig{
		{
			ID: "cfg.workflow.primary", TemplateID: "TPL-WORKFLOW-LONG-CHAIN-V1", WorkflowID: "workflow.primary",
			ScopeType: "workflow", ScopeID: "workflow.primary", Title: "Primary Workflow",
			Description: "Runs the primary workflow from runtime template configuration.",
			ConfigJSON:  `{"copy":{"runButton":"Run configured flow","coverageTitle":"Configured coverage","coverageEmpty":"No configured mappings."}}`,
			Status:      "needs-business-input", SortOrder: 1,
		},
		{
			ID: "cfg.workflow-step.default", TemplateID: "TPL-WORKFLOW-STEP-V1", WorkflowID: "workflow.primary",
			ScopeType: "step", ScopeID: "_default", Title: "Default workflow step presentation",
			ConfigJSON: `{"copy":{"topologyTitle":"Configured topology","requestTitle":"Configured request","responseTitle":"Configured response","logsTitle":"Configured logs","emptyRun":"No configured step run."}}`,
			Status:     "active", SortOrder: 0,
		},
		{
			ID: "cfg.step.one", TemplateID: "TPL-WORKFLOW-STEP-V1", WorkflowID: "workflow.primary",
			NodeID: "entry-service", ScopeType: "step", ScopeID: "step-01", Title: "Entry step",
			ConfigJSON: `{"serviceId":"worker-service","evidenceKinds":["request","response","logs"],"relatedMockTargets":["mock-a"],"inputs":[{"name":"order_id","source":"previous","required":false}],"exports":[{"name":"request_id","from":"response","path":"request_id"}],"copy":{"topologyTitle":"Entry topology"}}`,
			Status:     "active", SortOrder: 1,
		},
		{ID: "cfg.step.two", TemplateID: "TPL-WORKFLOW-STEP-V1", WorkflowID: "workflow.primary", NodeID: "entry-service", ScopeType: "step", ScopeID: "step-02", Title: "Worker step", Status: "active", SortOrder: 2},
		{ID: "cfg.edge.entry.worker", TemplateID: "TPL-ENVIRONMENT-OVERVIEW-V1", ScopeType: "topology-edge", ScopeID: "entry-service->worker-service", ConfigJSON: `{"from":"entry-service","to":"worker-service"}`, Status: "active", SortOrder: 1},
	}
}

func newRuntimeCatalogWorkflowServer(t *testing.T, runtime store.Store) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{
		ID:          "sample",
		DisplayName: "Sample Profile",
		Workflows:   []profile.Workflow{{ID: "workflow.profile", DisplayName: "Profile Workflow"}},
	}, runtime))
	t.Cleanup(server.Close)
	return server
}

func getRuntimeCatalogPayload(t *testing.T, baseURL string) runtimeCatalogPayload {
	t.Helper()
	resp, err := http.Get(baseURL + "/api/catalog")
	if err != nil {
		t.Fatalf("get catalog: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("catalog status = %d", resp.StatusCode)
	}
	var payload runtimeCatalogPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	return payload
}

func assertRuntimeCatalogEnvelope(t *testing.T, payload runtimeCatalogPayload, fixture runtimeCatalogWorkflowFixture) {
	t.Helper()
	if !payload.OK || payload.GeneratedAt == "" || payload.Navigation == nil || payload.Warnings == nil {
		t.Fatalf("catalog envelope = %#v", payload)
	}
	if payload.Source["kind"] != "store" || payload.Source["id"] != "sample" {
		t.Fatalf("catalog source = %#v", payload.Source)
	}
	if len(payload.Services) != len(fixture.Services) || len(payload.APICases) != len(fixture.Cases) {
		t.Fatalf("catalog inventory counts services=%d cases=%d", len(payload.Services), len(payload.APICases))
	}
}

func assertRuntimeCatalogWorkflowSummary(t *testing.T, payload runtimeCatalogPayload, fixture runtimeCatalogWorkflowFixture) runtimeCatalogWorkflow {
	t.Helper()
	if len(payload.Workflows) != 1 {
		t.Fatalf("workflow count = %d, payload=%#v", len(payload.Workflows), payload.Workflows)
	}
	workflow := payload.Workflows[0]
	if workflow.ID != "workflow.primary" || workflow.DisplayName != "Primary Workflow" || workflow.Description == "" {
		t.Fatalf("workflow metadata = %#v", workflow)
	}
	if workflow.StepCount != len(fixture.Bindings) || workflow.CaseCount != len(fixture.Bindings) || workflow.ServiceCount != 2 {
		t.Fatalf("workflow summary counts = %#v", workflow)
	}
	if workflow.BaseStepTimeoutMs != 300 || workflow.TimeoutOffsetMs != 50 || workflow.TimeoutMs != 650 {
		t.Fatalf("workflow timeout budget = %#v", workflow)
	}
	if workflow.Entrypoint != "/workflow-detail.html?id=workflow.primary" {
		t.Fatalf("workflow entrypoint = %q", workflow.Entrypoint)
	}
	if len(workflow.Graph.Nodes) != 2 || len(workflow.Graph.Edges) != 1 || workflow.Graph.Edges[0].From != "entry-service" || workflow.Graph.Edges[0].To != "worker-service" {
		t.Fatalf("workflow graph = %#v", workflow.Graph)
	}
	if len(workflow.Observability.Panels) != 5 || workflow.Observability.Panels[0].ID != "workflowGraph" {
		t.Fatalf("workflow observability = %#v", workflow.Observability)
	}
	if workflow.RunCount != 1 || workflow.LatestRun.ID != "run.primary" || len(workflow.LatestRun.Summary.Steps) != 0 {
		t.Fatalf("workflow run state = %#v", workflow)
	}
	return workflow
}

func assertRuntimeCatalogWorkflowPresentation(t *testing.T, workflow runtimeCatalogWorkflow) {
	t.Helper()
	presentation := workflow.Presentation
	if presentation.Template != "workflowStudio" || presentation.Title != "Primary Workflow" || len(presentation.Stages) != 1 || presentation.Stages[0].Steps[0].Title != "Entry step" {
		t.Fatalf("workflow presentation = %#v", presentation)
	}
	if presentation.Copy.RunButton != "Run configured flow" || presentation.Copy.CoverageTitle != "Configured coverage" || presentation.Copy.CoverageEmpty != "No configured mappings." {
		t.Fatalf("workflow presentation copy = %#v", presentation.Copy)
	}
}

func assertRuntimeCatalogWorkflowSteps(t *testing.T, workflow runtimeCatalogWorkflow, fixture runtimeCatalogWorkflowFixture) {
	t.Helper()
	if len(workflow.Steps) != len(fixture.Bindings) {
		t.Fatalf("workflow step count = %d", len(workflow.Steps))
	}
	for i, step := range workflow.Steps {
		assertRuntimeCatalogWorkflowStep(t, i, step, fixture.StepServices[i])
	}
}

func assertRuntimeCatalogWorkflowStep(t *testing.T, i int, step runtimeCatalogWorkflowStep, serviceID string) {
	t.Helper()
	wantStep := fmt.Sprintf("step-%02d", i+1)
	wantCase := fmt.Sprintf("case.step.%02d", i+1)
	wantService := serviceID
	if i == 0 {
		wantService = "worker-service"
	}
	if i == 1 {
		wantService = "entry-service"
	}
	if step.ID != wantStep || step.CaseID != wantCase || step.ServiceID != wantService || !step.Required {
		t.Fatalf("step %d = %#v", i, step)
	}
	if step.TimeoutMs != (i+1)*100 {
		t.Fatalf("step timeout %d = %#v", i, step)
	}
	if i == 0 {
		assertRuntimeCatalogFirstStep(t, step)
	}
}

func assertRuntimeCatalogFirstStep(t *testing.T, step runtimeCatalogWorkflowStep) {
	t.Helper()
	if step.DisplayName != "Entry step" {
		t.Fatalf("step template title = %#v", step)
	}
	if !step.Executable || len(step.EvidenceKinds) != 3 || step.EvidenceKinds[0] != "request" || len(step.RelatedMockTargets) != 1 {
		t.Fatalf("step runtime metadata = %#v", step)
	}
	if len(step.Inputs) != 1 || step.Inputs[0]["name"] != "order_id" || len(step.Exports) != 1 || step.Exports[0]["name"] != "request_id" {
		t.Fatalf("step inputs/exports = %#v", step)
	}
	if step.Presentation.Copy.TopologyTitle != "Entry topology" ||
		step.Presentation.Copy.RequestTitle != "Configured request" ||
		step.Presentation.Copy.ResponseTitle != "Configured response" ||
		step.Presentation.Copy.LogsTitle != "Configured logs" ||
		step.Presentation.Copy.EmptyRun != "No configured step run." {
		t.Fatalf("step presentation copy = %#v", step.Presentation.Copy)
	}
}
