package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"agent-testbench/internal/store"
)

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

func runSandboxService(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing sandbox service command")
	}
	switch args[0] {
	case cliCommandList, "discover":
		return runSandboxServiceList(ctx, args[1:])
	default:
		return fmt.Errorf("unknown sandbox service command: %s", args[0])
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
