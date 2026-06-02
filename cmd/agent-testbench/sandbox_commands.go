package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

type sandboxStartReport struct {
	OK         bool                        `json:"ok"`
	DryRun     bool                        `json:"dryRun,omitempty"`
	WorkflowID string                      `json:"workflowId,omitempty"`
	StorePath  string                      `json:"storePath"`
	Services   []sandboxStartServiceResult `json:"services"`
	Counts     sandboxStartReportCounts    `json:"counts"`
}

type sandboxStartReportCounts struct {
	Total   int `json:"total"`
	Started int `json:"started"`
	Planned int `json:"planned,omitempty"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

type sandboxStartServiceResult struct {
	ID             string `json:"id"`
	DisplayName    string `json:"displayName"`
	Kind           string `json:"kind"`
	ContainerName  string `json:"containerName,omitempty"`
	ServicePort    int    `json:"servicePort,omitempty"`
	ManagementPort int    `json:"managementPort,omitempty"`
	Command        string `json:"command,omitempty"`
	Skipped        bool   `json:"skipped"`
	Planned        bool   `json:"planned,omitempty"`
	SkipReason     string `json:"skipReason,omitempty"`
	ExitCode       int    `json:"exitCode"`
	Output         string `json:"output,omitempty"`
	Error          string `json:"error,omitempty"`
}

type sandboxStartFilters struct {
	ServiceID  string
	WorkflowID string
	Kind       string
}

type sandboxServiceListReport struct {
	OK            bool                     `json:"ok"`
	StorePath     string                   `json:"storePath"`
	EnvironmentID string                   `json:"environmentId,omitempty"`
	Count         int                      `json:"count"`
	Services      []sandboxServiceListItem `json:"services"`
}

type sandboxServiceListItem struct {
	ID                string   `json:"id"`
	DisplayName       string   `json:"displayName,omitempty"`
	Sources           []string `json:"sources,omitempty"`
	InProfileRegistry bool     `json:"inProfileRegistry"`
	InComponentGraph  bool     `json:"inComponentGraph,omitempty"`
	EnvironmentID     string   `json:"environmentId,omitempty"`
	ComponentID       string   `json:"componentId,omitempty"`
	Kind              string   `json:"kind,omitempty"`
	Role              string   `json:"role,omitempty"`
	ContainerName     string   `json:"containerName,omitempty"`
	Image             string   `json:"image,omitempty"`
	DockerService     string   `json:"dockerService,omitempty"`
	ComposeService    string   `json:"composeService,omitempty"`
	Required          bool     `json:"required,omitempty"`
	ServicePort       int      `json:"servicePort,omitempty"`
	ManagementPort    int      `json:"managementPort,omitempty"`
	StartupCommand    string   `json:"startupCommand,omitempty"`
	HasStartupCommand bool     `json:"hasStartupCommand"`
	HealthURL         string   `json:"healthUrl,omitempty"`
	Status            string   `json:"status,omitempty"`
}

func runSandbox(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing sandbox command")
	}
	switch args[0] {
	case "start":
		return runSandboxStart(ctx, args[1:])
	case "service":
		return runSandboxService(ctx, args[1:])
	case "interface":
		return runSandboxInterface(ctx, args[1:])
	default:
		return fmt.Errorf("unknown sandbox command: %s", args[0])
	}
}

func runSandboxService(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing sandbox service command")
	}
	switch args[0] {
	case cliCommandList, "discover":
		return runSandboxServiceList(ctx, args[1:])
	case "register":
		return runSandboxServiceRegister(ctx, args[1:])
	default:
		return fmt.Errorf("unknown sandbox service command: %s", args[0])
	}
}

func runSandboxInterface(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing sandbox interface command")
	}
	switch args[0] {
	case "register":
		return runSandboxInterfaceRegister(ctx, args[1:])
	default:
		return fmt.Errorf("unknown sandbox interface command: %s", args[0])
	}
}

func runSandboxServiceList(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("sandbox service list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	environmentID := flags.String("environment", "", "Environment id whose component graph should be shown beside the profile service registry")
	includeComponents := flags.Bool("include-components", false, "Include the selected environment component graph in the service list")
	serviceID := flags.String("service", "", "Only show one registered service")
	serviceKind := flags.String("kind", "", "Only show services of this kind")
	status := flags.String("status", "", "Only show services with this status")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *includeComponents && strings.TrimSpace(*environmentID) == "" {
		return errors.New("--include-components requires --environment ENV_ID")
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	runtime, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return err
	}
	defer closeCLIStore(runtime)
	catalog, err := runtime.GetProfileCatalog(ctx)
	if errors.Is(err, store.ErrNotFound) && strings.TrimSpace(*environmentID) != "" {
		catalog = store.ProfileCatalog{}
	} else if err != nil {
		return err
	}
	report := sandboxServiceListReport{
		OK:            true,
		StorePath:     maskStoreURL(resolvedStoreURL),
		EnvironmentID: strings.TrimSpace(*environmentID),
	}
	var graph store.EnvironmentComponentGraph
	includeGraph := strings.TrimSpace(*environmentID) != ""
	if includeGraph {
		if _, err := runtime.GetEnvironment(ctx, report.EnvironmentID); err != nil {
			return err
		}
		graph, err = runtime.GetEnvironmentComponentGraph(ctx, report.EnvironmentID)
		if err != nil {
			return err
		}
	}
	services := sandboxServiceListItems(catalog.Services, graph, report.EnvironmentID, includeGraph || *includeComponents)
	for _, service := range services {
		if !sandboxServiceListItemMatches(service, *serviceID, *serviceKind, *status) {
			continue
		}
		report.Services = append(report.Services, service)
	}
	report.Count = len(report.Services)
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printSandboxServiceListReport(report)
	return nil
}

func runSandboxServiceRegister(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("sandbox service register", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	id := flags.String("id", "", "Service id")
	displayName := flags.String("display-name", "", "Service display name")
	kind := flags.String("kind", "", "Service kind")
	servicePort := flags.Int("service-port", 0, "Service port")
	managementPort := flags.Int("management-port", 0, "Management port")
	startupCommand := flags.String("startup-command", "", "Startup command")
	healthURL := flags.String("health-url", "", "Health URL")
	status := flags.String("status", "", "Service status")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	runtime, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return err
	}
	defer closeCLIStore(runtime)
	response, err := controlplane.RegisterSandboxService(ctx, runtime, controlplane.SandboxServiceRegistrationRequest{
		ID:             *id,
		DisplayName:    *displayName,
		Kind:           *kind,
		ServicePort:    *servicePort,
		ManagementPort: *managementPort,
		StartupCommand: *startupCommand,
		HealthURL:      *healthURL,
		Status:         *status,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(response)
	}
	fmt.Printf("Registered service: %s\n", response.Service.ID)
	fmt.Printf("Store: %s\n", response.StoreID)
	fmt.Printf("Kind: %s\n", response.Service.Kind)
	if response.Service.ServicePort > 0 {
		fmt.Printf("Port: %d\n", response.Service.ServicePort)
	}
	return nil
}

func sandboxServiceListItems(services []store.CatalogService, graph store.EnvironmentComponentGraph, environmentID string, includeGraph bool) []sandboxServiceListItem {
	items := make([]sandboxServiceListItem, 0, len(services)+len(graph.Components))
	positions := map[string]int{}
	componentByID := map[string]store.EnvironmentComponent{}
	if includeGraph {
		for _, component := range graph.Components {
			componentByID[component.ComponentID] = component
		}
	}
	for _, service := range services {
		item := sandboxServiceListItem{
			ID:                service.ID,
			DisplayName:       service.DisplayName,
			Sources:           []string{"profile-service-registry"},
			InProfileRegistry: true,
			Kind:              service.Kind,
			ContainerName:     service.ContainerName,
			Image:             service.Image,
			DockerService:     service.DockerService,
			ServicePort:       service.ServicePort,
			ManagementPort:    service.ManagementPort,
			StartupCommand:    strings.TrimSpace(service.StartupCommand),
			HasStartupCommand: strings.TrimSpace(service.StartupCommand) != "",
			HealthURL:         service.HealthURL,
			Status:            service.Status,
		}
		if component, ok := componentByID[service.ID]; ok {
			item = sandboxServiceListItemWithComponent(item, component, environmentID)
		}
		positions[item.ID] = len(items)
		items = append(items, item)
	}
	if includeGraph {
		for _, component := range graph.Components {
			if _, ok := positions[component.ComponentID]; ok {
				continue
			}
			item := sandboxServiceListItemWithComponent(sandboxServiceListItem{
				ID:                component.ComponentID,
				DisplayName:       component.DisplayName,
				Sources:           []string{},
				InProfileRegistry: false,
			}, component, environmentID)
			items = append(items, item)
		}
	}
	return items
}

func sandboxServiceListItemWithComponent(item sandboxServiceListItem, component store.EnvironmentComponent, environmentID string) sandboxServiceListItem {
	item.Sources = appendMissingString(item.Sources, "environment-component-graph")
	item.InComponentGraph = true
	item.EnvironmentID = environmentID
	item.ComponentID = component.ComponentID
	item.Role = component.Role
	item.ComposeService = component.ComposeService
	item.Required = component.Required
	if strings.TrimSpace(item.DisplayName) == "" {
		item.DisplayName = component.DisplayName
	}
	if strings.TrimSpace(item.Kind) == "" {
		item.Kind = component.Kind
	}
	if strings.TrimSpace(item.Image) == "" {
		item.Image = component.Image
	}
	return item
}

func sandboxServiceListItemMatches(item sandboxServiceListItem, serviceID string, kind string, status string) bool {
	serviceID = strings.TrimSpace(serviceID)
	if serviceID != "" && item.ID != serviceID && item.ComponentID != serviceID {
		return false
	}
	kind = strings.TrimSpace(kind)
	if kind != "" && strings.TrimSpace(item.Kind) != kind {
		return false
	}
	status = strings.TrimSpace(status)
	return status == "" || strings.TrimSpace(item.Status) == status
}

func appendMissingString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, item := range values {
		if item == value {
			return values
		}
	}
	return append(values, value)
}

func runSandboxInterfaceRegister(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("sandbox interface register", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	id := flags.String("id", "", "Interface id")
	displayName := flags.String("display-name", "", "Interface display name")
	serviceID := flags.String("service-id", "", "Entry service id")
	operation := flags.String("operation", "", "Operation name")
	method := flags.String("method", "", "HTTP method")
	path := flags.String("path", "", "HTTP path")
	templateID := flags.String("template-id", "", "Request template id")
	caseID := flags.String("case-id", "", "API case id")
	caseTitle := flags.String("case-title", "", "API case title")
	requiredForAdmission := flags.Bool("required-for-admission", false, "Require this case for interface admission")
	timeoutMs := flags.Int("timeout-ms", 0, "Interface timeout in milliseconds")
	timeoutSeconds := flags.Int("timeout-seconds", 0, "Case timeout in seconds")
	status := flags.String("status", "", "Interface status")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	runtime, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return err
	}
	defer closeCLIStore(runtime)
	response, err := controlplane.RegisterSandboxInterface(ctx, runtime, controlplane.SandboxInterfaceRegistrationRequest{
		ID:          *id,
		DisplayName: *displayName,
		ServiceID:   *serviceID,
		Operation:   *operation,
		Method:      *method,
		Path:        *path,
		TemplateID:  *templateID,
		TimeoutMs:   *timeoutMs,
		Status:      *status,
		Case: controlplane.SandboxInterfaceCase{
			ID:                   *caseID,
			DisplayName:          *caseTitle,
			RequiredForAdmission: *requiredForAdmission,
			TimeoutSeconds:       *timeoutSeconds,
		},
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(response)
	}
	fmt.Printf("Registered interface: %s\n", response.Interface.ID)
	fmt.Printf("Store: %s\n", response.StoreID)
	fmt.Printf("Service: %s\n", response.Interface.ServiceID)
	fmt.Printf("Case: %s\n", response.Interface.CaseID)
	return nil
}

func runSandboxStart(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("sandbox start", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	serviceID := flags.String("service", "", "Only start one registered service")
	workflowID := flags.String("workflow", "", "Start only services required by a workflow")
	serviceKind := flags.String("kind", "", "Only start services of this kind; default includes all kinds")
	timeoutSeconds := flags.Int("timeout-seconds", 300, "Per-service startup command timeout")
	dryRun := flags.Bool("dry-run", false, "Plan service startup without running startup commands")
	outputFormat := flags.String("output-format", "", "Output format: text, json, or stream-json")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	resolvedOutputFormat, err := resolveCLIOutputFormat(*outputFormat, *jsonOutput)
	if err != nil {
		return err
	}
	if *timeoutSeconds <= 0 {
		return errors.New("--timeout-seconds must be greater than 0")
	}
	if strings.TrimSpace(*workflowID) != "" && strings.TrimSpace(*serviceID) != "" {
		return errors.New("--workflow cannot be combined with --service")
	}
	if strings.TrimSpace(*workflowID) != "" && strings.TrimSpace(*serviceKind) != "" {
		return errors.New("--workflow cannot be combined with --kind")
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	runtime, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return err
	}
	defer closeCLIStore(runtime)
	if resolvedOutputFormat == "stream-json" {
		ctx = contextWithAgentEventStream(ctx, os.Stdout)
	}
	catalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil {
		return err
	}
	report := sandboxStartReport{
		OK:         true,
		DryRun:     *dryRun,
		WorkflowID: strings.TrimSpace(*workflowID),
		StorePath:  maskStoreURL(resolvedStoreURL),
	}
	agentEmitRunStarted(ctx, newSandboxStartRunID(), "sandbox.start", sandboxStartTarget(report), "sandbox start started")
	workflowRequired, err := sandboxWorkflowRequiredServiceReasons(catalog, report.WorkflowID)
	if err != nil {
		return err
	}
	filters := sandboxStartFilters{
		ServiceID:  strings.TrimSpace(*serviceID),
		WorkflowID: report.WorkflowID,
		Kind:       strings.TrimSpace(*serviceKind),
	}
	startSandboxServices(ctx, &report, catalog.Services, workflowRequired, filters, time.Duration(*timeoutSeconds)*time.Second, *dryRun)
	if err := validateSandboxStartSelection(report, filters); err != nil {
		return err
	}
	if resolvedOutputFormat == "stream-json" {
		agentEmitRunCompleted(ctx, "sandbox.start", sandboxStartStatus(report), sandboxStartTarget(report), "sandbox start completed", sandboxStartError(report), report)
	} else if resolvedOutputFormat == "json" {
		if err := writeIndentedJSON(report); err != nil {
			return err
		}
	} else {
		printSandboxStartReport(report)
	}
	if !report.OK {
		return errors.New("one or more sandbox services failed to start")
	}
	return nil
}

func startSandboxServices(ctx context.Context, report *sandboxStartReport, services []store.CatalogService, workflowRequired map[string]string, filters sandboxStartFilters, timeout time.Duration, dryRun bool) {
	for _, service := range services {
		if !sandboxStartServiceMatches(service, workflowRequired[service.ID], filters) {
			continue
		}
		requiredReason := sandboxRequiredStartupReason(service.ID, filters.ServiceID, workflowRequired[service.ID])
		agentEmitStep(ctx, "step_started", "sandbox.service", "running", service.ID, "service startup started", "")
		result := runSandboxServiceStartup(ctx, service, timeout, dryRun, requiredReason)
		agentEmitStep(ctx, "step_completed", "sandbox.service", sandboxStartServiceStatus(result), service.ID, sandboxStartServiceMessage(result), result.Error)
		addSandboxStartResult(report, result)
	}
}

func sandboxStartServiceMatches(service store.CatalogService, workflowReason string, filters sandboxStartFilters) bool {
	if filters.ServiceID != "" && service.ID != filters.ServiceID {
		return false
	}
	if filters.WorkflowID != "" && workflowReason == "" {
		return false
	}
	return filters.Kind == "" || strings.TrimSpace(service.Kind) == filters.Kind
}

func addSandboxStartResult(report *sandboxStartReport, result sandboxStartServiceResult) {
	report.Services = append(report.Services, result)
	report.Counts.Total++
	switch {
	case result.Planned:
		report.Counts.Planned++
	case result.Skipped:
		report.Counts.Skipped++
	case result.ExitCode == 0:
		report.Counts.Started++
	default:
		report.Counts.Failed++
		report.OK = false
	}
}

func validateSandboxStartSelection(report sandboxStartReport, filters sandboxStartFilters) error {
	if filters.ServiceID != "" && report.Counts.Total == 0 {
		return fmt.Errorf("registered service not found in profile service registry: %s (sandbox start does not read the environment component graph; use environment restore for component-graph Docker startup or register the service with sandbox service register)", filters.ServiceID)
	}
	if filters.WorkflowID != "" && report.Counts.Total == 0 {
		return fmt.Errorf("workflow has no registered startable services: %s", filters.WorkflowID)
	}
	return nil
}

func newSandboxStartRunID() string {
	return "sandbox.start." + time.Now().UTC().Format("20060102T150405.000000000Z")
}

func sandboxStartTarget(report sandboxStartReport) string {
	if strings.TrimSpace(report.WorkflowID) != "" {
		return report.WorkflowID
	}
	return "profile-service-registry"
}

func sandboxStartStatus(report sandboxStartReport) string {
	if report.OK {
		return "passed"
	}
	return "failed"
}

func sandboxStartError(report sandboxStartReport) string {
	if report.OK {
		return ""
	}
	return "one or more sandbox services failed to start"
}

func sandboxStartServiceStatus(result sandboxStartServiceResult) string {
	switch {
	case result.Planned:
		return "planned"
	case result.Skipped:
		return "skipped"
	case result.ExitCode == 0:
		return "passed"
	default:
		return "failed"
	}
}

func sandboxStartServiceMessage(result sandboxStartServiceResult) string {
	switch sandboxStartServiceStatus(result) {
	case "planned":
		return "service startup planned"
	case "skipped":
		return result.SkipReason
	case "failed":
		return "service startup failed"
	default:
		return "service startup completed"
	}
}

func printSandboxStartReport(report sandboxStartReport) {
	fmt.Println("Sandbox Start")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Store: %s\n", report.StorePath)
	if report.DryRun {
		fmt.Println("Mode: dry-run")
		fmt.Printf("Total: %d Planned: %d Skipped: %d Failed: %d\n", report.Counts.Total, report.Counts.Planned, report.Counts.Skipped, report.Counts.Failed)
	} else {
		fmt.Printf("Total: %d Started: %d Skipped: %d Failed: %d\n", report.Counts.Total, report.Counts.Started, report.Counts.Skipped, report.Counts.Failed)
	}
	for _, service := range report.Services {
		state := "started"
		if service.Planned {
			state = "planned"
		}
		if service.Skipped {
			state = "skipped"
		}
		if service.ExitCode != 0 {
			state = "failed"
		}
		fmt.Printf("- %s [%s]\n", service.ID, state)
		if service.Command != "" {
			fmt.Printf("  command: %s\n", service.Command)
		}
		if service.SkipReason != "" {
			fmt.Printf("  reason: %s\n", service.SkipReason)
		}
		if service.Error != "" {
			fmt.Printf("  error: %s\n", service.Error)
		}
		if service.Output != "" {
			fmt.Printf("  output: %s\n", service.Output)
		}
	}
}

func printSandboxServiceListReport(report sandboxServiceListReport) {
	fmt.Println("Sandbox Services")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Store: %s\n", report.StorePath)
	fmt.Printf("Count: %d\n", report.Count)
	for _, service := range report.Services {
		label := service.ID
		if service.DisplayName != "" {
			label = fmt.Sprintf("%s (%s)", service.ID, service.DisplayName)
		}
		fmt.Printf("- %s\n", label)
		if len(service.Sources) > 0 {
			fmt.Printf("  sources: %s\n", strings.Join(service.Sources, ", "))
		}
		if service.Kind != "" {
			fmt.Printf("  kind: %s\n", service.Kind)
		}
		if service.ComposeService != "" {
			fmt.Printf("  compose: %s\n", service.ComposeService)
		}
		if service.Status != "" {
			fmt.Printf("  status: %s\n", service.Status)
		}
		if service.StartupCommand != "" {
			fmt.Printf("  startup: %s\n", service.StartupCommand)
		}
		if service.HealthURL != "" {
			fmt.Printf("  health: %s\n", service.HealthURL)
		}
	}
}
