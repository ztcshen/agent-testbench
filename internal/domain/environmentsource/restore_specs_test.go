package environmentsource

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestRepoSpecsMergesServicesOverRepoMapAndResolvesCheckouts(t *testing.T) {
	workspace := t.TempDir()

	got := RepoSpecs(
		`{"api":{"url":"git@example.com:old/api.git","branch":"old","checkout":"old-api"},"db":{"url":"https://example.com/team/db.git","ref":"abc123"}}`,
		`[{"id":"api","repo":"git@example.com:team/api.git","branch":"main","checkout":"services/api"},{"id":"worker","repo":"https://example.com/team/worker.git"}]`,
		workspace,
	)
	want := []RepoSpec{
		{
			ServiceID: "api",
			URL:       "git@example.com:team/api.git",
			Branch:    "main",
			Checkout:  filepath.Join(workspace, "services/api"),
		},
		{
			ServiceID: "db",
			URL:       "https://example.com/team/db.git",
			Ref:       "abc123",
			Checkout:  filepath.Join(workspace, "db"),
		},
		{
			ServiceID: "worker",
			URL:       "https://example.com/team/worker.git",
			Checkout:  filepath.Join(workspace, "worker"),
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("repo specs mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestPackageSpecFromComposeDefaultsCheckoutToWorkspace(t *testing.T) {
	workspace := t.TempDir()

	got := PackageSpecFromCompose(map[string]any{
		"package": map[string]any{
			"url":    "https://example.com/team/env-package.git",
			"branch": "restore",
		},
	}, workspace)

	if got.URL != "https://example.com/team/env-package.git" || got.Branch != "restore" || got.Checkout != workspace {
		t.Fatalf("unexpected package spec: %#v", got)
	}
}

func TestSourcePolicyReportRequiresRemoteComponentReposWhenRemoteOnly(t *testing.T) {
	report := SourcePolicyReport([]RepoSpec{
		{ServiceID: "api", URL: "git@example.com:team/api.git"},
		{ServiceID: "worker", URL: "../worker"},
	}, true)

	if report.OK || !report.RemoteOnly || len(report.Violations) != 1 {
		t.Fatalf("expected one remote-only violation, got %#v", report)
	}
	if report.Violations[0] != "component worker must use a remote Git URL, got local path/source: ../worker" {
		t.Fatalf("unexpected violation: %q", report.Violations[0])
	}
}
