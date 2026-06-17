package plangraph

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"agent-testbench/internal/domain/catalog"
)

type ImportOptions struct {
	MapID       string
	DisplayName string
	Description string
	Now         time.Time
}

func ImportCatalog(snapshot catalog.ProfileCatalog, options ImportOptions) (Graph, error) {
	profileID := strings.TrimSpace(snapshot.ProfileID)
	if profileID == "" {
		return Graph{}, errors.New("profile id is required")
	}
	now := options.Now
	if now.IsZero() {
		now = snapshot.IndexedAt
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	mapID := strings.TrimSpace(options.MapID)
	if mapID == "" {
		mapID = "map." + profileID
	}
	builder := newGraphBuilder(snapshot, mapID, now)
	builder.graph.Map = Map{
		ID:          mapID,
		ProfileID:   profileID,
		DisplayName: stringDefault(options.DisplayName, profileID+" workflow map"),
		Description: strings.TrimSpace(options.Description),
		Status:      "active",
		SummaryJSON: "{}",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := builder.importPaths(); err != nil {
		return Graph{}, err
	}
	builder.importValidationCases()
	builder.importMaterializations()
	builder.finalize()
	if err := ValidateDAG(builder.graph); err != nil {
		return Graph{}, err
	}
	return builder.graph, nil
}

type graphBuilder struct {
	catalog        catalog.ProfileCatalog
	mapID          string
	now            time.Time
	graph          Graph
	caseByID       map[string]catalog.APICase
	workflowByID   map[string]catalog.Workflow
	bindingsByPath map[string][]catalog.WorkflowBinding
	boundCaseIDs   map[string]bool
	nodeByID       map[string]Node
	pathByID       map[string]Path
	edgeByID       map[string]Edge
	matByID        map[string]Materialization
}

func newGraphBuilder(snapshot catalog.ProfileCatalog, mapID string, now time.Time) *graphBuilder {
	b := &graphBuilder{
		catalog:        snapshot,
		mapID:          mapID,
		now:            now,
		caseByID:       map[string]catalog.APICase{},
		workflowByID:   map[string]catalog.Workflow{},
		bindingsByPath: map[string][]catalog.WorkflowBinding{},
		boundCaseIDs:   map[string]bool{},
		nodeByID:       map[string]Node{},
		pathByID:       map[string]Path{},
		edgeByID:       map[string]Edge{},
		matByID:        map[string]Materialization{},
	}
	for _, item := range snapshot.APICases {
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" || !isActiveStatus(item.Status) {
			continue
		}
		b.caseByID[item.ID] = item
	}
	for _, item := range snapshot.Workflows {
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" {
			continue
		}
		b.workflowByID[item.ID] = item
	}
	for _, item := range snapshot.WorkflowBindings {
		item.WorkflowID = strings.TrimSpace(item.WorkflowID)
		item.CaseID = strings.TrimSpace(item.CaseID)
		if item.WorkflowID == "" || item.CaseID == "" {
			continue
		}
		b.bindingsByPath[item.WorkflowID] = append(b.bindingsByPath[item.WorkflowID], item)
		b.boundCaseIDs[item.CaseID] = true
	}
	return b
}

func (b *graphBuilder) importPaths() error {
	pathIDs := make([]string, 0, len(b.bindingsByPath))
	for id := range b.bindingsByPath {
		pathIDs = append(pathIDs, id)
	}
	sort.Strings(pathIDs)
	for index, pathID := range pathIDs {
		bindings := b.bindingsByPath[pathID]
		sortWorkflowBindings(bindings)
		workflow := b.workflowByID[pathID]
		b.pathByID[pathID] = Path{
			MapID:                b.mapID,
			ID:                   pathID,
			WorkflowID:           pathID,
			DisplayName:          stringDefault(workflow.DisplayName, pathID),
			Status:               "active",
			RequiredPropertyJSON: jsonText(map[string]any{"pathId": pathID}),
			ProvidedPropertyJSON: jsonText(map[string]any{"pathId": pathID, "stateEffect": StateEffectAdvance}),
			SummaryJSON:          jsonText(map[string]any{"stepCount": 0}),
			SortOrder:            index + 1,
		}
		var previousNodeID string
		pathStepIndex := 0
		for _, binding := range bindings {
			apiCase, ok := b.caseByID[binding.CaseID]
			if !ok {
				if binding.Required {
					return fmt.Errorf("required workflow binding %s/%s references missing or inactive case %s", pathID, strings.TrimSpace(binding.StepID), binding.CaseID)
				}
				continue
			}
			node := b.upsertCaseNode(apiCase, true)
			pathStepIndex++
			b.graph.PathSteps = append(b.graph.PathSteps, PathStep{
				MapID:       b.mapID,
				PathID:      pathID,
				StepIndex:   pathStepIndex,
				StepID:      binding.StepID,
				NodeID:      node.ID,
				CaseID:      node.CaseID,
				Required:    binding.Required,
				SummaryJSON: "{}",
			})
			if previousNodeID != "" {
				edgeID := controlEdgeID(pathID, pathStepIndex-1, previousNodeID, node.ID)
				b.edgeByID[edgeID] = Edge{
					MapID:        b.mapID,
					ID:           edgeID,
					FromNodeID:   previousNodeID,
					ToNodeID:     node.ID,
					Kind:         EdgeKindControl,
					PathID:       pathID,
					Required:     true,
					MappingsJSON: "[]",
					SummaryJSON:  "{}",
					SortOrder:    len(b.edgeByID) + 1,
				}
			}
			previousNodeID = node.ID
		}
		path := b.pathByID[pathID]
		path.SummaryJSON = jsonText(map[string]any{"stepCount": pathStepIndex})
		b.pathByID[pathID] = path
	}
	return nil
}

func (b *graphBuilder) importValidationCases() {
	caseIDs := sortedCaseIDs(b.caseByID)
	for _, caseID := range caseIDs {
		apiCase := b.caseByID[caseID]
		if b.boundCaseIDs[caseID] || !isValidationCase(apiCase) {
			continue
		}
		b.upsertCaseNode(apiCase, false)
	}
}

func (b *graphBuilder) importMaterializations() {
	fixtures := append([]catalog.Fixture(nil), b.catalog.Fixtures...)
	sort.SliceStable(fixtures, func(i, j int) bool {
		if fixtures[i].SortOrder != fixtures[j].SortOrder {
			return fixtures[i].SortOrder < fixtures[j].SortOrder
		}
		return fixtures[i].ID < fixtures[j].ID
	})
	for _, fixture := range fixtures {
		if !isActiveStatus(fixture.Status) || !strings.EqualFold(strings.TrimSpace(fixture.Kind), "workflow_prefix") {
			continue
		}
		sourcePathID := strings.TrimSpace(fixture.SourceWorkflowID)
		untilStep := strings.TrimSpace(fixture.SourceUntilStep)
		untilNodeID := b.nodeIDForWorkflowStep(sourcePathID, untilStep)
		if sourcePathID == "" || untilStep == "" || untilNodeID == "" {
			continue
		}
		materializationID := stringDefault(fixture.ID, sourcePathID+"."+untilStep)
		b.matByID[materializationID] = Materialization{
			MapID:             b.mapID,
			ID:                materializationID,
			FixtureID:         fixture.ID,
			SourcePathID:      sourcePathID,
			SourceWorkflowID:  sourcePathID,
			SourceUntilStep:   untilStep,
			SourceUntilNodeID: untilNodeID,
			SnapshotKind:      "workflow_prefix",
			TTLSeconds:        fixture.TTLSeconds,
			Status:            stringDefault(fixture.Status, "active"),
			SummaryJSON:       jsonText(map[string]any{"displayName": fixture.DisplayName}),
			SortOrder:         fixture.SortOrder,
		}
	}
	dependencies := append([]catalog.CaseDependency(nil), b.catalog.CaseDependencies...)
	sort.SliceStable(dependencies, func(i, j int) bool {
		if dependencies[i].SortOrder != dependencies[j].SortOrder {
			return dependencies[i].SortOrder < dependencies[j].SortOrder
		}
		return dependencies[i].ID < dependencies[j].ID
	})
	for _, dependency := range dependencies {
		if !isActiveStatus(dependency.Status) {
			continue
		}
		targetCaseID := strings.TrimSpace(dependency.CaseID)
		target, ok := b.nodeByID[targetCaseID]
		if !ok {
			apiCase, found := b.caseByID[targetCaseID]
			if !found {
				continue
			}
			target = b.upsertCaseNode(apiCase, false)
		}
		materializationID := strings.TrimSpace(dependency.FixtureID)
		materialization, ok := b.matByID[materializationID]
		if !ok {
			continue
		}
		edgeID := strings.TrimSpace(dependency.ID)
		if edgeID == "" {
			edgeID = fixtureEdgeID(materializationID, target.ID)
		}
		b.edgeByID[edgeID] = Edge{
			MapID:             b.mapID,
			ID:                edgeID,
			FromNodeID:        materialization.SourceUntilNodeID,
			ToNodeID:          target.ID,
			Kind:              EdgeKindFixture,
			MaterializationID: materialization.ID,
			Required:          dependency.Required,
			MappingsJSON:      jsonDefault(dependency.MappingsJSON, "[]"),
			SummaryJSON:       "{}",
			SortOrder:         len(b.edgeByID) + 1,
		}
	}
}

func (b *graphBuilder) upsertCaseNode(apiCase catalog.APICase, inPath bool) Node {
	nodeID := strings.TrimSpace(apiCase.ID)
	if nodeID == "" {
		return Node{}
	}
	if existing, ok := b.nodeByID[nodeID]; ok {
		return existing
	}
	role := NodeRolePrimary
	stateEffect := StateEffectAdvance
	baseCaseID := ""
	anchorNodeID := ""
	if !inPath || isValidationCase(apiCase) {
		role = NodeRoleValidation
		stateEffect = StateEffectUnchanged
		baseCaseID = b.baseCaseFor(apiCase)
		anchorNodeID = baseCaseID
	}
	required := map[string]any{"caseId": nodeID}
	if role == NodeRoleValidation {
		required["samePreconditionAsCase"] = baseCaseID
		required["stateEffect"] = StateEffectUnchanged
	}
	provided := map[string]any{"caseId": nodeID, "stateEffect": stateEffect}
	node := Node{
		MapID:                b.mapID,
		ID:                   nodeID,
		CaseID:               nodeID,
		InterfaceNodeID:      strings.TrimSpace(apiCase.NodeID),
		RequestTemplateID:    strings.TrimSpace(apiCase.RequestTemplateID),
		BaseCaseID:           baseCaseID,
		AnchorNodeID:         anchorNodeID,
		Role:                 role,
		StateEffect:          stateEffect,
		RenderMode:           strings.TrimSpace(apiCase.RenderMode),
		PatchJSON:            jsonDefault(apiCase.PatchJSON, ""),
		ExpectedJSON:         jsonDefault(apiCase.ExpectedJSON, ""),
		RequiredPropertyJSON: jsonText(required),
		ProvidedPropertyJSON: jsonText(provided),
		SummaryJSON: jsonText(map[string]any{
			"displayName": apiCase.DisplayName,
			"caseType":    apiCase.CaseType,
			"scenario":    apiCase.Scenario,
			"tags":        apiCase.Tags,
		}),
		SortOrder: apiCase.SortOrder,
	}
	b.nodeByID[node.ID] = node
	return node
}

func (b *graphBuilder) baseCaseFor(apiCase catalog.APICase) string {
	nodeID := strings.TrimSpace(apiCase.NodeID)
	if nodeID == "" {
		return ""
	}
	candidates := make([]catalog.APICase, 0)
	for _, candidate := range b.caseByID {
		if candidate.ID == apiCase.ID || strings.TrimSpace(candidate.NodeID) != nodeID {
			continue
		}
		if isValidationCase(candidate) || isDiffCase(candidate) {
			continue
		}
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		leftBound := b.boundCaseIDs[candidates[i].ID]
		rightBound := b.boundCaseIDs[candidates[j].ID]
		if leftBound != rightBound {
			return leftBound
		}
		if candidates[i].SortOrder != candidates[j].SortOrder {
			return candidates[i].SortOrder < candidates[j].SortOrder
		}
		return candidates[i].ID < candidates[j].ID
	})
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].ID
}

func (b *graphBuilder) nodeIDForWorkflowStep(workflowID string, stepID string) string {
	bindings := append([]catalog.WorkflowBinding(nil), b.bindingsByPath[workflowID]...)
	sortWorkflowBindings(bindings)
	for _, binding := range bindings {
		if strings.TrimSpace(binding.StepID) == stepID {
			return strings.TrimSpace(binding.CaseID)
		}
	}
	return ""
}

func (b *graphBuilder) finalize() {
	for _, item := range b.nodeByID {
		b.graph.Nodes = append(b.graph.Nodes, item)
	}
	sort.SliceStable(b.graph.Nodes, func(i, j int) bool {
		if b.graph.Nodes[i].SortOrder != b.graph.Nodes[j].SortOrder {
			return b.graph.Nodes[i].SortOrder < b.graph.Nodes[j].SortOrder
		}
		return b.graph.Nodes[i].ID < b.graph.Nodes[j].ID
	})
	for _, item := range b.edgeByID {
		b.graph.Edges = append(b.graph.Edges, item)
	}
	sort.SliceStable(b.graph.Edges, func(i, j int) bool {
		if b.graph.Edges[i].SortOrder != b.graph.Edges[j].SortOrder {
			return b.graph.Edges[i].SortOrder < b.graph.Edges[j].SortOrder
		}
		return b.graph.Edges[i].ID < b.graph.Edges[j].ID
	})
	for _, item := range b.pathByID {
		b.graph.Paths = append(b.graph.Paths, item)
	}
	sort.SliceStable(b.graph.Paths, func(i, j int) bool {
		if b.graph.Paths[i].SortOrder != b.graph.Paths[j].SortOrder {
			return b.graph.Paths[i].SortOrder < b.graph.Paths[j].SortOrder
		}
		return b.graph.Paths[i].ID < b.graph.Paths[j].ID
	})
	sort.SliceStable(b.graph.PathSteps, func(i, j int) bool {
		if b.graph.PathSteps[i].PathID != b.graph.PathSteps[j].PathID {
			return b.graph.PathSteps[i].PathID < b.graph.PathSteps[j].PathID
		}
		return b.graph.PathSteps[i].StepIndex < b.graph.PathSteps[j].StepIndex
	})
	for _, item := range b.matByID {
		b.graph.Materializations = append(b.graph.Materializations, item)
	}
	sort.SliceStable(b.graph.Materializations, func(i, j int) bool {
		if b.graph.Materializations[i].SortOrder != b.graph.Materializations[j].SortOrder {
			return b.graph.Materializations[i].SortOrder < b.graph.Materializations[j].SortOrder
		}
		return b.graph.Materializations[i].ID < b.graph.Materializations[j].ID
	})
	b.graph.Map.SummaryJSON = jsonText(map[string]any{
		"nodes":            len(b.graph.Nodes),
		"edges":            len(b.graph.Edges),
		"paths":            len(b.graph.Paths),
		"pathSteps":        len(b.graph.PathSteps),
		"materializations": len(b.graph.Materializations),
	})
}

func sortWorkflowBindings(bindings []catalog.WorkflowBinding) {
	sort.SliceStable(bindings, func(i, j int) bool {
		if bindings[i].SortOrder != bindings[j].SortOrder {
			return bindings[i].SortOrder < bindings[j].SortOrder
		}
		return bindings[i].StepID < bindings[j].StepID
	})
}

func controlEdgeID(pathID string, stepIndex int, fromNodeID string, toNodeID string) string {
	raw := fmt.Sprintf("%s.%03d.%s.%s", pathID, stepIndex, fromNodeID, toNodeID)
	if len(raw) <= 128 {
		return raw
	}
	hash := shortIDHash(pathID, fmt.Sprintf("%03d", stepIndex), fromNodeID, toNodeID)
	prefix := truncateIDPart(pathID, 80)
	if prefix == "" {
		return fmt.Sprintf("edge.%03d.%s", stepIndex, hash)
	}
	return fmt.Sprintf("edge.%s.%03d.%s", prefix, stepIndex, hash)
}

func fixtureEdgeID(materializationID string, targetNodeID string) string {
	raw := "edge." + materializationID + "." + targetNodeID
	if len(raw) <= 128 {
		return raw
	}
	hash := shortIDHash("fixture", materializationID, targetNodeID)
	prefix := truncateIDPart(materializationID, 80)
	if prefix == "" {
		return "edge.fixture." + hash
	}
	return "edge." + prefix + "." + hash
}

func shortIDHash(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return fmt.Sprintf("%x", hash.Sum(nil))[:16]
}

func truncateIDPart(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return strings.TrimRight(value[:max], ".-_")
}

func sortedCaseIDs(items map[string]catalog.APICase) []string {
	out := make([]string, 0, len(items))
	for id := range items {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func isActiveStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "active", "verified", "published":
		return true
	default:
		return false
	}
}

func isValidationCase(apiCase catalog.APICase) bool {
	caseType := strings.ToLower(strings.TrimSpace(apiCase.CaseType + " " + apiCase.Scenario))
	if strings.Contains(caseType, "negative") || strings.Contains(caseType, "invalid") ||
		strings.Contains(caseType, "failure") || strings.Contains(caseType, "error") ||
		strings.Contains(caseType, "boundary") || strings.Contains(caseType, "validation") {
		return true
	}
	return isDiffCase(apiCase)
}

func isDiffCase(apiCase catalog.APICase) bool {
	if strings.EqualFold(strings.TrimSpace(apiCase.RenderMode), "template_patch") {
		return true
	}
	patch := strings.TrimSpace(apiCase.PatchJSON)
	return patch != "" && patch != "[]" && patch != "{}"
}

func stringDefault(value string, defaultValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	return value
}

func jsonDefault(value string, defaultValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return value
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return value
	}
	return string(encoded)
}

func jsonText(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
