package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

func TestEnvironmentRestoreClonesRemoteReposForVerifiedWorkflow(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	remoteRepo := createBareGitRepo(t, "main")
	remoteURL := "https://example.test/entry-gateway.git"
	workspace := filepath.Join(t.TempDir(), "workspace")
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthServer.Close()
	fakeDockerEnv, dockerCallsPath := fakeDockerCommand(t)
	installGitRemoteFixture(t, filepath.Dir(dockerCallsPath), remoteURL, remoteRepo)

	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.restore",
		"--repo", "entry-gateway="+remoteURL,
		"--branch", "entry-gateway=main",
		"--checkout", "entry-gateway=services/entry-gateway",
		"--compose-file", "docker-compose.yml",
		"--start-command", "docker compose up -d",
		"--health-url", healthServer.URL+"/health",
		"--verification-workflow", "workflow.core-10",
	)
	sourceCompose := filepath.Join(t.TempDir(), "docker-compose.yml")
	writeFile(t, sourceCompose, "services: {}\n")
	runCLI(t, "environment", "startup-file", "put",
		"--store", "sqlite://"+storePath,
		"--file", "docker-compose.yml="+sourceCompose,
		"env.restore",
	)
	graphPath := filepath.Join(t.TempDir(), "graph.json")
	writeFile(t, graphPath, mustJSON(t, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "entry-gateway", Kind: "app", Role: "business-service", ComposeService: "entry-gateway", Required: true, HealthCheckJSON: `{"type":"url","url":"` + healthServer.URL + `/health"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
		},
	}))
	runCLI(t, "environment", "components", "replace", "--store", "sqlite://"+storePath, "--file", graphPath, "env.restore")

	dryRunOut := runCLIWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--json", "env.restore")
	var dryRun struct {
		OK                   bool   `json:"ok"`
		Executed             bool   `json:"executed"`
		VerificationWorkflow string `json:"verificationWorkflow"`
		Repos                []struct {
			ServiceID string   `json:"serviceId"`
			Action    string   `json:"action"`
			Checkout  string   `json:"checkout"`
			Command   []string `json:"command"`
		} `json:"repos"`
		Docker struct {
			OK       bool       `json:"ok"`
			Action   string     `json:"action"`
			Commands [][]string `json:"commands"`
		} `json:"docker"`
		Preflight struct {
			OK    bool `json:"ok"`
			Tools []struct {
				Name     string `json:"name"`
				Required bool   `json:"required"`
				OK       bool   `json:"ok"`
			} `json:"tools"`
			HeavySteps []string `json:"heavySteps"`
		} `json:"preflight"`
		Readiness struct {
			OK                         bool `json:"ok"`
			PauseBeforeHeavyValidation bool `json:"pauseBeforeHeavyValidation"`
			Items                      []struct {
				Name   string `json:"name"`
				OK     bool   `json:"ok"`
				Detail string `json:"detail"`
			} `json:"items"`
		} `json:"readiness"`
		NextActions []string `json:"nextActions"`
	}
	if err := json.Unmarshal([]byte(dryRunOut), &dryRun); err != nil {
		t.Fatalf("decode restore dry-run json: %v\n%s", err, dryRunOut)
	}
	expectedCheckout := filepath.Join(workspace, "services", "entry-gateway")
	if !dryRun.OK || dryRun.Executed || dryRun.VerificationWorkflow != "workflow.core-10" || len(dryRun.Repos) != 1 {
		t.Fatalf("restore dry-run report = %#v", dryRun)
	}
	if dryRun.Repos[0].ServiceID != "entry-gateway" || dryRun.Repos[0].Action != "clone" || dryRun.Repos[0].Checkout != expectedCheckout || strings.Join(dryRun.Repos[0].Command, " ") == "" {
		t.Fatalf("restore dry-run repo = %#v", dryRun.Repos[0])
	}
	if !dryRun.Docker.OK || dryRun.Docker.Action != "plan-docker-compose" || len(dryRun.Docker.Commands) == 0 || !commandSlicesContain(dryRun.Docker.Commands, "up") {
		t.Fatalf("restore dry-run docker plan = %#v", dryRun.Docker)
	}
	if !dryRun.Preflight.OK || !restorePreflightHasTool(dryRun.Preflight.Tools, "git", true) || !restorePreflightHasTool(dryRun.Preflight.Tools, "docker", true) || !restorePreflightHasTool(dryRun.Preflight.Tools, "docker compose", true) || len(dryRun.Preflight.HeavySteps) == 0 {
		t.Fatalf("restore dry-run preflight = %#v", dryRun.Preflight)
	}
	if !dryRun.Readiness.OK || !dryRun.Readiness.PauseBeforeHeavyValidation || !restoreReadinessHasItem(dryRun.Readiness.Items, "component-repositories", true, "will be cloned") || !restoreReadinessHasItem(dryRun.Readiness.Items, "compose-services-and-middleware", true, "including middleware") || !restoreReadinessHasItem(dryRun.Readiness.Items, "health-probes", true, "1 Store-backed") || !restoreReadinessHasItem(dryRun.Readiness.Items, "operator-pause", true, "pause before") {
		t.Fatalf("restore dry-run readiness = %#v", dryRun.Readiness)
	}
	if len(dryRun.NextActions) == 0 || !strings.Contains(strings.Join(dryRun.NextActions, "\n"), "workflow.core-10") {
		t.Fatalf("restore dry-run should anchor next actions to verification workflow: %#v", dryRun.NextActions)
	}
	if _, err := os.Stat(expectedCheckout); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create checkout, stat err=%v", err)
	}

	executeOut := runCLIWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--execute", "--json", "env.restore")
	var executed struct {
		OK        bool   `json:"ok"`
		RestoreID string `json:"restoreId"`
		Executed  bool   `json:"executed"`
		Repos     []struct {
			Action string `json:"action"`
			OK     bool   `json:"ok"`
		} `json:"repos"`
		Docker struct {
			OK           bool `json:"ok"`
			HealthChecks []struct {
				URL string `json:"url"`
				OK  bool   `json:"ok"`
			} `json:"healthChecks"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(executeOut), &executed); err != nil {
		t.Fatalf("decode restore execute json: %v\n%s", err, executeOut)
	}
	if !executed.OK || !executed.Executed || len(executed.Repos) != 1 || executed.Repos[0].Action != "clone" || !executed.Repos[0].OK {
		t.Fatalf("restore execute report = %#v", executed)
	}
	if !executed.Docker.OK || len(executed.Docker.HealthChecks) != 1 || !executed.Docker.HealthChecks[0].OK {
		t.Fatalf("restore execute docker report = %#v", executed.Docker)
	}
	dockerCalls, err := os.ReadFile(dockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	composePath := filepath.Join(workspace, "docker-compose.yml")
	if want := "compose -f " + composePath + " up -d"; !strings.Contains(string(dockerCalls), want) {
		t.Fatalf("fake docker calls missing %q:\n%s", want, dockerCalls)
	}
	if raw, err := os.ReadFile(filepath.Join(expectedCheckout, "README.md")); err != nil || !strings.Contains(string(raw), "restore fixture") {
		t.Fatalf("restored checkout missing fixture file raw=%q err=%v", raw, err)
	}
	inspectOut := runCLI(t, "environment", "inspect", "--store", "sqlite://"+storePath, "--json", "env.restore")
	var inspected struct {
		Environment struct {
			Summary struct {
				LastRestore struct {
					ID                   string `json:"id"`
					OK                   bool   `json:"ok"`
					Executed             bool   `json:"executed"`
					Phase                string `json:"phase"`
					VerificationWorkflow string `json:"verificationWorkflow"`
					Docker               struct {
						Action       string `json:"action"`
						OK           bool   `json:"ok"`
						HealthChecks int    `json:"healthChecks"`
						HealthPassed int    `json:"healthPassed"`
					} `json:"docker"`
					Repositories []struct {
						ServiceID string `json:"serviceId"`
						Action    string `json:"action"`
						OK        bool   `json:"ok"`
					} `json:"repositories"`
					Readiness struct {
						OK          bool `json:"ok"`
						FailedItems []struct {
							Name string `json:"name"`
						} `json:"failedItems"`
					} `json:"readiness"`
				} `json:"lastRestore"`
				RestoreAttempts []struct {
					ID    string `json:"id"`
					Phase string `json:"phase"`
				} `json:"restoreAttempts"`
			} `json:"summary"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(inspectOut), &inspected); err != nil {
		t.Fatalf("decode restored environment inspect json: %v\n%s", err, inspectOut)
	}
	lastRestore := inspected.Environment.Summary.LastRestore
	if lastRestore.ID != executed.RestoreID || !lastRestore.OK || !lastRestore.Executed || lastRestore.Phase != "completed" || lastRestore.VerificationWorkflow != "workflow.core-10" || lastRestore.Docker.Action != "run-docker-compose" || !lastRestore.Docker.OK || lastRestore.Docker.HealthChecks != 1 || lastRestore.Docker.HealthPassed != 1 || len(lastRestore.Repositories) != 1 || lastRestore.Repositories[0].Action != "clone" || !lastRestore.Repositories[0].OK {
		t.Fatalf("persisted restore summary = %#v; executed restore id=%s", lastRestore, executed.RestoreID)
	}
	if !lastRestore.Readiness.OK || len(lastRestore.Readiness.FailedItems) != 0 {
		t.Fatalf("persisted readiness summary = %#v", lastRestore.Readiness)
	}
	attempts := inspected.Environment.Summary.RestoreAttempts
	if len(attempts) != 2 || attempts[0].ID == attempts[1].ID || attempts[1].ID != executed.RestoreID || attempts[1].Phase != "completed" {
		t.Fatalf("persisted restore attempts = %#v; executed restore id=%s", attempts, executed.RestoreID)
	}
}

func TestEnvironmentRestorePreflightReportsMissingGitForMissingCheckout(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeBin := t.TempDir()
	writeFile(t, filepath.Join(fakeBin, "docker"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(fakeBin, "docker"), 0o755); err != nil {
		t.Fatalf("chmod fake docker: %v", err)
	}
	t.Setenv("PATH", fakeBin)
	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID:                     "env.preflight",
		ReposJSON:              `{"entry-gateway":{"url":"https://example.com/team/entry-gateway.git","checkout":"entry-gateway"}}`,
		ComposeJSON:            `{"composeFile":"docker-compose.yml"}`,
		HealthChecksJSON:       `[]`,
		VerificationWorkflowID: "workflow.core-10",
	}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{})
	if err != nil {
		t.Fatalf("build restore preflight report: %v", err)
	}
	if report.OK || report.Preflight.OK || !restoreTypedPreflightHasTool(report.Preflight.Tools, "git", false) || !restoreTypedPreflightHasTool(report.Preflight.Tools, "docker", true) {
		t.Fatalf("missing git preflight report = %#v", report.Preflight)
	}
}

func TestEnvironmentRestoreRequiresRemoteGitSourcesForSQLOneClickEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		storeURL string
	}{
		{name: "postgres", storeURL: "postgres://tester@127.0.0.1:5432/agent_testbench?sslmode=disable"},
		{name: "mysql", storeURL: "mysql://tester:secret@127.0.0.1:3306/agent_testbench?tls=false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := filepath.Join(t.TempDir(), "workspace")
			report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
				ID:                     "env.remote.sources." + tt.name,
				ReposJSON:              `{"llt":{"url":"/Users/zlh/codes/agent-testbench-llt-simulator","checkout":"llt"}}`,
				ComposeJSON:            `{"composeFile":"compose/docker-compose.yml","package":{"url":"/Users/zlh/codes/agent-testbench-validation","checkout":"."}}`,
				HealthChecksJSON:       `[{"kind":"url","url":"http://127.0.0.1:28080/health"}]`,
				VerificationWorkflowID: "workflow.core-10",
			}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{
				StoreURL: tt.storeURL,
			}, environmentRestoreDockerCleanupOptions{})
			if err != nil {
				t.Fatalf("build %s restore remote source policy report: %v", tt.name, err)
			}
			if report.OK || report.SourcePolicy.OK || !report.SourcePolicy.RemoteOnly || len(report.SourcePolicy.Violations) != 1 || report.Docker.Action != "skipped-due-to-source-policy" {
				t.Fatalf("%s remote source policy report = %#v", tt.name, report)
			}
			if !strings.Contains(report.SourcePolicy.Violations[0], "component llt") {
				t.Fatalf("%s source policy should only reject component repositories, got %#v", tt.name, report.SourcePolicy.Violations)
			}
			if !restoreTypedReadinessHasItem(report.Readiness.Items, "remote-git-sources", false, "remote Git URL") {
				t.Fatalf("%s readiness should include remote source violation: %#v", tt.name, report.Readiness.Items)
			}
		})
	}
}

func TestEnvironmentRestoreRequiresRemoteSourcesForSQLStoreBackends(t *testing.T) {
	for _, storeURL := range []string{
		"postgres://tester@127.0.0.1:5432/agent_testbench?sslmode=disable",
		"postgresql://tester@127.0.0.1:5432/agent_testbench?sslmode=disable",
		"mysql://tester:secret@127.0.0.1:3306/agent_testbench?tls=false",
	} {
		if !environmentRestoreRequiresRemoteSources(storeURL) {
			t.Fatalf("SQL Store URL should require remote restore sources: %s", storeURL)
		}
	}
	for _, storeURL := range []string{"", "sqlite:///tmp/agent-testbench.sqlite", "file:///tmp/agent-testbench.sqlite"} {
		if environmentRestoreRequiresRemoteSources(storeURL) {
			t.Fatalf("compatibility Store URL should not require SQL remote source policy: %s", storeURL)
		}
	}
}

func TestEnvironmentRestoreReportsComponentGraphReadiness(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID:                     "env.component.graph",
		ComposeJSON:            `{"startCommand":"true"}`,
		HealthChecksJSON:       `[{"kind":"url","url":"http://127.0.0.1:18080/health"}]`,
		VerificationWorkflowID: "workflow.core-10",
	}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:     "mysql",
				Kind:            "middleware",
				Role:            "database",
				ComposeService:  "mysql",
				Required:        true,
				RuntimeJSON:     `{}`,
				HealthCheckJSON: `{"type":"compose-service","service":"mysql"}`,
				SummaryJSON:     `{}`,
			},
			{
				ComponentID:     "service.alpha",
				Kind:            "app",
				Role:            "business-service",
				ComposeService:  "service-alpha",
				Required:        true,
				RuntimeJSON:     `{}`,
				HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/service-alpha/health"}`,
				SummaryJSON:     `{}`,
			},
		},
		Dependencies: []store.ComponentDependency{
			{
				ConsumerComponentID: "service.alpha",
				ProviderComponentID: "mysql",
				Phase:               "startup",
				Capability:          "sql",
				Required:            true,
				ProfileJSON:         `{}`,
			},
		},
		Assets: []store.ComponentConfigAsset{
			{
				OwnerComponentID:  "service.alpha",
				AssetID:           "service.alpha.mysql.ddl",
				AssetKind:         "mysql-ddl",
				TargetComponentID: "mysql",
				TargetPath:        "compose/mysql/init/service-alpha.sql",
				ContentInline:     "create table service_alpha_smoke (id bigint primary key);",
				SizeBytes:         int64(len("create table service_alpha_smoke (id bigint primary key);")),
				SummaryJSON:       `{}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("build component graph restore report: %v", err)
	}
	if !report.ComponentGraph.Configured || !report.ComponentGraph.OK || report.ComponentGraph.Components != 2 || report.ComponentGraph.BlockingDependencies != 1 || report.ComponentGraph.Assets != 1 || report.ComponentGraph.MissingHealthChecks != 0 {
		t.Fatalf("component graph report = %#v", report.ComponentGraph)
	}
	if strings.Join(report.ComponentGraph.BlockingOrder, ",") != "mysql,service.alpha" {
		t.Fatalf("blocking dependency order = %#v", report.ComponentGraph.BlockingOrder)
	}
	if !report.ComponentStartupPlan.OK || len(report.ComponentStartupPlan.Batches) != 2 || len(report.ComponentStartupPlan.HealthGates) != 2 {
		t.Fatalf("component startup plan = %#v", report.ComponentStartupPlan)
	}
	if got := report.ComponentStartupPlan.Batches[0].Components[0].ComponentID + "," + report.ComponentStartupPlan.Batches[1].Components[0].ComponentID; got != "mysql,service.alpha" {
		t.Fatalf("component startup batches = %s plan=%#v", got, report.ComponentStartupPlan)
	}
	if !restoreTypedReadinessHasItem(report.Readiness.Items, "component-graph", true, "2 component(s)") {
		t.Fatalf("readiness should include component graph item: %#v", report.Readiness.Items)
	}
	if !restoreTypedReadinessHasItem(report.Readiness.Items, "component-startup-plan", true, "2 startup batch") {
		t.Fatalf("readiness should include component startup plan item: %#v", report.Readiness.Items)
	}
}

func TestEnvironmentRestoreRequiresComponentGraphForSQLOneClick(t *testing.T) {
	tests := []struct {
		name     string
		storeURL string
	}{
		{name: "postgres", storeURL: "postgres://tester@127.0.0.1:5432/agent_testbench?sslmode=disable"},
		{name: "mysql", storeURL: "mysql://tester:secret@127.0.0.1:3306/agent_testbench?tls=false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := filepath.Join(t.TempDir(), "workspace")
			report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
				ID:                     "env." + tt.name + ".component.required",
				ComposeJSON:            `{"startCommand":"true"}`,
				HealthChecksJSON:       `[{"kind":"url","url":"http://127.0.0.1:18080/health"}]`,
				VerificationWorkflowID: "workflow.core-10",
			}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{
				StoreURL: tt.storeURL,
			}, environmentRestoreDockerCleanupOptions{})
			if err != nil {
				t.Fatalf("build %s restore without component graph: %v", tt.name, err)
			}
			if report.OK || report.Readiness.OK || report.ComponentGraph.Configured {
				t.Fatalf("%s restore without component graph should fail readiness: %#v", tt.name, report)
			}
			if !restoreTypedReadinessHasItem(report.Readiness.Items, "component-graph", false, "requires a Store component graph") {
				t.Fatalf("%s readiness should require component graph: %#v", tt.name, report.Readiness.Items)
			}
		})
	}
}

func TestEnvironmentRestoreRejectsBlockingComponentDependencyCycle(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID:                     "env.component.cycle",
		ComposeJSON:            `{"startCommand":"true"}`,
		HealthChecksJSON:       `[]`,
		VerificationWorkflowID: "workflow.core-10",
	}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:     "app.a",
				Kind:            "app",
				Role:            "business-service",
				ComposeService:  "app-a",
				Required:        true,
				RuntimeJSON:     `{}`,
				HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/app-a/health"}`,
				SummaryJSON:     `{}`,
			},
			{
				ComponentID:     "app.b",
				Kind:            "app",
				Role:            "business-service",
				ComposeService:  "app-b",
				Required:        true,
				RuntimeJSON:     `{}`,
				HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/app-b/health"}`,
				SummaryJSON:     `{}`,
			},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app.a", ProviderComponentID: "app.b", Phase: "startup", Capability: "http", Required: true, ProfileJSON: `{}`},
			{ConsumerComponentID: "app.b", ProviderComponentID: "app.a", Phase: "startup", Capability: "http", Required: true, ProfileJSON: `{}`},
		},
	})
	if err != nil {
		t.Fatalf("build component cycle restore report: %v", err)
	}
	if report.OK || report.ComponentGraph.OK || len(report.ComponentGraph.BlockingCycles) == 0 {
		t.Fatalf("blocking dependency cycle should fail restore graph: %#v", report.ComponentGraph)
	}
	if !strings.Contains(report.ComponentGraph.Error, "cycle") || !strings.Contains(report.ComponentGraph.Error, "app.a") || !strings.Contains(report.ComponentGraph.Error, "app.b") {
		t.Fatalf("cycle error should name the component path: %q", report.ComponentGraph.Error)
	}
	if !restoreTypedReadinessHasItem(report.Readiness.Items, "component-graph", false, "cycle") {
		t.Fatalf("readiness should include component cycle failure: %#v", report.Readiness.Items)
	}
}

func TestEnvironmentRestoreAllowsRuntimeComponentDependencyCycle(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID:                     "env.component.runtime-cycle",
		ComposeJSON:            `{"startCommand":"true"}`,
		HealthChecksJSON:       `[]`,
		VerificationWorkflowID: "workflow.core-10",
	}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:     "app.a",
				Kind:            "app",
				Role:            "business-service",
				ComposeService:  "app-a",
				Required:        true,
				RuntimeJSON:     `{}`,
				HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/app-a/health"}`,
				SummaryJSON:     `{}`,
			},
			{
				ComponentID:     "app.b",
				Kind:            "app",
				Role:            "business-service",
				ComposeService:  "app-b",
				Required:        true,
				RuntimeJSON:     `{}`,
				HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/app-b/health"}`,
				SummaryJSON:     `{}`,
			},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app.a", ProviderComponentID: "app.b", Phase: "runtime", Capability: "http", Required: true, ProfileJSON: `{}`},
			{ConsumerComponentID: "app.b", ProviderComponentID: "app.a", Phase: "runtime", Capability: "http", Required: true, ProfileJSON: `{}`},
		},
	})
	if err != nil {
		t.Fatalf("build runtime cycle restore report: %v", err)
	}
	if !report.OK || !report.ComponentGraph.OK || report.ComponentGraph.BlockingDependencies != 0 || report.ComponentGraph.RuntimeDependencies != 2 || len(report.ComponentGraph.BlockingCycles) != 0 {
		t.Fatalf("runtime dependency cycle should be allowed by blocking graph gate: %#v", report.ComponentGraph)
	}
	if strings.Join(report.ComponentGraph.BlockingOrder, ",") != "app.a,app.b" {
		t.Fatalf("runtime-only graph should have stable component order: %#v", report.ComponentGraph.BlockingOrder)
	}
}

func TestEnvironmentRestoreUsesComponentHealthChecksForReadiness(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID:                     "env.component.health",
		ComposeJSON:            `{"startCommand":"true"}`,
		HealthChecksJSON:       `[]`,
		VerificationWorkflowID: "workflow.core-10",
	}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:     "app",
				Kind:            "app",
				Role:            "business-service",
				ComposeService:  "app",
				Required:        true,
				RuntimeJSON:     `{}`,
				HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/actuator/health"}`,
				SummaryJSON:     `{}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("build component health restore report: %v", err)
	}
	if !report.OK || len(report.HealthChecks) != 1 {
		t.Fatalf("component health checks should be restore probes: report=%#v health=%#v", report, report.HealthChecks)
	}
	check, ok := report.HealthChecks[0].(map[string]any)
	if !ok || valueString(check["kind"]) != "url" || valueString(check["service"]) != "app" || valueString(check["url"]) != "http://127.0.0.1:18080/actuator/health" || valueString(check["componentId"]) != "app" {
		t.Fatalf("component health check was not normalized: %#v", report.HealthChecks)
	}
	if !restoreTypedReadinessHasItem(report.Readiness.Items, "health-probes", true, "1 Store-backed health probe") {
		t.Fatalf("readiness should count component health probes: %#v", report.Readiness.Items)
	}
}

func TestEnvironmentRestoreRequiresURLHealthForBusinessComponents(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID:                     "env.component.business-health",
		ComposeJSON:            `{"startCommand":"true"}`,
		HealthChecksJSON:       `[]`,
		VerificationWorkflowID: "workflow.core-10",
	}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:     "mysql",
				Kind:            "middleware",
				Role:            "database",
				ComposeService:  "mysql",
				Required:        true,
				RuntimeJSON:     `{}`,
				HealthCheckJSON: `{"type":"compose-service"}`,
				SummaryJSON:     `{}`,
			},
			{
				ComponentID:     "app",
				Kind:            "app",
				Role:            "business-service",
				ComposeService:  "app",
				Required:        true,
				RuntimeJSON:     `{}`,
				HealthCheckJSON: `{"type":"compose-service"}`,
				SummaryJSON:     `{}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("build business health restore report: %v", err)
	}
	if report.OK || report.ComponentGraph.OK || report.ComponentGraph.MissingHealthChecks != 1 {
		t.Fatalf("business service compose-only health should fail readiness: %#v", report.ComponentGraph)
	}
	if !strings.Contains(report.ComponentGraph.Error, "app: business-service health check requires url") {
		t.Fatalf("business health error should require url: %q", report.ComponentGraph.Error)
	}
}

func TestEnvironmentRestoreRejectsInvalidComponentHealthCheck(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID:                     "env.component.invalid-health",
		ComposeJSON:            `{"startCommand":"true"}`,
		HealthChecksJSON:       `[]`,
		VerificationWorkflowID: "workflow.core-10",
	}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:     "app",
				Kind:            "app",
				Role:            "business-service",
				Required:        true,
				RuntimeJSON:     `{}`,
				HealthCheckJSON: `{"kind":"url"}`,
				SummaryJSON:     `{}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("build invalid component health restore report: %v", err)
	}
	if report.OK || report.ComponentGraph.OK || report.ComponentGraph.MissingHealthChecks != 1 {
		t.Fatalf("component graph should reject invalid health check: %#v", report.ComponentGraph)
	}
	if !restoreTypedReadinessHasItem(report.Readiness.Items, "component-graph", false, "url health check requires url") {
		t.Fatalf("readiness should include invalid component health detail: %#v", report.Readiness.Items)
	}
}

func TestEnvironmentRestoreRejectsComponentRemoteAssetWithoutRemoteURL(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID:                     "env.component.remote-asset",
		ComposeJSON:            `{"startCommand":"true"}`,
		HealthChecksJSON:       `[{"kind":"url","url":"http://127.0.0.1:18080/health"}]`,
		VerificationWorkflowID: "workflow.core-10",
	}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:     "app",
				Kind:            "app",
				Role:            "business-service",
				Required:        true,
				RuntimeJSON:     `{}`,
				HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/app/health"}`,
				SummaryJSON:     `{}`,
			},
		},
		Assets: []store.ComponentConfigAsset{
			{
				OwnerComponentID: "app",
				AssetID:          "app.large-ddl",
				AssetKind:        "mysql-ddl",
				TargetPath:       "compose/mysql/init/app.sql",
				RemoteRefJSON:    `{"path":"compose/mysql/init/app.sql"}`,
				SizeBytes:        48 * 1024,
				SummaryJSON:      `{}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("build remote asset restore report: %v", err)
	}
	if report.ComponentGraph.OK || report.ComponentGraph.RemoteAssets != 1 || report.ComponentGraph.MissingRemoteAssetRefs != 1 {
		t.Fatalf("component graph remote asset report = %#v", report.ComponentGraph)
	}
	if !restoreTypedReadinessHasItem(report.Readiness.Items, "component-graph", false, "remote Git URL/path") {
		t.Fatalf("readiness should reject incomplete remote asset refs: %#v", report.Readiness.Items)
	}
}

func TestEnvironmentRestoreMaterializesRemoteComponentAsset(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	sourceCheckout := filepath.Join(t.TempDir(), "asset-source")
	runGit(t, "", "init", "-b", "main", sourceCheckout)
	writeFile(t, filepath.Join(sourceCheckout, "compose/mysql/init/app.sql"), "create table app_remote (id bigint primary key);\n")
	runGit(t, sourceCheckout, "add", ".")
	runGit(t, sourceCheckout, "-c", "user.name=Open Test", "-c", "user.email=open-test@example.com", "commit", "-m", "asset source")
	runGit(t, sourceCheckout, "remote", "add", "origin", "git@example.com:team/assets.git")

	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID:                     "env.component.remote-materialize",
		ComposeJSON:            `{"startCommand":"true"}`,
		HealthChecksJSON:       `[{"kind":"url","url":"http://127.0.0.1:18080/health"}]`,
		VerificationWorkflowID: "workflow.core-10",
	}, workspace, true, false, true, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:     "app",
				Kind:            "app",
				Role:            "business-service",
				Required:        true,
				RuntimeJSON:     `{}`,
				HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/app/health"}`,
				SummaryJSON:     `{}`,
			},
		},
		Assets: []store.ComponentConfigAsset{
			{
				OwnerComponentID: "app",
				AssetID:          "app.remote-ddl",
				AssetKind:        "mysql-ddl",
				TargetPath:       "compose/mysql/init/app.sql",
				RemoteRefJSON:    `{"url":"git@example.com:team/assets.git","checkout":"` + filepath.ToSlash(sourceCheckout) + `","path":"compose/mysql/init/app.sql"}`,
				SizeBytes:        48 * 1024,
				SummaryJSON:      `{}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("build remote asset materialize report: %v", err)
	}
	if !report.OK || len(report.ComponentAssets) != 1 || !report.ComponentAssets[0].OK || report.ComponentAssets[0].Action != "materialize" {
		t.Fatalf("remote component asset report = %#v", report)
	}
	raw, err := os.ReadFile(filepath.Join(workspace, "compose/mysql/init/app.sql"))
	if err != nil || !strings.Contains(string(raw), "app_remote") {
		t.Fatalf("remote component asset was not written raw=%q err=%v", raw, err)
	}
}

func TestEnvironmentRestoreSQLStoreUsesStoreGeneratedStartupFiles(t *testing.T) {
	tests := []struct {
		name     string
		storeURL string
	}{
		{name: "postgres", storeURL: "postgres://tester@127.0.0.1:5432/agent_testbench?sslmode=disable"},
		{name: "mysql", storeURL: "mysql://tester:secret@127.0.0.1:3306/agent_testbench?tls=false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := filepath.Join(t.TempDir(), "workspace")
			fakeBin := t.TempDir()
			writeFile(t, filepath.Join(fakeBin, "git"), "#!/bin/sh\nexit 0\n")
			writeFile(t, filepath.Join(fakeBin, "docker"), "#!/bin/sh\nif [ \"$1\" = compose ] && [ \"$2\" = version ]; then exit 0; fi\nexit 0\n")
			if err := os.Chmod(filepath.Join(fakeBin, "git"), 0o755); err != nil {
				t.Fatalf("chmod fake git: %v", err)
			}
			if err := os.Chmod(filepath.Join(fakeBin, "docker"), 0o755); err != nil {
				t.Fatalf("chmod fake docker: %v", err)
			}
			t.Setenv("PATH", fakeBin)

			report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
				ID:                     "env." + tt.name + ".generated",
				ReposJSON:              `{"llt":{"url":"git@github.com:ztcshen/agent-testbench-llt-simulator.git","checkout":"llt"}}`,
				ComposeJSON:            `{"composeFile":"compose/docker-compose.yml","composeFiles":["compose/docker-compose.yml"],"generatedFiles":{"compose/docker-compose.yml":"services:\n  llt:\n    image: alpine:3.20\n"},"package":{"url":"/Users/zlh/codes/agent-testbench-validation","checkout":"."}}`,
				HealthChecksJSON:       `[{"kind":"url","url":"http://127.0.0.1:28080/health"}]`,
				VerificationWorkflowID: "workflow.core-10",
			}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{
				StoreURL: tt.storeURL,
			}, environmentRestoreDockerCleanupOptions{})
			if err != nil {
				t.Fatalf("build %s restore generated startup report: %v", tt.name, err)
			}
			if !report.SourcePolicy.OK || !report.SourcePolicy.RemoteOnly || report.Package.Action != "ignored-for-sql-store-restore" || report.Docker.Action != "plan-docker-compose" {
				t.Fatalf("%s generated startup report = %#v", tt.name, report)
			}
			if len(report.Docker.Generated) != 1 || report.Docker.Generated[0].Action != "plan-write" || !report.Docker.Generated[0].OK {
				t.Fatalf("%s generated startup file report = %#v", tt.name, report.Docker.Generated)
			}
			if !restoreTypedReadinessHasItem(report.Readiness.Items, "store-startup-files", true, "generated from Store metadata") {
				t.Fatalf("%s readiness should accept Store generated startup files: %#v", tt.name, report.Readiness.Items)
			}
		})
	}
}

func TestEnvironmentRestoreSQLStoreRejectsLocalStartupFilesWithoutStoreGeneratedContent(t *testing.T) {
	tests := []struct {
		name     string
		storeURL string
	}{
		{name: "postgres", storeURL: "postgres://tester@127.0.0.1:5432/agent_testbench?sslmode=disable"},
		{name: "mysql", storeURL: "mysql://tester:secret@127.0.0.1:3306/agent_testbench?tls=false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := filepath.Join(t.TempDir(), "workspace")
			fakeBin := t.TempDir()
			writeFile(t, filepath.Join(fakeBin, "git"), "#!/bin/sh\nexit 0\n")
			writeFile(t, filepath.Join(fakeBin, "docker"), "#!/bin/sh\nif [ \"$1\" = compose ] && [ \"$2\" = version ]; then exit 0; fi\nexit 0\n")
			if err := os.Chmod(filepath.Join(fakeBin, "git"), 0o755); err != nil {
				t.Fatalf("chmod fake git: %v", err)
			}
			if err := os.Chmod(filepath.Join(fakeBin, "docker"), 0o755); err != nil {
				t.Fatalf("chmod fake docker: %v", err)
			}
			t.Setenv("PATH", fakeBin)

			report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
				ID:                     "env." + tt.name + ".local.compose",
				ReposJSON:              `{"llt":{"url":"git@github.com:ztcshen/agent-testbench-llt-simulator.git","checkout":"llt"}}`,
				ComposeJSON:            `{"composeFile":"compose/docker-compose.yml","composeFiles":["compose/docker-compose.yml"],"package":{"url":"/Users/zlh/codes/agent-testbench-validation","checkout":"."}}`,
				HealthChecksJSON:       `[{"kind":"url","url":"http://127.0.0.1:28080/health"}]`,
				VerificationWorkflowID: "workflow.core-10",
			}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{
				StoreURL: tt.storeURL,
			}, environmentRestoreDockerCleanupOptions{})
			if err != nil {
				t.Fatalf("build %s restore local startup report: %v", tt.name, err)
			}
			if !report.SourcePolicy.OK || report.Package.Action != "ignored-for-sql-store-restore" {
				t.Fatalf("%s local startup pre-readiness report = %#v", tt.name, report)
			}
			if !restoreTypedReadinessHasItem(report.Readiness.Items, "store-startup-files", false, "missing generatedFiles") {
				t.Fatalf("%s readiness should reject local startup files without Store content: %#v", tt.name, report.Readiness.Items)
			}
		})
	}
}

func TestEnvironmentRestoreSQLStoreRejectsMissingComposeStartupAssets(t *testing.T) {
	tests := []struct {
		name     string
		storeURL string
	}{
		{name: "postgres", storeURL: "postgres://tester@127.0.0.1:5432/agent_testbench?sslmode=disable"},
		{name: "mysql", storeURL: "mysql://tester:secret@127.0.0.1:3306/agent_testbench?tls=false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := filepath.Join(t.TempDir(), "workspace")
			fakeBin := t.TempDir()
			writeFile(t, filepath.Join(fakeBin, "git"), "#!/bin/sh\nexit 0\n")
			writeFile(t, filepath.Join(fakeBin, "docker"), "#!/bin/sh\nif [ \"$1\" = compose ] && [ \"$2\" = version ]; then exit 0; fi\nexit 0\n")
			if err := os.Chmod(filepath.Join(fakeBin, "git"), 0o755); err != nil {
				t.Fatalf("chmod fake git: %v", err)
			}
			if err := os.Chmod(filepath.Join(fakeBin, "docker"), 0o755); err != nil {
				t.Fatalf("chmod fake docker: %v", err)
			}
			t.Setenv("PATH", fakeBin)

			report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
				ID:                     "env." + tt.name + ".missing.assets",
				ReposJSON:              `{"app":{"url":"git@example.com:team/app.git","checkout":"app"}}`,
				ComposeJSON:            `{"composeFile":"compose/docker-compose.yml","composeFiles":["compose/docker-compose.yml"],"generatedFiles":{"compose/docker-compose.yml":"services:\n  mysql:\n    image: mysql:8\n    volumes:\n      - ./mysql/init:/docker-entrypoint-initdb.d\n  app:\n    image: alpine:3.20\n    command: [\"/bin/sh\", \"/sandbox/compose/scripts/run-app.sh\"]\n    volumes:\n      - ${DOCKER_APP_REPO:-/tmp/app}:/workspace/app\n      - ${SANDBOX_ROOT:-/tmp/sandbox}:/sandbox\n"},"env":{"DOCKER_APP_REPO":"$AGENT_TESTBENCH_WORKSPACE/app","SANDBOX_ROOT":"$AGENT_TESTBENCH_WORKSPACE"}}`,
				HealthChecksJSON:       `[{"kind":"url","url":"http://127.0.0.1:18080/health"}]`,
				VerificationWorkflowID: "workflow.core-10",
			}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{
				StoreURL: tt.storeURL,
			}, environmentRestoreDockerCleanupOptions{})
			if err != nil {
				t.Fatalf("build %s restore missing startup assets report: %v", tt.name, err)
			}
			if report.OK || report.Preflight.OK || len(report.Preflight.StartupAssets) != 2 {
				t.Fatalf("%s missing startup assets report = %#v", tt.name, report.Preflight.StartupAssets)
			}
			if !restoreTypedReadinessHasItem(report.Readiness.Items, "startup-assets", false, "compose/mysql/init") {
				t.Fatalf("%s readiness should include missing startup assets: %#v", tt.name, report.Readiness.Items)
			}
		})
	}
}

func TestEnvironmentRestoreSQLStoreAcceptsStoreGeneratedComposeStartupAssets(t *testing.T) {
	tests := []struct {
		name     string
		storeURL string
	}{
		{name: "postgres", storeURL: "postgres://tester@127.0.0.1:5432/agent_testbench?sslmode=disable"},
		{name: "mysql", storeURL: "mysql://tester:secret@127.0.0.1:3306/agent_testbench?tls=false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := filepath.Join(t.TempDir(), "workspace")
			fakeBin := t.TempDir()
			writeFile(t, filepath.Join(fakeBin, "git"), "#!/bin/sh\nexit 0\n")
			writeFile(t, filepath.Join(fakeBin, "docker"), "#!/bin/sh\nif [ \"$1\" = compose ] && [ \"$2\" = version ]; then exit 0; fi\nexit 0\n")
			if err := os.Chmod(filepath.Join(fakeBin, "git"), 0o755); err != nil {
				t.Fatalf("chmod fake git: %v", err)
			}
			if err := os.Chmod(filepath.Join(fakeBin, "docker"), 0o755); err != nil {
				t.Fatalf("chmod fake docker: %v", err)
			}
			t.Setenv("PATH", fakeBin)

			report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
				ID:                     "env." + tt.name + ".assets",
				ReposJSON:              `{"app":{"url":"git@example.com:team/app.git","checkout":"app"}}`,
				ComposeJSON:            `{"composeFile":"compose/docker-compose.yml","composeFiles":["compose/docker-compose.yml"],"generatedFiles":{"compose/docker-compose.yml":"services:\n  mysql:\n    image: mysql:8\n    volumes:\n      - ./mysql/init:/docker-entrypoint-initdb.d\n  app:\n    image: alpine:3.20\n    command: [\"/bin/sh\", \"/sandbox/compose/scripts/run-app.sh\"]\n    volumes:\n      - ${DOCKER_APP_REPO:-/tmp/app}:/workspace/app\n      - ${SANDBOX_ROOT:-/tmp/sandbox}:/sandbox\n","compose/mysql/init/schema.sql":"create database app;\n","compose/scripts/run-app.sh":"#!/bin/sh\nexit 0\n"},"env":{"DOCKER_APP_REPO":"$AGENT_TESTBENCH_WORKSPACE/app","SANDBOX_ROOT":"$AGENT_TESTBENCH_WORKSPACE"}}`,
				HealthChecksJSON:       `[{"kind":"url","url":"http://127.0.0.1:18080/health"}]`,
				VerificationWorkflowID: "workflow.core-10",
			}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{
				StoreURL: tt.storeURL,
			}, environmentRestoreDockerCleanupOptions{})
			if err != nil {
				t.Fatalf("build %s restore startup assets report: %v", tt.name, err)
			}
			if !report.Preflight.OK || len(report.Preflight.StartupAssets) != 2 {
				t.Fatalf("%s startup assets report = %#v readiness=%#v docker=%#v", tt.name, report.Preflight.StartupAssets, report.Readiness, report.Docker)
			}
			if !restoreTypedReadinessHasItem(report.Readiness.Items, "startup-assets", true, "2 Compose startup asset") {
				t.Fatalf("%s readiness should accept Store generated startup assets: %#v", tt.name, report.Readiness.Items)
			}
		})
	}
}

func TestEnvironmentRestoreMaterializesComponentAssetsAsStartupFiles(t *testing.T) {
	tests := []struct {
		name     string
		storeURL string
	}{
		{name: "postgres", storeURL: "postgres://tester@127.0.0.1:5432/agent_testbench?sslmode=disable"},
		{name: "mysql", storeURL: "mysql://tester:secret@127.0.0.1:3306/agent_testbench?tls=false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := filepath.Join(t.TempDir(), "workspace")
			fakeBin := t.TempDir()
			writeFile(t, filepath.Join(fakeBin, "git"), "#!/bin/sh\nexit 0\n")
			writeFile(t, filepath.Join(fakeBin, "docker"), "#!/bin/sh\nif [ \"$1\" = compose ] && [ \"$2\" = version ]; then exit 0; fi\nexit 0\n")
			if err := os.Chmod(filepath.Join(fakeBin, "git"), 0o755); err != nil {
				t.Fatalf("chmod fake git: %v", err)
			}
			if err := os.Chmod(filepath.Join(fakeBin, "docker"), 0o755); err != nil {
				t.Fatalf("chmod fake docker: %v", err)
			}
			t.Setenv("PATH", fakeBin)

			report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
				ID:                     "env." + tt.name + ".component.assets",
				ReposJSON:              `{"app":{"url":"https://example.com/team/app.git","checkout":"app"}}`,
				ComposeJSON:            `{"composeFile":"compose/docker-compose.yml","composeFiles":["compose/docker-compose.yml"],"generatedFiles":{"compose/docker-compose.yml":"services:\n  mysql:\n    image: mysql:8\n    volumes:\n      - ./mysql/init:/docker-entrypoint-initdb.d\n  app:\n    image: alpine:3.20\n    command: [\"/bin/sh\", \"/sandbox/compose/scripts/run-app.sh\"]\n    volumes:\n      - ${DOCKER_APP_REPO:-/tmp/app}:/workspace/app\n      - ${SANDBOX_ROOT:-/tmp/sandbox}:/sandbox\n"},"env":{"DOCKER_APP_REPO":"$AGENT_TESTBENCH_WORKSPACE/app","SANDBOX_ROOT":"$AGENT_TESTBENCH_WORKSPACE"}}`,
				HealthChecksJSON:       `[{"kind":"url","url":"http://127.0.0.1:18080/health"}]`,
				VerificationWorkflowID: "workflow.core-10",
			}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{
				StoreURL: tt.storeURL,
			}, environmentRestoreDockerCleanupOptions{}, store.EnvironmentComponentGraph{
				Components: []store.EnvironmentComponent{
					{ComponentID: "mysql", Kind: "middleware", Role: "database", Required: true, HealthCheckJSON: `{"type":"compose-service","service":"mysql"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
					{ComponentID: "app", Kind: "app", Role: "business-service", Required: true, HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/health"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
				},
				Assets: []store.ComponentConfigAsset{
					{OwnerComponentID: "app", AssetID: "app.mysql.schema", AssetKind: "mysql-ddl", TargetComponentID: "mysql", TargetPath: "compose/mysql/init/schema.sql", ContentInline: "create database app;\n", SummaryJSON: `{}`},
					{OwnerComponentID: "app", AssetID: "app.run-script", AssetKind: "container-start-script", TargetComponentID: "app", TargetPath: "compose/scripts/run-app.sh", ContentInline: "#!/bin/sh\nexit 0\n", SummaryJSON: `{}`},
				},
			})
			if err != nil {
				t.Fatalf("build %s restore component asset report: %v", tt.name, err)
			}
			if len(report.Preflight.StartupAssets) != 2 {
				t.Fatalf("%s component asset startup report = %#v readiness=%#v", tt.name, report.Preflight.StartupAssets, report.Readiness)
			}
			if !restoreTypedReadinessHasItem(report.Readiness.Items, "startup-assets", true, "2 Compose startup asset") {
				t.Fatalf("%s readiness should accept component asset startup files: %#v", tt.name, report.Readiness.Items)
			}
			if _, ok := stringMapFromAny(report.Compose["generatedFiles"])["compose/mysql/init/schema.sql"]; !ok {
				t.Fatalf("%s component schema asset was not projected into generatedFiles: %#v", tt.name, report.Compose["generatedFiles"])
			}
		})
	}
}

func TestEnvironmentRestoreOrdersComponentAssetsByBlockingDependencyOrder(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID:                     "env.component.asset-order",
		ComposeJSON:            `{"startCommand":"true"}`,
		HealthChecksJSON:       `[]`,
		VerificationWorkflowID: "workflow.core-10",
	}, workspace, false, false, true, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker", Required: true, HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18081/health"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
			{ComponentID: "db", Kind: "middleware", Role: "database", ComposeService: "db", Required: true, HealthCheckJSON: `{"type":"compose-service"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
			{ComponentID: "app", Kind: "app", Role: "business-service", ComposeService: "app", Required: true, HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/health"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app", ProviderComponentID: "db", Phase: "startup", Capability: "sql", Required: true, ProfileJSON: `{}`},
			{ConsumerComponentID: "worker", ProviderComponentID: "app", Phase: "startup", Capability: "http", Required: true, ProfileJSON: `{}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "worker", AssetID: "worker.remote", AssetKind: "script", TargetPath: "b-worker-remote.sh", RemoteRefJSON: `{"url":"git@example.com:team/assets.git","path":"b-worker-remote.sh"}`, ApplyOrder: 1, SummaryJSON: `{}`},
			{OwnerComponentID: "app", AssetID: "app.late", AssetKind: "config", TargetPath: "a-app-late.txt", ContentInline: "app late\n", ApplyOrder: 20, SummaryJSON: `{}`},
			{OwnerComponentID: "db", AssetID: "db.schema", AssetKind: "mysql-ddl", TargetPath: "z-db-schema.sql", ContentInline: "create database app;\n", ApplyOrder: 10, SummaryJSON: `{}`},
			{OwnerComponentID: "app", AssetID: "app.remote", AssetKind: "script", TargetPath: "c-app-remote.sh", RemoteRefJSON: `{"url":"git@example.com:team/assets.git","path":"c-app-remote.sh"}`, ApplyOrder: 5, SummaryJSON: `{}`},
			{OwnerComponentID: "app", AssetID: "app.early", AssetKind: "config", TargetPath: "d-app-early.txt", ContentInline: "app early\n", ApplyOrder: 1, SummaryJSON: `{}`},
		},
	})
	if err != nil {
		t.Fatalf("build component asset order report: %v", err)
	}
	if !report.OK {
		t.Fatalf("component asset order report should be OK: %#v", report)
	}
	if got := strings.Join(report.ComponentGraph.BlockingOrder, ","); got != "db,app,worker" {
		t.Fatalf("blocking order = %s", got)
	}
	var generatedPaths []string
	for _, item := range report.Docker.Generated {
		generatedPaths = append(generatedPaths, strings.TrimPrefix(item.Path, workspace+string(os.PathSeparator)))
	}
	if got := strings.Join(generatedPaths, ","); got != "z-db-schema.sql,d-app-early.txt,a-app-late.txt" {
		t.Fatalf("generated file order = %s reports=%#v", got, report.Docker.Generated)
	}
	var remoteAssetIDs []string
	for _, item := range report.ComponentAssets {
		remoteAssetIDs = append(remoteAssetIDs, item.AssetID)
	}
	if got := strings.Join(remoteAssetIDs, ","); got != "app.remote,worker.remote" {
		t.Fatalf("remote asset order = %s reports=%#v", got, report.ComponentAssets)
	}
}
