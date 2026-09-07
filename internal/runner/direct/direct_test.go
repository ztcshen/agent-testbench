//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package direct_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"agent-testbench/internal/runner/direct"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

type fixture struct {
	runtime *sqlite.Store
	binding direct.Binding
	hits    atomic.Int32
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{}
	t.Setenv("DIRECT_TEST_ADAPTER", "1")
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)
	var err error
	f.runtime, err = sqlite.Open(context.Background(), sqlite.Config{Path: filepath.Join(t.TempDir(), "store.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.runtime.Close() })
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "case.json"), []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	f.binding = direct.Binding{
		ID: "local-tests", ProjectDir: project, Executable: os.Args[0],
		Args: []string{"-test.run=^TestDirectAdapter$"}, EnvironmentNames: []string{"DIRECT_TEST_ADAPTER", "GORACE"},
		TargetURL: server.URL, EvidenceDir: t.TempDir(), TimeoutSeconds: 1,
	}
	if _, err := direct.Register(context.Background(), f.runtime, direct.RegisterRequest{Binding: f.binding}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) run(t *testing.T, caseID, runID string) direct.Report {
	t.Helper()
	report, err := direct.Run(context.Background(), f.runtime, direct.RunRequest{BindingID: f.binding.ID, CaseID: caseID, RunID: runID})
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestDirectModeRecordsRealNativeResultsAndIsolatedState(t *testing.T) {
	f := newFixture(t)
	first := f.run(t, "pass", "first")
	second := f.run(t, "pass", "second")
	if !first.OK || !second.OK || first.Mode != "direct" || first.Snapshot.ManagesTarget {
		t.Fatalf("reports=%#v %#v", first, second)
	}
	if first.EvidenceDir == second.EvidenceDir || f.hits.Load() != 2 {
		t.Fatal("runs did not execute independently")
	}
	for _, report := range []direct.Report{first, second} {
		if _, err := os.Stat(filepath.Join(report.EvidenceDir, "adapter-state")); err != nil {
			t.Fatal(err)
		}
		if report.Result.Data["secret"] != "[REDACTED]" {
			t.Fatal("public result exposes secret")
		}
		rows, err := f.runtime.ListEvidence(context.Background(), report.RunID)
		if err != nil || len(rows) != 5 {
			t.Fatalf("evidence rows=%d err=%v", len(rows), err)
		}
		for _, row := range rows {
			if row.SHA256 == "" {
				t.Fatal("missing artifact hash")
			}
		}
		cases, err := f.runtime.ListAPICaseRuns(context.Background(), report.RunID)
		if err != nil || len(cases) != 1 || cases[0].Status != store.StatusPassed {
			t.Fatalf("cases=%#v err=%v", cases, err)
		}
	}
}

func TestDirectOutcomesPreserveFailureMeaning(t *testing.T) {
	f := newFixture(t)
	for _, test := range []struct{ id, status, category string }{
		{"fail", store.StatusFailed, "assertion"}, {"malformed", store.StatusFailed, "protocol"},
		{"dry", store.StatusFailed, "protocol"}, {"no-checks", store.StatusFailed, "protocol"},
		{"crash", store.StatusFailed, "process-exit"}, {"timeout", store.StatusFailed, "timeout"},
		{"skip", store.StatusSkipped, ""},
	} {
		t.Run(test.id, func(t *testing.T) {
			report := f.run(t, test.id, "run."+test.id)
			if report.OK || report.Status != test.status || report.FailureCategory != test.category {
				t.Fatalf("report=%#v", report)
			}
			stored, err := direct.Inspect(context.Background(), f.runtime, report.RunID)
			if err != nil || stored.Status != report.Status {
				t.Fatalf("stored=%#v err=%v", stored, err)
			}
		})
	}
}

func TestDirectRequestIdentityPreventsDuplicateProcesses(t *testing.T) {
	f := newFixture(t)
	request := direct.RunRequest{BindingID: f.binding.ID, CaseID: "pass", RunID: "same-request"}
	var group sync.WaitGroup
	errorsFound := make(chan error, 2)
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := direct.Run(context.Background(), f.runtime, request)
			errorsFound <- err
		}()
	}
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.hits.Load() != 1 {
		t.Fatalf("executed %d times", f.hits.Load())
	}
	request.Overrides = map[string]any{}
	if _, err := direct.Run(context.Background(), f.runtime, request); err != nil {
		t.Fatalf("equivalent empty overrides conflicted: %v", err)
	}
	request.Overrides = map[string]any{"changed": true}
	if _, err := direct.Run(context.Background(), f.runtime, request); !errors.Is(err, direct.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestSourceDriftRequiresExplicitRegistrationRefresh(t *testing.T) {
	f := newFixture(t)
	if err := os.WriteFile(filepath.Join(f.binding.ProjectDir, "case.json"), []byte(`{"version":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	request := direct.RunRequest{BindingID: f.binding.ID, CaseID: "pass", RunID: "changed-source"}
	if _, err := direct.BuildPlan(context.Background(), f.runtime, request); !errors.Is(err, direct.ErrConflict) {
		t.Fatalf("plan err=%v", err)
	}
	report := f.run(t, "pass", request.RunID)
	if report.FailureCategory != "source-drift" || f.hits.Load() != 0 {
		t.Fatalf("report=%#v", report)
	}
	if _, err := direct.Register(context.Background(), f.runtime, direct.RegisterRequest{Binding: f.binding}); err != nil {
		t.Fatal(err)
	}
	if !f.run(t, "pass", "refreshed").OK {
		t.Fatal("refreshed source did not run")
	}
}

func TestDirectAdapter(t *testing.T) {
	if os.Getenv("DIRECT_TEST_ADAPTER") != "1" {
		return
	}
	var request direct.AdapterRequest
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		os.Exit(2)
	}
	if request.Operation == "discover" {
		manifest := direct.Manifest{ProtocolVersion: 1, SourceFiles: []string{"case.json"}}
		for _, id := range []string{"pass", "fail", "malformed", "dry", "no-checks", "crash", "timeout", "skip", "large-data"} {
			manifest.Cases = append(manifest.Cases, direct.Case{ID: id, Title: id, Kind: "case", SourcePath: "case.json"})
		}
		_ = json.NewEncoder(os.Stdout).Encode(manifest)
		os.Exit(0)
	}
	switch request.CaseID {
	case "malformed":
		fmt.Print("invalid-json")
		os.Exit(0)
	case "crash":
		os.Exit(3)
	case "timeout":
		<-time.After(time.Hour)
	}
	result := direct.AdapterResult{ProtocolVersion: 1, CaseID: request.CaseID, Status: "passed", Executed: true}
	response, err := http.Get(request.TargetURL)
	passed := err == nil && response.StatusCode == http.StatusOK && request.CaseID != "fail"
	if response != nil {
		_ = response.Body.Close()
	}
	result.Checks = []direct.Check{{Name: "response", Passed: passed}}
	if !passed {
		result.Status = "failed"
	}
	if request.CaseID == "dry" {
		result.DryRun = true
	}
	if request.CaseID == "no-checks" {
		result.Checks = nil
	}
	if request.CaseID == "skip" {
		result.Status, result.Executed = "skipped", false
	}
	result.Data = map[string]any{"stateDir": request.StateDir, "secret": "synthetic-secret"}
	if request.CaseID == "large-data" {
		result.Data["body"] = strings.Repeat("x", 300<<10)
	}
	_ = os.WriteFile(filepath.Join(request.StateDir, "adapter-state"), []byte(request.RunID), 0o600)
	_ = json.NewEncoder(os.Stdout).Encode(result)
	if !passed {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestLargeNativePayloadKeepsVerdictAndRawArtifact(t *testing.T) {
	f := newFixture(t)
	report := f.run(t, "large-data", "large-result")
	if !report.OK || report.Result.Data["summaryTruncated"] != true {
		t.Fatalf("report=%#v", report)
	}
	raw, err := os.ReadFile(filepath.Join(report.EvidenceDir, "stdout.txt"))
	if err != nil || len(raw) < 300<<10 {
		t.Fatalf("raw artifact size=%d err=%v", len(raw), err)
	}
}
