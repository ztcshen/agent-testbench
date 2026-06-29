package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"agent-testbench/internal/domain/plangraph"
	"agent-testbench/internal/store"
)

type mapValidationListReport struct {
	OK        bool                 `json:"ok"`
	MapID     string               `json:"mapId"`
	Interface string               `json:"interface,omitempty"`
	Anchor    string               `json:"anchor,omitempty"`
	Count     int                  `json:"count"`
	Groups    []mapValidationGroup `json:"groups"`
}

type mapValidationAttachReport struct {
	OK     bool               `json:"ok"`
	MapID  string             `json:"mapId"`
	Node   store.TestPlanNode `json:"node"`
	Counts struct {
		Validation int `json:"validation"`
	} `json:"counts"`
}

type mapValidationPromoteReport struct {
	OK     bool               `json:"ok"`
	MapID  string             `json:"mapId"`
	Node   store.TestPlanNode `json:"node"`
	Counts struct {
		Primary    int `json:"primary"`
		Validation int `json:"validation"`
	} `json:"counts"`
}

type mapValidationGroup struct {
	InterfaceNodeID string                     `json:"interfaceNodeId,omitempty"`
	AnchorNodeID    string                     `json:"anchorNodeId,omitempty"`
	AnchorCaseID    string                     `json:"anchorCaseId,omitempty"`
	Count           int                        `json:"count"`
	Families        []mapValidationFamily      `json:"families"`
	Cases           []mapValidationCaseSummary `json:"cases"`
}

type mapValidationFamily struct {
	Family string `json:"family"`
	Count  int    `json:"count"`
}

type mapValidationCaseSummary struct {
	NodeID       string `json:"nodeId"`
	CaseID       string `json:"caseId"`
	DisplayName  string `json:"displayName,omitempty"`
	InterfaceID  string `json:"interfaceNodeId,omitempty"`
	AnchorNodeID string `json:"anchorNodeId,omitempty"`
	BaseCaseID   string `json:"baseCaseId,omitempty"`
	Family       string `json:"family"`
	RenderMode   string `json:"renderMode,omitempty"`
}

const mapCommandValidation = "validation"

func runMapValidation(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing map validation command")
	}
	switch args[0] {
	case cliCommandList:
		return runMapValidationList(ctx, args[1:])
	case "attach":
		return runMapValidationAttach(ctx, args[1:])
	case "promote":
		return runMapValidationPromote(ctx, args[1:])
	default:
		return fmt.Errorf("unknown map validation command: %s", args[0])
	}
}

func runMapValidationList(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map validation list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	interfaceID := flags.String("interface", "", "Interface node id")
	anchor := flags.String("anchor", "", "Anchor node id or case id")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("map validation list does not accept positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	_, graph, cleanup, err := openRequiredMapGraphForCLI(ctx, *storeRef, *storeURL, *mapID)
	if err != nil {
		return err
	}
	defer cleanup()
	report := buildMapValidationListReport(graph, *interfaceID, *anchor)
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapValidationListReport(report)
	return nil
}

func runMapValidationAttach(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map validation attach", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	anchorRef := flags.String("anchor", "", "Anchor node id or case id")
	caseID := flags.String("case", "", "Validation case id")
	interfaceID := flags.String("interface", "", "Override interface node id")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("map validation attach does not accept positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	if strings.TrimSpace(*anchorRef) == "" {
		return errors.New("--anchor is required")
	}
	if strings.TrimSpace(*caseID) == "" {
		return errors.New("--case is required")
	}
	runtime, graph, cleanup, err := openRequiredMapGraphForCLI(ctx, *storeRef, *storeURL, *mapID)
	if err != nil {
		return err
	}
	defer cleanup()
	anchor, ok := findMapNodeByNodeOrCase(graph.Nodes, *anchorRef)
	if !ok {
		return fmt.Errorf("map validation anchor not found: %s", *anchorRef)
	}
	node, err := mapValidationNodeForAttach(ctx, runtime, graph, anchor, *caseID, *interfaceID)
	if err != nil {
		return err
	}
	graph = upsertMapNode(graph, node)
	if err := plangraph.ValidateDAG(graph); err != nil {
		return err
	}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		return err
	}
	report := mapValidationAttachReport{OK: true, MapID: graph.Map.ID, Node: node}
	report.Counts.Validation = countValidationNodes(graph.Nodes)
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapValidationAttachReport(report)
	return nil
}

func runMapValidationPromote(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map validation promote", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	caseID := flags.String("case", "", "Case id to promote to a primary map node")
	nodeID := flags.String("node", "", "Map node id to promote to a primary map node")
	interfaceID := flags.String("interface", "", "Override interface node id")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("map validation promote does not accept positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	target := firstNonEmpty(strings.TrimSpace(*nodeID), strings.TrimSpace(*caseID))
	if target == "" {
		return errors.New("--case or --node is required")
	}
	runtime, graph, cleanup, err := openRequiredMapGraphForCLI(ctx, *storeRef, *storeURL, *mapID)
	if err != nil {
		return err
	}
	defer cleanup()
	node, err := mapPrimaryNodeForPromote(ctx, runtime, graph, target, strings.TrimSpace(*caseID), *interfaceID)
	if err != nil {
		return err
	}
	graph = upsertMapNode(graph, node)
	if err := plangraph.ValidateDAG(graph); err != nil {
		return err
	}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		return err
	}
	report := mapValidationPromoteReport{OK: true, MapID: graph.Map.ID, Node: node}
	report.Counts.Primary = countPrimaryNodes(graph.Nodes)
	report.Counts.Validation = countValidationNodes(graph.Nodes)
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapValidationPromoteReport(report)
	return nil
}

func buildMapValidationListReport(graph store.TestPlanGraph, interfaceFilter string, anchorFilter string) mapValidationListReport {
	interfaceFilter = strings.TrimSpace(interfaceFilter)
	anchorFilter = strings.TrimSpace(anchorFilter)
	anchorNodeIDs := map[string]bool{}
	if anchorFilter != "" {
		if anchor, ok := findMapNodeByNodeOrCase(graph.Nodes, anchorFilter); ok {
			anchorNodeIDs[anchor.ID] = true
			if anchor.CaseID != "" {
				anchorNodeIDs[anchor.CaseID] = true
			}
		}
	}
	groupsByKey := map[string]*mapValidationGroup{}
	for _, node := range graph.Nodes {
		if !mapNodeIsValidation(node) {
			continue
		}
		if interfaceFilter != "" && node.InterfaceNodeID != interfaceFilter {
			continue
		}
		if anchorFilter != "" && !anchorNodeIDs[node.AnchorNodeID] && !anchorNodeIDs[node.BaseCaseID] && node.AnchorNodeID != anchorFilter && node.BaseCaseID != anchorFilter {
			continue
		}
		key := node.InterfaceNodeID + "\x00" + node.AnchorNodeID
		group := groupsByKey[key]
		if group == nil {
			group = &mapValidationGroup{InterfaceNodeID: node.InterfaceNodeID, AnchorNodeID: node.AnchorNodeID, AnchorCaseID: node.BaseCaseID}
			groupsByKey[key] = group
		}
		family := validationFamilyForNode(node)
		group.Cases = append(group.Cases, mapValidationCaseSummary{
			NodeID:       node.ID,
			CaseID:       node.CaseID,
			DisplayName:  mapNodeDisplayName(node),
			InterfaceID:  node.InterfaceNodeID,
			AnchorNodeID: node.AnchorNodeID,
			BaseCaseID:   node.BaseCaseID,
			Family:       family,
			RenderMode:   node.RenderMode,
		})
	}
	groups := make([]mapValidationGroup, 0, len(groupsByKey))
	total := 0
	for _, group := range groupsByKey {
		sort.SliceStable(group.Cases, func(i, j int) bool {
			return group.Cases[i].CaseID < group.Cases[j].CaseID
		})
		group.Count = len(group.Cases)
		group.Families = mapValidationFamilies(group.Cases)
		total += group.Count
		groups = append(groups, *group)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].InterfaceNodeID != groups[j].InterfaceNodeID {
			return groups[i].InterfaceNodeID < groups[j].InterfaceNodeID
		}
		return groups[i].AnchorNodeID < groups[j].AnchorNodeID
	})
	return mapValidationListReport{OK: true, MapID: graph.Map.ID, Interface: interfaceFilter, Anchor: anchorFilter, Count: total, Groups: groups}
}

func mapValidationNodeForAttach(ctx context.Context, runtime store.Store, graph store.TestPlanGraph, anchor store.TestPlanNode, caseID string, interfaceID string) (store.TestPlanNode, error) {
	caseID = strings.TrimSpace(caseID)
	if node, ok := findMapNodeByNodeOrCase(graph.Nodes, caseID); ok {
		node.MapID = graph.Map.ID
		node.Role = plangraph.NodeRoleValidation
		node.StateEffect = plangraph.StateEffectUnchanged
		node.AnchorNodeID = anchor.ID
		node.BaseCaseID = stringDefault(anchor.CaseID, anchor.ID)
		node.InterfaceNodeID = stringDefault(interfaceID, stringDefault(node.InterfaceNodeID, anchor.InterfaceNodeID))
		return node, nil
	}
	apiCase, ok := findCatalogAPICaseForMap(ctx, runtime, graph.Map.ProfileID, caseID)
	if !ok {
		return store.TestPlanNode{}, fmt.Errorf("validation case not found in active catalog: %s", caseID)
	}
	displayName := apiCase.DisplayName
	if displayName == "" {
		displayName = caseID
	}
	return store.TestPlanNode{
		MapID:                graph.Map.ID,
		ID:                   caseID,
		CaseID:               caseID,
		InterfaceNodeID:      stringDefault(interfaceID, stringDefault(apiCase.NodeID, anchor.InterfaceNodeID)),
		RequestTemplateID:    apiCase.RequestTemplateID,
		BaseCaseID:           stringDefault(anchor.CaseID, anchor.ID),
		AnchorNodeID:         anchor.ID,
		Role:                 plangraph.NodeRoleValidation,
		StateEffect:          plangraph.StateEffectUnchanged,
		RenderMode:           apiCase.RenderMode,
		PatchJSON:            normalizeMapJSON(apiCase.PatchJSON, ""),
		ExpectedJSON:         normalizeMapJSON(apiCase.ExpectedJSON, ""),
		RequiredPropertyJSON: mustCompactJSON(map[string]any{"caseId": caseID, "samePreconditionAsCase": stringDefault(anchor.CaseID, anchor.ID), "stateEffect": plangraph.StateEffectUnchanged}),
		ProvidedPropertyJSON: mustCompactJSON(map[string]any{"caseId": caseID, "stateEffect": plangraph.StateEffectUnchanged}),
		SummaryJSON:          mustCompactJSON(map[string]any{"displayName": displayName, "caseType": apiCase.CaseType, "scenario": apiCase.Scenario, "tags": apiCase.Tags}),
		SortOrder:            apiCase.SortOrder,
	}, nil
}

func mapPrimaryNodeForPromote(ctx context.Context, runtime store.Store, graph store.TestPlanGraph, target string, caseID string, interfaceID string) (store.TestPlanNode, error) {
	if node, ok := findMapNodeByNodeOrCase(graph.Nodes, target); ok {
		return promoteMapNodeToPrimary(node, nil, interfaceID), nil
	}
	caseID = strings.TrimSpace(firstNonEmpty(caseID, target))
	apiCase, ok := findCatalogAPICaseForMap(ctx, runtime, graph.Map.ProfileID, caseID)
	if !ok {
		return store.TestPlanNode{}, fmt.Errorf("case not found in map or active catalog: %s", target)
	}
	node := store.TestPlanNode{
		MapID:             graph.Map.ID,
		ID:                caseID,
		CaseID:            caseID,
		InterfaceNodeID:   stringDefault(interfaceID, apiCase.NodeID),
		RequestTemplateID: apiCase.RequestTemplateID,
		RenderMode:        apiCase.RenderMode,
		PatchJSON:         normalizeMapJSON(apiCase.PatchJSON, ""),
		ExpectedJSON:      normalizeMapJSON(apiCase.ExpectedJSON, ""),
		SummaryJSON:       mustCompactJSON(map[string]any{"displayName": stringDefault(apiCase.DisplayName, caseID), "caseType": apiCase.CaseType, "scenario": apiCase.Scenario, "tags": apiCase.Tags}),
		SortOrder:         apiCase.SortOrder,
	}
	return promoteMapNodeToPrimary(node, &apiCase, interfaceID), nil
}

func promoteMapNodeToPrimary(node store.TestPlanNode, apiCase *store.CatalogAPICase, interfaceID string) store.TestPlanNode {
	node.Role = plangraph.NodeRolePrimary
	node.StateEffect = plangraph.StateEffectAdvance
	node.BaseCaseID = ""
	node.AnchorNodeID = ""
	if strings.TrimSpace(node.CaseID) == "" {
		node.CaseID = node.ID
	}
	if strings.TrimSpace(interfaceID) != "" {
		node.InterfaceNodeID = strings.TrimSpace(interfaceID)
	} else if apiCase != nil && strings.TrimSpace(node.InterfaceNodeID) == "" {
		node.InterfaceNodeID = apiCase.NodeID
	}
	if apiCase != nil && strings.TrimSpace(node.RequestTemplateID) == "" {
		node.RequestTemplateID = apiCase.RequestTemplateID
	}
	node.RequiredPropertyJSON = mustCompactJSON(map[string]any{"caseId": node.CaseID})
	node.ProvidedPropertyJSON = mustCompactJSON(map[string]any{"caseId": node.CaseID, "stateEffect": plangraph.StateEffectAdvance})
	if strings.TrimSpace(node.SummaryJSON) == "" {
		node.SummaryJSON = "{}"
	}
	return node
}

func findCatalogAPICaseForMap(ctx context.Context, runtime store.Store, profileID string, caseID string) (store.CatalogAPICase, bool) {
	if catalog, err := runtime.GetProfileCatalogByID(ctx, profileID); err == nil {
		if item, ok := findCatalogAPICase(catalog.APICases, caseID); ok {
			return item, true
		}
	}
	if catalog, err := runtime.GetProfileCatalog(ctx); err == nil {
		return findCatalogAPICase(catalog.APICases, caseID)
	}
	return store.CatalogAPICase{}, false
}

func findMapNodeByNodeOrCase(nodes []store.TestPlanNode, ref string) (store.TestPlanNode, bool) {
	ref = strings.TrimSpace(ref)
	for _, node := range nodes {
		if node.ID == ref || node.CaseID == ref {
			return node, true
		}
	}
	return store.TestPlanNode{}, false
}

func upsertMapNode(graph store.TestPlanGraph, node store.TestPlanNode) store.TestPlanGraph {
	for index, item := range graph.Nodes {
		if item.ID == node.ID {
			graph.Nodes[index] = node
			return graph
		}
	}
	graph.Nodes = append(graph.Nodes, node)
	sort.SliceStable(graph.Nodes, func(i, j int) bool {
		if graph.Nodes[i].SortOrder != graph.Nodes[j].SortOrder {
			return graph.Nodes[i].SortOrder < graph.Nodes[j].SortOrder
		}
		return graph.Nodes[i].ID < graph.Nodes[j].ID
	})
	return graph
}

func countValidationNodes(nodes []store.TestPlanNode) int {
	count := 0
	for _, node := range nodes {
		if mapNodeIsValidation(node) {
			count++
		}
	}
	return count
}

func countPrimaryNodes(nodes []store.TestPlanNode) int {
	count := 0
	for _, node := range nodes {
		if !mapNodeIsValidation(node) {
			count++
		}
	}
	return count
}

func mapNodeIsValidation(node store.TestPlanNode) bool {
	return node.Role == plangraph.NodeRoleValidation || node.StateEffect == plangraph.StateEffectUnchanged
}

func mapNodeDisplayName(node store.TestPlanNode) string {
	return plangraph.NodeDisplayName(node)
}

func validationFamilyForNode(node store.TestPlanNode) string {
	return plangraph.ValidationFamilyForNode(node)
}

func mapValidationFamilies(cases []mapValidationCaseSummary) []mapValidationFamily {
	counts := map[string]int{}
	for _, item := range cases {
		counts[item.Family]++
	}
	families := make([]mapValidationFamily, 0, len(counts))
	for family, count := range counts {
		families = append(families, mapValidationFamily{Family: family, Count: count})
	}
	sort.SliceStable(families, func(i, j int) bool {
		return families[i].Family < families[j].Family
	})
	return families
}

func normalizeMapJSON(value string, emptyValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return emptyValue
	}
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return value
	}
	raw, err := json.Marshal(decoded)
	if err != nil {
		return value
	}
	return string(raw)
}

func printMapValidationListReport(report mapValidationListReport) {
	fmt.Println("Map Validation Cases")
	fmt.Printf("Map: %s\n", report.MapID)
	fmt.Printf("Cases: %d\n", report.Count)
	for _, group := range report.Groups {
		fmt.Printf("- interface=%s anchor=%s cases=%d\n", group.InterfaceNodeID, group.AnchorNodeID, group.Count)
	}
}

func printMapValidationAttachReport(report mapValidationAttachReport) {
	fmt.Println("Map Validation Attached")
	fmt.Printf("Map: %s\n", report.MapID)
	fmt.Printf("Case: %s\n", report.Node.CaseID)
	fmt.Printf("Anchor: %s\n", report.Node.AnchorNodeID)
}

func printMapValidationPromoteReport(report mapValidationPromoteReport) {
	fmt.Println("Map Case Promoted")
	fmt.Printf("Map: %s\n", report.MapID)
	fmt.Printf("Case: %s\n", report.Node.CaseID)
	fmt.Printf("Role: %s\n", report.Node.Role)
	fmt.Printf("State Effect: %s\n", report.Node.StateEffect)
}
