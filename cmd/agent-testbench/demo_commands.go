package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-testbench/internal/domain/apicasespec"
	"agent-testbench/internal/runner/apicase"
	"agent-testbench/internal/store/mysql"
	"agent-testbench/internal/store/postgres"
	"agent-testbench/internal/store/sqlite"
)

const demoDefaultRunPrefix = "demo-create-item"

type demoCommandOptions struct {
	outputDir   string
	evidenceDir string
	storeRef    string
	runID       string
	profileID   string
	jsonOutput  bool
	clean       bool
}

type demoCommandReport struct {
	OK                 bool   `json:"ok"`
	RunID              string `json:"runId"`
	CaseID             string `json:"caseId"`
	Status             string `json:"status"`
	Store              string `json:"store"`
	OutputRoot         string `json:"outputRoot"`
	OutputRetained     bool   `json:"outputRetained"`
	EvidencePath       string `json:"evidencePath"`
	DemoEndpoint       string `json:"demoEndpoint"`
	NextInspectCommand string `json:"nextInspectCommand,omitempty"`
}

func runDemo(ctx context.Context, args []string) error {
	options, err := parseDemoCommandOptions(args)
	if err != nil {
		return err
	}
	report, err := executeDemo(ctx, options)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeIndentedJSON(report)
	}
	printDemoReport(report)
	return nil
}

func parseDemoCommandOptions(args []string) (demoCommandOptions, error) {
	flags := flag.NewFlagSet("demo", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	options := demoCommandOptions{}
	outputDir := flags.String("output-dir", options.outputDir, "Demo output directory; defaults to a temporary directory")
	evidenceDir := flags.String("evidence-dir", options.evidenceDir, "Evidence output directory; defaults to OUTPUT_DIR/evidence")
	storeRef := flags.String("store", options.storeRef, "Named Store config or Store DSN; defaults to OUTPUT_DIR/store.sqlite")
	runID := flags.String("run-id", options.runID, "Run id; defaults to a demo-create-item timestamp")
	profileID := flags.String("profile", "demo", "Profile id for Store records")
	jsonOutput := flags.Bool("json", options.jsonOutput, "Emit a machine-readable JSON report")
	clean := flags.Bool("clean", options.clean, "Remove demo output after a successful run")
	if err := flags.Parse(args); err != nil {
		return demoCommandOptions{}, err
	}
	if flags.NArg() != 0 {
		return demoCommandOptions{}, fmt.Errorf("unexpected demo arguments: %s", strings.Join(flags.Args(), " "))
	}
	options.outputDir = *outputDir
	options.evidenceDir = *evidenceDir
	options.storeRef = *storeRef
	options.runID = *runID
	options.profileID = *profileID
	options.jsonOutput = *jsonOutput
	options.clean = *clean
	return options, nil
}

func executeDemo(ctx context.Context, options demoCommandOptions) (demoCommandReport, error) {
	outputRoot, cleanup, err := prepareDemoOutputRoot(options.outputDir)
	if err != nil {
		return demoCommandReport{}, err
	}
	if options.clean {
		defer cleanup()
	}

	evidenceDir := strings.TrimSpace(options.evidenceDir)
	if evidenceDir == "" {
		evidenceDir = filepath.Join(outputRoot, "evidence")
	}
	storeRef := strings.TrimSpace(options.storeRef)
	if storeRef == "" {
		storeRef = "sqlite://" + filepath.Join(outputRoot, "store.sqlite")
	}
	storeURL, err := resolveRequiredDailyStoreReference(storeRef, "")
	if err != nil {
		return demoCommandReport{}, err
	}
	if err := upgradeDemoStoreSchema(ctx, storeURL); err != nil {
		return demoCommandReport{}, err
	}

	server := httptest.NewServer(http.HandlerFunc(handleDemoAPIRequest))
	defer server.Close()

	casePath, err := writeDemoCase(outputRoot)
	if err != nil {
		return demoCommandReport{}, err
	}
	runID := strings.TrimSpace(options.runID)
	if runID == "" {
		runID = demoDefaultRunPrefix + "-" + time.Now().UTC().Format("20060102T150405")
	}
	result, err := apicase.Run(ctx, apicase.RunOptions{
		CasePath:    casePath,
		BaseURL:     server.URL,
		EvidenceDir: evidenceDir,
		RunID:       runID,
	})
	if err != nil {
		return demoCommandReport{}, err
	}
	if err := indexCaseRun(ctx, storeURL, options.profileID, result); err != nil {
		return demoCommandReport{}, err
	}

	maskedStore := maskStoreURL(storeURL)
	nextInspectCommand := ""
	if !options.clean {
		nextInspectCommand = fmt.Sprintf("agent-testbench case inspect --view runs --store %s --run %s --json", maskedStore, result.RunID)
	}
	return demoCommandReport{
		OK:                 result.Status == "passed",
		RunID:              result.RunID,
		CaseID:             result.CaseID,
		Status:             result.Status,
		Store:              maskedStore,
		OutputRoot:         outputRoot,
		OutputRetained:     !options.clean,
		EvidencePath:       result.EvidencePath,
		DemoEndpoint:       server.URL,
		NextInspectCommand: nextInspectCommand,
	}, nil
}

func prepareDemoOutputRoot(outputDir string) (string, func(), error) {
	if strings.TrimSpace(outputDir) == "" {
		dir, err := os.MkdirTemp("", "agent-testbench-demo-")
		if err != nil {
			return "", func() {}, fmt.Errorf("create demo output directory: %w", err)
		}
		return dir, func() { _ = os.RemoveAll(dir) }, nil
	}
	absolute, err := filepath.Abs(outputDir)
	if err != nil {
		return "", func() {}, fmt.Errorf("resolve demo output directory: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return "", func() {}, fmt.Errorf("create demo output directory: %w", err)
	}
	return absolute, func() { _ = os.RemoveAll(absolute) }, nil
}

func upgradeDemoStoreSchema(ctx context.Context, storeURL string) error {
	backend, err := storeBackendFromURL(storeURL)
	if err != nil {
		return err
	}
	switch backend {
	case "sqlite":
		cfg, err := sqlite.ParseConfigFromURL(storeURL)
		if err != nil {
			return err
		}
		_, err = sqlite.UpgradeSchema(ctx, cfg)
		return err
	case "postgres":
		cfg, err := postgres.ParseConfigFromURL(storeURL)
		if err != nil {
			return err
		}
		_, err = postgresUpgradeSchema(ctx, cfg)
		return err
	case "mysql":
		cfg, err := mysql.ParseConfigFromURL(storeURL)
		if err != nil {
			return err
		}
		_, err = mysqlUpgradeSchema(ctx, cfg)
		return err
	default:
		return fmt.Errorf("unsupported demo Store backend: %s", backend)
	}
}

func writeDemoCase(outputRoot string) (string, error) {
	caseDir := filepath.Join(outputRoot, "cases")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		return "", fmt.Errorf("create demo case directory: %w", err)
	}
	casePath := filepath.Join(caseDir, "create-item.json")
	item := apicasespec.NewHTTPCase(
		"case.create-item",
		"Create Item",
		http.MethodPost,
		"/v1/items",
		map[string]string{"Content-Type": "application/json"},
		map[string]any{"id": "item-001", "name": "Example Item"},
		http.StatusCreated,
	)
	item.Assertions.ResponseContains = []string{"created"}
	if err := os.WriteFile(casePath, apicasespec.JSON(item), 0o644); err != nil {
		return "", fmt.Errorf("write demo case: %w", err)
	}
	return casePath, nil
}

func handleDemoAPIRequest(response http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		http.Error(response, `{"error":"read request body"}`, http.StatusBadRequest)
		return
	}
	if request.Method == http.MethodPost && request.URL.Path == "/v1/items" {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(response, `{"status":"created","received":%s}`, demoJSONBody(body))
		return
	}
	http.NotFound(response, request)
}

func demoJSONBody(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "null"
	}
	return trimmed
}

func printDemoReport(report demoCommandReport) {
	fmt.Println("AgentTestBench Demo")
	fmt.Printf("Case Run: %s\n", report.RunID)
	fmt.Printf("Case: %s\n", report.CaseID)
	fmt.Printf("Status: %s\n", report.Status)
	fmt.Printf("Evidence bundle: %s\n", report.EvidencePath)
	fmt.Printf("Store: %s\n", report.Store)
	fmt.Printf("Demo endpoint: %s\n", report.DemoEndpoint)
	fmt.Printf("Demo output root: %s\n", report.OutputRoot)
	if report.OutputRetained {
		fmt.Printf("Next: %s\n", report.NextInspectCommand)
	} else {
		fmt.Println("Demo output cleanup: enabled")
	}
}
