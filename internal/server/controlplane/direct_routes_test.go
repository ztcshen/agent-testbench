//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"

	"agent-testbench/internal/runner/direct"
	"agent-testbench/internal/store/sqlite"
)

func TestDirectAPIRegistersPlansAndRunsWithoutManagedEnvironment(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python is required for the native adapter integration")
	}
	project, err := filepath.Abs("../../../examples/direct")
	if err != nil {
		t.Fatal(err)
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer target.Close()
	runtime, err := sqlite.Open(context.Background(), sqlite.Config{Path: filepath.Join(t.TempDir(), "store.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	server := httptest.NewServer(NewDirectHandler(runtime))
	defer server.Close()
	registration := direct.RegisterRequest{Binding: direct.Binding{
		ID: "api-local", ProjectDir: project, Executable: python, Args: []string{"adapter.py"},
		TargetURL: target.URL, EvidenceDir: t.TempDir(),
	}}
	var registered direct.RegistrationResult
	directAPIPost(t, server.URL+"/api/direct/bindings", registration, &registered, http.StatusOK)
	if registered.Revision == 0 || len(registered.Registration.Manifest.Cases) != 1 {
		t.Fatalf("registration=%#v", registered)
	}
	request := direct.RunRequest{BindingID: "api-local", CaseID: "case.health", RunID: "api-run"}
	var plan direct.Plan
	directAPIPost(t, server.URL+"/api/direct/plan", request, &plan, http.StatusOK)
	if plan.Mode != "direct" || plan.ManagesTarget || plan.Runtime != "native-process" {
		t.Fatalf("plan=%#v", plan)
	}
	var report direct.Report
	directAPIPost(t, server.URL+"/api/direct/runs", request, &report, http.StatusOK)
	if !report.OK || report.Status != "passed" {
		t.Fatalf("report=%#v", report)
	}
	response, err := http.Get(server.URL + "/api/direct/runs/api-run")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("inspect status=%d", response.StatusCode)
	}
	var stored direct.Report
	if err := json.NewDecoder(response.Body).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	if stored.RunID != report.RunID || stored.Snapshot.SourceDigest == "" {
		t.Fatalf("stored=%#v", stored)
	}
	var invalid map[string]any
	directAPIPost(t, server.URL+"/api/direct/runs", map[string]any{"bindingId": "api-local", "caseId": "case.health", "command": "unregistered"}, &invalid, http.StatusBadRequest)
}

func directAPIPost(t *testing.T, endpoint string, value, result any, expectedStatus int) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		t.Fatalf("%s status=%d expected=%d", endpoint, response.StatusCode, expectedStatus)
	}
	if err := json.NewDecoder(response.Body).Decode(result); err != nil {
		t.Fatal(err)
	}
}

func TestDirectAPIRejectsBrowserCrossOriginExecution(t *testing.T) {
	handler := NewDirectHandler(nil)
	for _, test := range []struct{ host, origin string }{
		{"external.example", ""}, {"localhost:8123", "https://external.example"},
	} {
		request := httptest.NewRequest(http.MethodPost, "http://"+test.host+"/api/direct/runs", bytes.NewBufferString(`{}`))
		request.Header.Set("Origin", test.origin)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status=%d", response.Code)
		}
	}
}
