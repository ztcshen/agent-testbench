package storecontract

import (
	"context"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

func requireEnvironmentContract(t *testing.T, ctx context.Context, s store.Store, started time.Time) store.Environment {
	t.Helper()

	env := requireEnvironmentCatalogContract(t, ctx, s, started)
	requireEnvironmentFilesContract(t, ctx, s, env.ID)
	requireEnvironmentRuntimeMetadataContract(t, ctx, s, env.ID)
	requireEnvironmentListContract(t, ctx, s)
	return env
}

func requireEnvironmentCatalogContract(t *testing.T, ctx context.Context, s store.Store, started time.Time) store.Environment {
	t.Helper()

	env, err := s.UpsertEnvironment(ctx, contractEnvironmentFixture())
	if err != nil {
		t.Fatalf("upsert environment: %v", err)
	}
	if env.CreatedAt.IsZero() || env.UpdatedAt.IsZero() {
		t.Fatalf("environment timestamps should be set: %#v", env)
	}
	env.LastVerificationRunID = contractRunID
	env.LastVerificationStatus = store.StatusPassed
	env.EvidenceComplete = true
	env.TopologyComplete = true
	env.Verified = true
	env.Status = "verified"
	env.LastVerifiedAt = started.Add(time.Minute)
	env, err = s.UpsertEnvironment(ctx, env)
	if err != nil {
		t.Fatalf("update environment verification: %v", err)
	}
	loadedEnv, err := s.GetEnvironment(ctx, "env.team.accepted")
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}
	if !loadedEnv.Verified || loadedEnv.LastVerificationStatus != store.StatusPassed || !loadedEnv.EvidenceComplete || !loadedEnv.TopologyComplete {
		t.Fatalf("loaded environment verification = %#v", loadedEnv)
	}
	if !jsonEqual(loadedEnv.ReposJSON, env.ReposJSON) || loadedEnv.VerificationWorkflowID != "workflow.smoke" {
		t.Fatalf("loaded environment catalog fields = %#v", loadedEnv)
	}
	return env
}

func contractEnvironmentFixture() store.Environment {
	return store.Environment{
		ID:                     "env.team.accepted",
		DisplayName:            "Team Accepted Environment",
		Description:            "Shared environment accepted by verification workflow",
		Status:                 "draft",
		ServicesJSON:           `[{"id":"service.alpha","repo":"../service-alpha"}]`,
		ReposJSON:              `{"service.alpha":{"url":"../service-alpha","branch":"main"}}`,
		ComposeJSON:            `{"composeFile":"docker-compose.yml","startCommand":"docker compose up -d"}`,
		HealthChecksJSON:       `[{"id":"alpha-health","url":"http://127.0.0.1:18080/health"}]`,
		VerificationWorkflowID: "workflow.smoke",
		SummaryJSON:            `{"owner":"team"}`,
	}
}

func requireEnvironmentFilesContract(t *testing.T, ctx context.Context, s store.Store, envID string) {
	t.Helper()

	files := contractEnvironmentFilesFixture()
	if err := s.ReplaceEnvironmentFiles(ctx, envID, files); err != nil {
		t.Fatalf("replace environment files: %v", err)
	}
	loadedFiles, err := s.ListEnvironmentFiles(ctx, envID)
	if err != nil {
		t.Fatalf("list environment files: %v", err)
	}
	if len(loadedFiles) != 2 || loadedFiles[0].Path != "compose/docker-compose.yml" || loadedFiles[1].Kind != store.EnvironmentFileKindComposeEnvFile {
		t.Fatalf("loaded environment files = %#v", loadedFiles)
	}
	loadedEnv, err := s.GetEnvironment(ctx, envID)
	if err != nil {
		t.Fatalf("get environment after files: %v", err)
	}
	if !jsonEqual(loadedEnv.ComposeJSON, `{"composeFile":"compose/docker-compose.yml","composeFiles":["compose/docker-compose.yml"],"envFiles":["compose/runtime.env"],"generatedFiles":{"compose/docker-compose.yml":"services:\n  service-alpha:\n    image: alpine:3.20\n","compose/runtime.env":"APP_MODE=test\n"},"startCommand":"docker compose up -d"}`) {
		t.Fatalf("structured files were not merged into compose json: %s", loadedEnv.ComposeJSON)
	}
}

func contractEnvironmentFilesFixture() []store.EnvironmentFile {
	return []store.EnvironmentFile{
		{
			Path:          "compose/docker-compose.yml",
			Kind:          store.EnvironmentFileKindComposeFile,
			ContentInline: "services:\n  service-alpha:\n    image: alpine:3.20\n",
			Required:      true,
			ApplyOrder:    10,
			SummaryJSON:   `{"source":"contract"}`,
		},
		{
			Path:          "compose/runtime.env",
			Kind:          store.EnvironmentFileKindComposeEnvFile,
			ContentInline: "APP_MODE=test\n",
			Required:      true,
			ApplyOrder:    20,
			SummaryJSON:   `{"source":"contract"}`,
		},
	}
}

func requireEnvironmentRuntimeMetadataContract(t *testing.T, ctx context.Context, s store.Store, envID string) {
	t.Helper()

	services := []store.EnvironmentService{
		{ServiceID: "service.alpha", RepoURL: "https://example.com/service-alpha.git", Branch: "main", Ref: "v1.0.0", Checkout: "service-alpha", SummaryJSON: `{"source":"contract"}`},
	}
	if err := s.ReplaceEnvironmentServices(ctx, envID, services); err != nil {
		t.Fatalf("replace environment services: %v", err)
	}
	loadedServices, err := s.ListEnvironmentServices(ctx, envID)
	if err != nil {
		t.Fatalf("list environment services: %v", err)
	}
	if len(loadedServices) != 1 || loadedServices[0].RepoURL != "https://example.com/service-alpha.git" || loadedServices[0].Checkout != "service-alpha" {
		t.Fatalf("loaded environment services = %#v", loadedServices)
	}
	checks := []store.EnvironmentHealthCheck{
		{CheckID: "health-alpha", Kind: "url", URL: "http://127.0.0.1:18080/health", ApplyOrder: 1, SummaryJSON: `{"source":"contract"}`},
		{CheckID: "health-seed", Kind: "compose-service", ComposeService: "seed", Expect: "service_completed_successfully", ApplyOrder: 2, SummaryJSON: `{"source":"contract"}`},
	}
	if err := s.ReplaceEnvironmentHealthChecks(ctx, envID, checks); err != nil {
		t.Fatalf("replace environment health checks: %v", err)
	}
	loadedChecks, err := s.ListEnvironmentHealthChecks(ctx, envID)
	if err != nil {
		t.Fatalf("list environment health checks: %v", err)
	}
	if len(loadedChecks) != 2 || loadedChecks[1].ComposeService != "seed" || loadedChecks[1].Expect != "service_completed_successfully" {
		t.Fatalf("loaded environment health checks = %#v", loadedChecks)
	}
	loadedEnv, err := s.GetEnvironment(ctx, envID)
	if err != nil {
		t.Fatalf("get environment after runtime metadata: %v", err)
	}
	if !jsonEqual(loadedEnv.ServicesJSON, `[{"branch":"main","checkout":"service-alpha","id":"service.alpha","ref":"v1.0.0","repo":"https://example.com/service-alpha.git"}]`) ||
		!jsonEqual(loadedEnv.ReposJSON, `{"service.alpha":{"branch":"main","checkout":"service-alpha","ref":"v1.0.0","url":"https://example.com/service-alpha.git"}}`) ||
		!jsonEqual(loadedEnv.HealthChecksJSON, `[{"id":"health-alpha","kind":"url","url":"http://127.0.0.1:18080/health"},{"expect":"service_completed_successfully","id":"health-seed","kind":"compose-service","service":"seed"}]`) {
		t.Fatalf("structured runtime metadata was not merged: services=%s repos=%s health=%s", loadedEnv.ServicesJSON, loadedEnv.ReposJSON, loadedEnv.HealthChecksJSON)
	}
}

func requireEnvironmentListContract(t *testing.T, ctx context.Context, s store.Store) {
	t.Helper()

	environments, err := s.ListEnvironments(ctx)
	if err != nil {
		t.Fatalf("list environments: %v", err)
	}
	if len(environments) != 1 || environments[0].ID != "env.team.accepted" || !environments[0].Verified {
		t.Fatalf("environments = %#v", environments)
	}
}
