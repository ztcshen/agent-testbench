package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type doctorCommandReport struct {
	OK     bool                `json:"ok"`
	Checks []doctorCheckReport `json:"checks"`
	Next   []string            `json:"next"`
}

type doctorCheckReport struct {
	Name     string `json:"name"`
	Code     string `json:"code"`
	OK       bool   `json:"ok"`
	Optional bool   `json:"optional,omitempty"`
	Fixed    bool   `json:"fixed,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Fix      string `json:"fix,omitempty"`
}

type doctorOptions struct {
	Fix             bool
	Deep            bool
	TraceGraphQLURL string
}

const (
	doctorCheckActiveStore           = "active-store"
	doctorCheckTraceGraphQL          = "trace-graphql"
	doctorCodeRuntimeFreshness       = "runtime.fresh"
	doctorCodeRuntimeShellEntrypoint = "runtime.shell-entrypoint"
	doctorCodeTraceGraphQL           = "trace.graphql"
)

func runDoctor(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	fix := flags.Bool("fix", false, "Apply low-risk local setup repairs")
	deep := flags.Bool("deep", false, "Run slower diagnostics such as Docker Compose, Store schema, and optional trace endpoint checks")
	traceURL := flags.String("trace-graphql-url", strings.TrimSpace(os.Getenv("AGENT_TESTBENCH_TRACE_GRAPHQL_URL")), "SkyWalking GraphQL URL for deep reachability diagnostics")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable diagnostics report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected doctor arguments: %s", strings.Join(flags.Args(), " "))
	}
	report := buildDoctorReport(ctx, doctorOptions{Fix: *fix, Deep: *deep, TraceGraphQLURL: *traceURL})
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printDoctorReport(report)
	return nil
}

func buildDoctorReport(ctx context.Context, opts doctorOptions) doctorCommandReport {
	fixErr := ""
	if opts.Fix {
		if err := applyDoctorFixes(); err != nil {
			fixErr = err.Error()
		}
	}
	status := buildStatusReport(ctx, opts.Deep)
	checks := []doctorCheckReport{
		doctorToolCheck("git", false),
		doctorToolCheck("go", false),
		doctorToolCheck("npm", false),
		doctorToolCheck("docker", true),
		doctorRepoCheck(status.Repo),
		doctorStoreCheck(status.Store),
		doctorRuntimeDirectoryCheck(status.Runtime),
		doctorRuntimeCheck(status.Runtime),
		doctorRuntimeFreshnessCheck(status.Runtime),
		doctorShellEntrypointCheck(status.Runtime),
	}
	if fixErr != "" {
		checks = append(checks, doctorCheckReport{Name: "doctor-fix", Code: "doctor.fix", OK: false, Detail: fixErr, Fix: "check repository and config-home permissions, then rerun agent-testbench doctor --fix"})
	}
	if opts.Deep {
		checks = append(checks, doctorDockerComposeCheck(ctx))
		checks = append(checks, doctorStoreSchemaCheck(status.Store))
		if strings.TrimSpace(opts.TraceGraphQLURL) != "" {
			checks = append(checks, doctorTraceGraphQLCheck(ctx, opts.TraceGraphQLURL))
		}
	}
	ok := true
	for _, check := range checks {
		if !check.OK && !check.Optional {
			ok = false
			break
		}
	}
	return doctorCommandReport{OK: ok, Checks: checks, Next: status.Next}
}

func doctorShellEntrypointCheck(runtime statusRuntimeReport) doctorCheckReport {
	found, err := exec.LookPath("agent-testbench")
	if err != nil {
		return doctorCheckReport{
			Name:     "shell-entrypoint",
			Code:     doctorCodeRuntimeShellEntrypoint,
			OK:       false,
			Optional: true,
			Detail:   "agent-testbench is not on PATH",
			Fix:      "build the runtime with agent-testbench setup --build-runtime, then add " + filepath.Dir(runtime.Path) + " to PATH",
		}
	}
	found = filepath.Clean(found)
	if sameRuntimePath(found, runtime.Path) {
		return doctorCheckReport{Name: "shell-entrypoint", Code: doctorCodeRuntimeShellEntrypoint, OK: true, Optional: true, Detail: found}
	}
	return doctorCheckReport{
		Name:     "shell-entrypoint",
		Code:     doctorCodeRuntimeShellEntrypoint,
		OK:       false,
		Optional: true,
		Detail:   fmt.Sprintf("PATH resolves agent-testbench to %s, expected %s", found, runtime.Path),
		Fix:      "put " + filepath.Dir(runtime.Path) + " before stale wrappers on PATH, or set ATB_BIN=" + runtime.Path,
	}
}

func doctorDockerComposeCheck(ctx context.Context) doctorCheckReport {
	step := runUpdateCommandStep(ctx, ".", "docker-compose-version", "docker", "compose", "version")
	if !step.OK {
		return doctorCheckReport{Name: "docker-compose", Code: "docker.compose", OK: false, Optional: true, Detail: strings.TrimSpace(step.Error), Fix: "install Docker Compose before Docker-backed restore flows"}
	}
	return doctorCheckReport{Name: "docker-compose", Code: "docker.compose", OK: true, Optional: true, Detail: strings.TrimSpace(step.Output)}
}

func doctorStoreSchemaCheck(store statusStoreReport) doctorCheckReport {
	if !store.Configured {
		return doctorCheckReport{Name: "store-schema", Code: "store.schema", OK: false, Optional: true, Detail: "no active Store configured", Fix: "configure an active Store before deep Store diagnostics"}
	}
	schema := jsonObjectFromAny(store.Schema)
	if boolFromReportAny(schema["ok"]) {
		return doctorCheckReport{Name: "store-schema", Code: "store.schema", OK: true, Detail: fmt.Sprintf("%s pending=%d", store.Backend, intFromReportAny(schema["pending"]))}
	}
	return doctorCheckReport{Name: "store-schema", Code: "store.schema", OK: false, Detail: valueString(schema["error"]), Fix: "run agent-testbench store status --json and fix the Store connection or schema"}
}

func doctorTraceGraphQLCheck(ctx context.Context, rawURL string) doctorCheckReport {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	body := bytes.NewBufferString(`{"query":"{__typename}"}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, body)
	if err != nil {
		return doctorCheckReport{Name: doctorCheckTraceGraphQL, Code: doctorCodeTraceGraphQL, OK: false, Optional: true, Detail: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return doctorCheckReport{Name: doctorCheckTraceGraphQL, Code: doctorCodeTraceGraphQL, OK: false, Optional: true, Detail: err.Error(), Fix: "check AGENT_TESTBENCH_TRACE_GRAPHQL_URL or network reachability"}
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: close trace GraphQL response body: %v\n", closeErr)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return doctorCheckReport{Name: doctorCheckTraceGraphQL, Code: doctorCodeTraceGraphQL, OK: false, Optional: true, Detail: resp.Status, Fix: "check the SkyWalking GraphQL endpoint"}
	}
	return doctorCheckReport{Name: doctorCheckTraceGraphQL, Code: doctorCodeTraceGraphQL, OK: true, Optional: true, Detail: resp.Status}
}

var doctorFixedActiveStore bool
var doctorFixedRuntimeDirectory bool

func applyDoctorFixes() error {
	doctorFixedActiveStore = false
	doctorFixedRuntimeDirectory = false
	repo, err := resolveUpdateRepo("")
	if err != nil {
		return err
	}
	runtimeDir := filepath.Join(repo, ".runtime", "bin")
	if err := os.MkdirAll(runtimeDir, 0o755); err == nil {
		doctorFixedRuntimeDirectory = true
	}
	if _, err := activeStoreConfig(); err != nil {
		if errors.Is(err, errNoActiveStoreConfigured) {
			cfg, loadErr := loadStoreConfig()
			if loadErr != nil {
				return loadErr
			}
			if cfg.Stores == nil {
				cfg.Stores = map[string]storeConfigEntry{}
			}
			entry, entryErr := newStoreConfigEntry("local", "sqlite://"+filepath.Join(repo, ".runtime", "agent-testbench-local.sqlite"))
			if entryErr != nil {
				return entryErr
			}
			cfg.Stores[entry.Name] = entry
			cfg.Active = entry.Name
			if saveErr := saveStoreConfig(cfg); saveErr != nil {
				return saveErr
			}
			doctorFixedActiveStore = true
		}
	}
	return nil
}

func doctorActiveStoreIsFixed() bool {
	return doctorFixedActiveStore
}

func doctorRuntimeDirectoryWasFixed() bool {
	return doctorFixedRuntimeDirectory
}

func printDoctorReport(report doctorCommandReport) {
	fmt.Println("AgentTestBench Doctor")
	for _, check := range report.Checks {
		state := "ok"
		if !check.OK && check.Optional {
			state = "warn"
		} else if !check.OK {
			state = "issue"
		}
		fmt.Printf("- %s [%s] %s\n", check.Name, state, check.Detail)
		if check.Fix != "" {
			fmt.Printf("  fix: %s\n", check.Fix)
		}
	}
	printNextActions(report.Next)
}
