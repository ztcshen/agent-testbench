//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"agent-testbench/internal/runner/direct"
	"agent-testbench/internal/store/sqlite"
)

func TestDirectCLIUsesExplicitLightweightMode(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python is required for the native adapter integration")
	}
	project, err := filepath.Abs("../../examples/direct")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	docker := filepath.Join(root, "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\ntouch \""+filepath.Join(root, "docker-called")+"\"\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer target.Close()
	path := filepath.Join(root, "store.sqlite")
	runtime, err := sqlite.Open(context.Background(), sqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	dsn := "sqlite://" + path
	commands := [][]string{
		{"register", "--id", "native-demo", "--project", project, "--executable", python, "--arg", "adapter.py", "--target-url", target.URL, "--evidence-dir", filepath.Join(root, "runs")},
		{"discover"}, {"plan", "--binding", "native-demo", "--case", "case.health"},
		{"run", "--binding", "native-demo", "--case", "case.health", "--run-id", "cli-direct"},
		{"inspect", "--run", "cli-direct"},
	}
	for _, args := range commands {
		if err := runDirect(context.Background(), append(args, "--store", dsn, "--json")); err != nil {
			t.Fatalf("%s: %v", args[0], err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "docker-called")); !os.IsNotExist(err) {
		t.Fatal("direct mode invoked Docker")
	}
	runtime, err = sqlite.Open(context.Background(), sqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	report, err := direct.Inspect(context.Background(), runtime, "cli-direct")
	if err != nil || !report.OK {
		t.Fatalf("report=%#v err=%v", report, err)
	}
}

func TestServeDirectModeIsExplicitAndLoopbackOnly(t *testing.T) {
	config, err := serveConfigFromArgs([]string{"--mode", "direct"})
	if err != nil || config.mode != "direct" {
		t.Fatalf("config=%#v err=%v", config, err)
	}
	for _, args := range [][]string{{"--mode", "unknown"}, {"--mode", "direct", "--host", "0.0.0.0"}, {"--mode", "direct", "--profile", "unneeded"}} {
		if _, err := serveConfigFromArgs(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestDirectCommandsAreInDefaultDiscovery(t *testing.T) {
	report := commandCatalogForAreaWithOptions("", "", commandCatalogOptions{})
	want := map[string]bool{"direct register": false, "direct discover": false, "direct plan": false, "direct run": false, "direct inspect": false}
	for _, item := range report.Commands {
		if _, exists := want[item.Command]; exists {
			want[item.Command] = true
		}
	}
	for command, found := range want {
		if !found {
			t.Errorf("default discovery is missing %s", command)
		}
	}
}

func TestDirectRunExitRequiresAConfirmedPass(t *testing.T) {
	for _, test := range []struct {
		status       string
		ok, succeeds bool
	}{
		{"passed", true, true}, {"passed", false, false}, {"running", false, false}, {"skipped", false, false}, {"failed", false, false},
	} {
		err := directOutcomeError(direct.Report{Status: test.status, OK: test.ok, RunID: "example"})
		if (err == nil) != test.succeeds {
			t.Errorf("status=%s ok=%t err=%v", test.status, test.ok, err)
		}
	}
}
