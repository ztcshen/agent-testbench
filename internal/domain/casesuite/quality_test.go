package casesuite

import (
	"context"
	"testing"

	"agent-testbench/internal/domain/profile"
)

func TestQualityAuditsMaintainedCaseAuthoringGaps(t *testing.T) {
	bundle := profile.Bundle{
		ID: "sample",
		InterfaceNodes: []profile.InterfaceNode{
			{ID: "node.alpha", DisplayName: "Node Alpha"},
			{ID: "node.beta", DisplayName: "Node Beta"},
			{ID: "node.empty", DisplayName: "Node Without Cases"},
		},
		APICases: []profile.APICase{
			{ID: "case.complete", DisplayName: "Complete Case", Description: "Covers the main path.", NodeID: "node.alpha", CasePath: "cases/complete.json", Tags: []string{"regression"}, Priority: "p0", Owner: "team-a", Status: "active", SortOrder: 1},
			{ID: "case.gaps", DisplayName: "Gap Case", NodeID: "node.beta", Status: "active", SortOrder: 2},
		},
		TemplateConfigs: []profile.TemplateConfig{
			{ID: "cfg.case.complete", ScopeType: "case", ScopeID: "case.complete", Status: "active", ConfigJSON: `{"caseId":"case.complete","caseExecution":{"method":"GET","path":"/items"}}`},
		},
	}
	cases := SelectCases(bundle, Filter{Status: "active"})

	report, err := Quality(context.Background(), bundle, recordStore{}, Filter{Status: "active"}, cases)
	if err != nil {
		t.Fatalf("quality: %v", err)
	}
	if report.OK || report.Counts.Nodes != 3 || report.Counts.NodesWithoutCases != 1 || report.Counts.Cases != 2 || report.Counts.CompleteCases != 1 || report.Counts.IncompleteCases != 1 {
		t.Fatalf("quality counts = %#v", report.Counts)
	}
	if report.Counts.MissingDescription != 1 || report.Counts.MissingTags != 1 || report.Counts.MissingPriority != 1 || report.Counts.MissingOwner != 1 || report.Counts.MissingRunnable != 1 || report.Counts.MissingExecution != 1 {
		t.Fatalf("quality gap counts = %#v", report.Counts)
	}
	byCase := map[string]QualityCase{}
	for _, item := range report.Cases {
		byCase[item.CaseID] = item
	}
	if !byCase["case.complete"].Complete || len(byCase["case.complete"].Issues) != 0 {
		t.Fatalf("complete case = %#v", byCase["case.complete"])
	}
	if byCase["case.gaps"].Complete || !containsString(byCase["case.gaps"].Issues, "missing-owner") || !containsString(byCase["case.gaps"].Issues, "missing-execution-config") {
		t.Fatalf("gap case = %#v", byCase["case.gaps"])
	}
	if len(report.Nodes) != 1 || report.Nodes[0].NodeID != "node.empty" || !containsString(report.Nodes[0].Issues, "no-maintained-cases") {
		t.Fatalf("quality nodes = %#v", report.Nodes)
	}
}

func TestQualityActiveFilterIgnoresInactiveInterfaceNodes(t *testing.T) {
	bundle := profile.Bundle{
		ID: "sample",
		InterfaceNodes: []profile.InterfaceNode{
			{ID: "node.active", DisplayName: "Active Node", Status: "active"},
			{ID: "node.inactive", DisplayName: "Inactive Node", Status: "inactive"},
		},
		APICases: []profile.APICase{
			{ID: "case.active", DisplayName: "Active Case", Description: "Ready.", NodeID: "node.active", CasePath: "cases/active.json", Tags: []string{"regression"}, Priority: "p0", Owner: "team-a", Status: "active"},
		},
		TemplateConfigs: []profile.TemplateConfig{
			{ID: "cfg.case.active", ScopeType: "case", ScopeID: "case.active", Status: "active", ConfigJSON: `{"caseId":"case.active","caseExecution":{"method":"GET","path":"/active"}}`},
		},
	}
	cases := SelectCases(bundle, Filter{Status: "active"})

	report, err := Quality(context.Background(), bundle, recordStore{}, Filter{Status: "active"}, cases)
	if err != nil {
		t.Fatalf("quality: %v", err)
	}
	if !report.OK || report.Counts.Nodes != 1 || report.Counts.NodesWithoutCases != 0 || len(report.Nodes) != 0 {
		t.Fatalf("active quality should ignore inactive nodes: counts=%#v nodes=%#v", report.Counts, report.Nodes)
	}
}

func TestInspectBlocksCaseWhenDependencyServiceCannotBeStartedOrChecked(t *testing.T) {
	bundle := profile.Bundle{
		ID: "sample",
		Services: []profile.Service{
			{ID: "service.bridge", DisplayName: "Bridge", Status: "active"},
		},
		InterfaceNodes: []profile.InterfaceNode{
			{ID: "node.bridge", DisplayName: "Bridge Node", ServiceID: "service.bridge"},
		},
		APICases: []profile.APICase{
			{ID: "case.bridge", DisplayName: "Bridge Case", Description: "Requires a local bridge.", NodeID: "node.bridge", CasePath: "cases/bridge.json", Tags: []string{"regression"}, Priority: "p0", Owner: "team-a", Status: "active"},
		},
		TemplateConfigs: []profile.TemplateConfig{
			{ID: "cfg.case.bridge", ScopeType: "case", ScopeID: "case.bridge", Status: "active", ConfigJSON: `{"caseId":"case.bridge","caseExecution":{"method":"POST","path":"/bridge"}}`},
		},
	}
	cases := SelectCases(bundle, Filter{Status: "active"})

	report, err := Inspect(context.Background(), bundle, recordStore{}, Filter{Status: "active"}, cases)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if report.OK || report.Counts.Ready != 0 || report.Counts.Blocked != 1 || len(report.Items) != 1 {
		t.Fatalf("inspection counts = %#v items=%#v", report.Counts, report.Items)
	}
	item := report.Items[0]
	if item.Ready || item.ServiceID != "service.bridge" || item.ServiceReady || !containsString(item.ServiceIssues, "missing-service-startup-command") {
		t.Fatalf("inspection should expose service readiness blocker: %#v", item)
	}
}

func TestInspectAcceptsComposeManagedDependencyService(t *testing.T) {
	bundle := profile.Bundle{
		ID: "sample",
		Services: []profile.Service{
			{
				ID:            "service.compose",
				DisplayName:   "Compose Service",
				Status:        "active",
				DockerService: "compose-api",
				ContainerName: "sample-compose-api",
				Image:         "example/compose-api:1",
			},
		},
		InterfaceNodes: []profile.InterfaceNode{
			{ID: "node.compose", DisplayName: "Compose Node", ServiceID: "service.compose"},
		},
		APICases: []profile.APICase{
			{ID: "case.compose", DisplayName: "Compose Case", Description: "Uses a compose service.", NodeID: "node.compose", CasePath: "cases/compose.json", Tags: []string{"regression"}, Priority: "p0", Owner: "team-a", Status: "active"},
		},
		TemplateConfigs: []profile.TemplateConfig{
			{ID: "cfg.case.compose", ScopeType: "case", ScopeID: "case.compose", Status: "active", ConfigJSON: `{"caseId":"case.compose","caseExecution":{"method":"GET","path":"/compose"}}`},
		},
	}
	cases := SelectCases(bundle, Filter{Status: "active"})

	report, err := Inspect(context.Background(), bundle, recordStore{}, Filter{Status: "active"}, cases)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !report.OK || report.Counts.Ready != 1 || report.Counts.Blocked != 0 || len(report.Items) != 1 {
		t.Fatalf("inspection counts = %#v items=%#v", report.Counts, report.Items)
	}
	item := report.Items[0]
	if !item.Ready || !item.ServiceReady || containsString(item.ServiceIssues, "missing-service-startup-command") {
		t.Fatalf("compose-managed service should be ready: %#v", item)
	}
}

func TestQualityAuditsCaseLifecycleStatus(t *testing.T) {
	bundle := profile.Bundle{
		ID: "sample",
		InterfaceNodes: []profile.InterfaceNode{
			{ID: "node.alpha", DisplayName: "Node Alpha"},
		},
		APICases: []profile.APICase{
			{ID: "case.active", DisplayName: "Active Case", Description: "Ready.", NodeID: "node.alpha", CasePath: "cases/active.json", Tags: []string{"regression"}, Priority: "p0", Owner: "team-a", Status: "active", SortOrder: 1},
			{ID: "case.review", DisplayName: "Review Case", Description: "Needs review.", NodeID: "node.alpha", CasePath: "cases/review.json", Tags: []string{"regression"}, Priority: "p1", Owner: "team-a", Status: "review", SortOrder: 2},
			{ID: "case.invalid", DisplayName: "Invalid Case", Description: "Bad status.", NodeID: "node.alpha", CasePath: "cases/invalid.json", Tags: []string{"regression"}, Priority: "p2", Owner: "team-a", Status: "paused", SortOrder: 3},
		},
		TemplateConfigs: []profile.TemplateConfig{
			{ID: "cfg.case.active", ScopeType: "case", ScopeID: "case.active", Status: "active", ConfigJSON: `{"caseId":"case.active","caseExecution":{"method":"GET","path":"/active"}}`},
			{ID: "cfg.case.review", ScopeType: "case", ScopeID: "case.review", Status: "active", ConfigJSON: `{"caseId":"case.review","caseExecution":{"method":"GET","path":"/review"}}`},
			{ID: "cfg.case.invalid", ScopeType: "case", ScopeID: "case.invalid", Status: "active", ConfigJSON: `{"caseId":"case.invalid","caseExecution":{"method":"GET","path":"/invalid"}}`},
		},
	}
	cases := SelectCases(bundle, Filter{})

	report, err := Quality(context.Background(), bundle, recordStore{}, Filter{}, cases)
	if err != nil {
		t.Fatalf("quality: %v", err)
	}
	if report.OK || report.Counts.Cases != 3 || report.Counts.CompleteCases != 1 || report.Counts.IncompleteCases != 2 || report.Counts.NonExecutableLifecycle != 2 || report.Counts.InvalidStatus != 1 {
		t.Fatalf("quality lifecycle counts = %#v", report.Counts)
	}
	byCase := map[string]QualityCase{}
	for _, item := range report.Cases {
		byCase[item.CaseID] = item
	}
	if !byCase["case.active"].Complete {
		t.Fatalf("active case = %#v", byCase["case.active"])
	}
	if byCase["case.review"].Complete || byCase["case.review"].Lifecycle != "review" || !containsString(byCase["case.review"].Issues, "non-executable-lifecycle") {
		t.Fatalf("review case = %#v", byCase["case.review"])
	}
	if byCase["case.invalid"].Complete || byCase["case.invalid"].Lifecycle != "invalid" || !containsString(byCase["case.invalid"].Issues, "invalid-status") {
		t.Fatalf("invalid case = %#v", byCase["case.invalid"])
	}

	plan, err := QualityPlan(context.Background(), bundle, recordStore{}, Filter{}, cases)
	if err != nil {
		t.Fatalf("quality plan: %v", err)
	}
	if plan.Counts.ReviewLifecycle != 2 {
		t.Fatalf("quality plan counts = %#v", plan.Counts)
	}
	lifecycleActions := 0
	for _, action := range plan.Actions {
		if action.Type == "review-case-lifecycle" {
			lifecycleActions++
			if !containsString(action.Issues, "non-executable-lifecycle") && !containsString(action.Issues, "invalid-status") {
				t.Fatalf("lifecycle action missing lifecycle issue = %#v", action)
			}
		}
	}
	if lifecycleActions != 2 {
		t.Fatalf("lifecycle action count = %d actions=%#v", lifecycleActions, plan.Actions)
	}
}

func TestQualityAcceptsExternalExecutorSourceAsRunnable(t *testing.T) {
	bundle := profile.Bundle{
		ID: "sample",
		InterfaceNodes: []profile.InterfaceNode{
			{ID: "node.alpha", DisplayName: "Node Alpha"},
		},
		Executors: []profile.ExecutorDescriptor{
			{ID: "executor.karate", Kind: "karate", SourcePath: "tests/api.feature", Status: "active"},
		},
		APICases: []profile.APICase{
			{
				ID:          "case.karate",
				DisplayName: "Karate Case",
				Description: "Runs through an external Karate feature.",
				NodeID:      "node.alpha",
				Tags:        []string{"regression"},
				Priority:    "p0",
				Owner:       "team-a",
				Status:      "active",
				SourceKind:  "karate",
				SourcePath:  "tests/api.feature",
				ExecutorID:  "executor.karate",
			},
			{
				ID:          "case.missing-executor",
				DisplayName: "Missing Executor Case",
				Description: "References an external source without an executor.",
				NodeID:      "node.alpha",
				Tags:        []string{"regression"},
				Priority:    "p1",
				Owner:       "team-a",
				Status:      "active",
				SourceKind:  "karate",
				SourcePath:  "tests/missing.feature",
				ExecutorID:  "executor.missing",
			},
		},
	}
	cases := SelectCases(bundle, Filter{Status: "active"})

	report, err := Quality(context.Background(), bundle, recordStore{}, Filter{Status: "active"}, cases)
	if err != nil {
		t.Fatalf("quality: %v", err)
	}
	if report.OK || report.Counts.Cases != 2 || report.Counts.CompleteCases != 1 || report.Counts.IncompleteCases != 1 || report.Counts.MissingRunnable != 0 || report.Counts.MissingExecution != 1 {
		t.Fatalf("quality external source counts = %#v", report.Counts)
	}
	byCase := map[string]QualityCase{}
	for _, item := range report.Cases {
		byCase[item.CaseID] = item
	}
	if !byCase["case.karate"].Complete || !byCase["case.karate"].HasRunnableFile || !byCase["case.karate"].HasExecutionConfig {
		t.Fatalf("karate case = %#v", byCase["case.karate"])
	}
	if byCase["case.missing-executor"].Complete || !containsString(byCase["case.missing-executor"].Issues, "missing-executor") {
		t.Fatalf("missing executor case = %#v", byCase["case.missing-executor"])
	}
}

func TestQualityPlanBuildsActionableAuthoringSteps(t *testing.T) {
	bundle := profile.Bundle{
		ID: "sample",
		InterfaceNodes: []profile.InterfaceNode{
			{ID: "node.alpha", DisplayName: "Node Alpha"},
			{ID: "node.empty", DisplayName: "Node Empty"},
		},
		APICases: []profile.APICase{
			{ID: "case.complete", DisplayName: "Complete Case", Description: "Covers the main path.", NodeID: "node.alpha", CasePath: "cases/complete.json", Tags: []string{"regression"}, Priority: "p0", Owner: "team-a", Status: "active", SortOrder: 1},
			{ID: "case.gaps", DisplayName: "Gap Case", NodeID: "node.alpha", Status: "active", SortOrder: 2},
		},
		TemplateConfigs: []profile.TemplateConfig{
			{ID: "cfg.case.complete", ScopeType: "case", ScopeID: "case.complete", Status: "active", ConfigJSON: `{"caseId":"case.complete","caseExecution":{"method":"GET","path":"/items"}}`},
		},
	}
	cases := SelectCases(bundle, Filter{Status: "active"})

	report, err := QualityPlan(context.Background(), bundle, recordStore{}, Filter{Status: "active"}, cases)
	if err != nil {
		t.Fatalf("quality plan: %v", err)
	}
	if !report.OK || report.Counts.Total != 4 || report.Counts.DraftCase != 1 || report.Counts.CompleteMetadata != 1 || report.Counts.AddRunnable != 1 || report.Counts.AddExecution != 1 {
		t.Fatalf("quality plan counts = %#v", report.Counts)
	}
	actionsByType := map[string]QualityPlanAction{}
	for _, action := range report.Actions {
		actionsByType[action.Type] = action
	}
	if actionsByType["draft-case"].NodeID != "node.empty" || actionsByType["draft-case"].SuggestedCaseID != "case.node-empty.default" || !containsString(actionsByType["draft-case"].Command, "draft") {
		t.Fatalf("draft action = %#v", actionsByType["draft-case"])
	}
	if actionsByType["complete-case-metadata"].CaseID != "case.gaps" || !containsString(actionsByType["complete-case-metadata"].Fields, "owner") {
		t.Fatalf("metadata action = %#v", actionsByType["complete-case-metadata"])
	}
	if actionsByType["add-runnable-source"].CaseID != "case.gaps" {
		t.Fatalf("runnable action = %#v", actionsByType["add-runnable-source"])
	}
	if actionsByType["add-execution-config"].CaseID != "case.gaps" {
		t.Fatalf("execution action = %#v", actionsByType["add-execution-config"])
	}
}
