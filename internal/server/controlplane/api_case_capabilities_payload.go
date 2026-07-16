package controlplane

import (
	"time"

	"agent-testbench/internal/domain/casemaintenance"
	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profilecatalog"
	"agent-testbench/internal/store"
)

type apiCaseCapabilitiesPayload struct {
	OK              bool                `json:"ok"`
	ProfileID       string              `json:"profileId,omitempty"`
	CatalogRevision int64               `json:"catalogRevision,omitempty"`
	Cases           []apiCaseCapability `json:"cases"`
	Graph           map[string][]string `json:"graph,omitempty"`
}

type apiCaseCapability struct {
	ID                string              `json:"id"`
	Title             string              `json:"title,omitempty"`
	Description       string              `json:"description,omitempty"`
	NodeID            string              `json:"nodeId,omitempty"`
	CaseType          string              `json:"caseType,omitempty"`
	Scenario          string              `json:"scenario,omitempty"`
	Tags              []string            `json:"tags"`
	Priority          string              `json:"priority,omitempty"`
	Owner             string              `json:"owner,omitempty"`
	Status            string              `json:"status"`
	Required          bool                `json:"requiredForAdmission"`
	SortOrder         int                 `json:"sortOrder,omitempty"`
	Operation         string              `json:"operation,omitempty"`
	CasePath          string              `json:"casePath,omitempty"`
	RequestTemplateID string              `json:"requestTemplateId,omitempty"`
	SourceKind        string              `json:"sourceKind,omitempty"`
	SourcePath        string              `json:"sourcePath,omitempty"`
	ExecutorID        string              `json:"executorId,omitempty"`
	BaseURL           string              `json:"baseUrl,omitempty"`
	EvidenceDir       string              `json:"evidenceDir,omitempty"`
	TimeoutSeconds    int                 `json:"timeoutSeconds,omitempty"`
	DefaultOverrides  map[string]any      `json:"defaultOverrides,omitempty"`
	ExecutionReady    bool                `json:"executionReady"`
	Workflow          map[string]string   `json:"workflow,omitempty"`
	Graph             apiCaseServiceGraph `json:"graph"`
	RunCount          int                 `json:"runCount"`
	LatestRun         map[string]any      `json:"latestRun,omitempty"`
}

type apiCaseServiceGraph struct {
	Nodes []apiCaseServiceNode `json:"nodes"`
	Edges []catalogEdge        `json:"edges"`
}

type apiCaseServiceNode struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	Role        string `json:"role,omitempty"`
	Href        string `json:"href,omitempty"`
}

func apiCaseCapabilitiesFromBundle(bundle profile.Bundle) apiCaseCapabilitiesPayload {
	return apiCaseCapabilitiesFromCatalog(profilecatalog.FromBundle(bundle, time.Time{}))
}

func apiCaseCapabilitiesFromCatalog(catalog store.ProfileCatalog) apiCaseCapabilitiesPayload {
	nodeByID := make(map[string]store.CatalogInterfaceNode)
	for _, node := range catalog.InterfaceNodes {
		nodeByID[node.ID] = node
	}
	serviceByID := make(map[string]store.CatalogService)
	for _, service := range catalog.Services {
		serviceByID[service.ID] = service
	}
	cases := make([]apiCaseCapability, 0, len(catalog.APICases))
	for _, item := range catalog.APICases {
		node := nodeByID[item.NodeID]
		service := serviceByID[node.ServiceID]
		capability := newAPICaseCapability(item.ID, item.DisplayName, item.NodeID, node.DisplayName, node.ServiceID, service.DisplayName, service.Kind, item.CasePath, item.SourceKind, item.SourcePath, item.ExecutorID, item.BaseURL, item.EvidenceDir, item.TimeoutSeconds, jsonObject(item.DefaultOverridesJSON))
		capability.RequestTemplateID = item.RequestTemplateID
		capability.Description = item.Description
		capability.NodeID = item.NodeID
		capability.CaseType = item.CaseType
		capability.Scenario = item.Scenario
		capability.Tags = append([]string{}, item.Tags...)
		capability.Priority = item.Priority
		capability.Owner = item.Owner
		capability.Status = firstNonEmpty(item.Status, "active")
		capability.Required = item.RequiredForAdmission
		capability.SortOrder = item.SortOrder
		capability.ExecutionReady = casemaintenance.ExecutionReady(catalog, item)
		cases = append(cases, capability)
	}
	return apiCaseCapabilitiesPayload{
		OK:        true,
		ProfileID: catalog.ProfileID,
		Cases:     cases,
		Graph:     map[string][]string{},
	}
}

func newAPICaseCapability(id string, displayName string, nodeID string, nodeDisplayName string, serviceID string, serviceName string, serviceKind string, casePath string, sourceKind string, sourcePath string, executorID string, baseURL string, evidenceDir string, timeoutSeconds int, defaultOverrides map[string]any) apiCaseCapability {
	return apiCaseCapability{
		ID:               id,
		Title:            firstNonEmpty(displayName, id),
		Operation:        firstNonEmpty(nodeDisplayName, nodeID),
		CasePath:         casePath,
		SourceKind:       sourceKind,
		SourcePath:       sourcePath,
		ExecutorID:       executorID,
		BaseURL:          baseURL,
		EvidenceDir:      evidenceDir,
		TimeoutSeconds:   timeoutSeconds,
		DefaultOverrides: defaultOverrides,
		Workflow:         map[string]string{},
		Graph:            apiCaseGraphForService(serviceID, serviceName, serviceKind),
	}
}

func apiCaseGraphForService(serviceID string, serviceName string, serviceKind string) apiCaseServiceGraph {
	graph := apiCaseServiceGraph{Nodes: []apiCaseServiceNode{}, Edges: []catalogEdge{}}
	if serviceID == "" {
		return graph
	}
	graph.Nodes = append(graph.Nodes, apiCaseServiceNode{
		ID:          serviceID,
		DisplayName: firstNonEmpty(serviceName, serviceID),
		Role:        firstNonEmpty(serviceKind, "service"),
		Href:        "/environment-node.html?id=" + serviceID,
	})
	return graph
}
