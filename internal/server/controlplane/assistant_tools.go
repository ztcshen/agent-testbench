package controlplane

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/domain/plangraph"
	"agent-testbench/internal/domain/redaction"
	"agent-testbench/internal/store"
)

type assistantToolRequest struct {
	SessionID string            `json:"sessionId"`
	Action    string            `json:"action"`
	Args      map[string]string `json:"args"`
}

func handleAssistantTool(w http.ResponseWriter, r *http.Request, runtime store.Store) {
	sessions, ok := runtime.(store.AssistantSessionStore)
	if !ok {
		assistantError(w, 503, errors.New("assistant Store is unavailable"))
		return
	}
	var input assistantToolRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		assistantError(w, 400, errors.New("invalid assistant tool request"))
		return
	}
	session, err := sessions.GetAssistantSession(r.Context(), input.SessionID)
	if assistantReadError(w, err) {
		return
	}
	graph, err := runtime.GetTestPlanGraph(r.Context(), session.MapID)
	if assistantReadError(w, err) {
		return
	}
	if graph.Map.ProfileID != session.ProfileID {
		assistantError(w, 409, errors.New("map profile changed; start a new conversation"))
		return
	}
	state := jsonObject(session.StateJSON)
	if fingerprint := valueString(state["graphFingerprint"]); fingerprint != "" && fingerprint != mapplanner.GraphFingerprint(graph) {
		assistantError(w, 409, errors.New("map changed during analysis; start a new analysis"))
		return
	}
	args := input.Args
	if args == nil {
		args = map[string]string{}
	}
	for key, value := range args {
		if len(value) > 512 || (key != "nodeId" && key != "pathId" && key != "caseRunId" && key != "query") {
			assistantError(w, 400, errors.New("unsupported assistant tool argument"))
			return
		}
	}
	nodeID, pathID := args["nodeId"], args["pathId"]
	focus := mapFromAny(state["focus"])
	if nodeID == "" {
		nodeID = valueString(focus["nodeId"])
	}
	if pathID == "" {
		pathID = valueString(focus["pathId"])
	}
	if !assistantGraphSelection(graph, nodeID, pathID) {
		assistantError(w, 400, errors.New("node or path is outside the selected map"))
		return
	}
	var data any
	switch input.Action {
	case "map.read":
		data, err = assistantMapContext(r, runtime, graph, nodeID, pathID, args["query"])
	case "node.explain":
		if nodeID == "" {
			err = errors.New("select a node before explaining dependencies")
			break
		}
		data, err = plangraph.ExplainCase(graph, plangraph.ExplainOptions{NodeID: nodeID})
	case "plan.explain":
		query := mapplanner.Query{MapID: graph.Map.ID, Scope: mapplanner.ScopeAll, PlannerMode: mapplanner.ModeExplain}
		if nodeID != "" {
			query.Scope, query.NodeID = mapplanner.ScopeCase, nodeID
		} else if pathID != "" {
			query.Scope, query.PathID = mapplanner.ScopeWorkflows, pathID
		}
		data, err = mapplanner.Explain(graph, query)
	case "runs.list", "evidence.read":
		data, err = assistantCaseEvidence(r, runtime, graph, nodeID, pathID, args["caseRunId"], input.Action == "evidence.read")
	default:
		err = errors.New("assistant tool is not registered")
	}
	if err != nil {
		assistantError(w, 400, err)
		return
	}
	projected := redaction.Value(cloneJSONObject(map[string]any{"value": data})).(map[string]any)
	writeJSON(w, map[string]any{"ok": true, "action": input.Action, "mapId": session.MapID, "data": projected["value"]})
}

func assistantGraphSelection(graph store.TestPlanGraph, nodeID, pathID string) bool {
	nodeOK, pathOK := nodeID == "", pathID == ""
	for _, node := range graph.Nodes {
		if node.ID == nodeID {
			nodeOK = true
		}
	}
	for _, path := range graph.Paths {
		if path.ID == pathID {
			pathOK = true
		}
	}
	return nodeOK && pathOK
}

func assistantMapContext(r *http.Request, runtime store.Store, graph store.TestPlanGraph, nodeID, pathID, query string) (map[string]any, error) {
	catalog, err := runtime.GetProfileCatalogByID(r.Context(), graph.Map.ProfileID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	pathNodes := map[string]bool{}
	for _, step := range graph.PathSteps {
		if step.PathID == pathID {
			pathNodes[step.NodeID] = true
		}
	}
	selected := []store.TestPlanNode{}
	query = strings.ToLower(strings.TrimSpace(query))
	matched := 0
	for _, node := range graph.Nodes {
		include := pathID == "" || pathNodes[node.ID] || pathNodes[node.AnchorNodeID]
		if nodeID != "" {
			include = node.ID == nodeID || node.AnchorNodeID == nodeID
		}
		if query != "" {
			include = strings.Contains(strings.ToLower(node.ID+" "+node.CaseID+" "+node.SummaryJSON), query)
		}
		if !include {
			continue
		}
		matched++
		if len(selected) < 60 {
			selected = append(selected, node)
		}
	}
	projection := graph
	projection.Nodes = selected
	projection.Materializations = nil
	visible := map[string]bool{}
	for _, node := range selected {
		visible[node.ID] = true
	}
	projection.Edges = nil
	for _, edge := range graph.Edges {
		if visible[edge.FromNodeID] && visible[edge.ToNodeID] {
			projection.Edges = append(projection.Edges, edge)
		}
	}
	projection.PathSteps = nil
	for _, step := range graph.PathSteps {
		if visible[step.NodeID] {
			projection.PathSteps = append(projection.PathSteps, step)
		}
	}
	return map[string]any{"graph": projection, "context": mapReadContext(projection, catalog), "totalNodes": len(graph.Nodes), "matchedNodes": matched, "truncated": matched > len(selected), "graphFingerprint": mapplanner.GraphFingerprint(graph)}, nil
}

func assistantCaseEvidence(r *http.Request, runtime store.Store, graph store.TestPlanGraph, nodeID, pathID, caseRunID string, detail bool) (map[string]any, error) {
	cases := map[string]bool{}
	pathNodes := map[string]bool{}
	for _, step := range graph.PathSteps {
		if step.PathID == pathID {
			pathNodes[step.NodeID] = true
		}
	}
	for _, node := range graph.Nodes {
		if (nodeID == "" || node.ID == nodeID) && (pathID == "" || pathNodes[node.ID] || pathNodes[node.AnchorNodeID]) {
			cases[node.CaseID], cases[node.BaseCaseID] = true, true
		}
	}
	delete(cases, "")
	runs, err := runtime.ListRuns(r.Context())
	if err != nil {
		return nil, err
	}
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].CreatedAt.After(runs[j].CreatedAt) })
	items := []map[string]any{}
	checked := 0
	for _, run := range runs {
		if run.ProfileID != graph.Map.ProfileID {
			continue
		}
		checked++
		if checked > 50 && caseRunID == "" {
			break
		}
		children, readErr := runtime.ListAPICaseRuns(r.Context(), run.ID)
		if readErr != nil {
			return nil, readErr
		}
		for _, child := range children {
			if !cases[child.CaseID] || (caseRunID != "" && child.ID != caseRunID) {
				continue
			}
			item := map[string]any{"caseRunId": child.ID, "runId": run.ID, "caseId": child.CaseID, "nodeId": child.TestPlanNodeID, "status": child.Status, "createdAt": child.CreatedAt, "assertions": jsonObject(child.AssertionSummaryJSON)}
			if detail {
				records, evidenceErr := runtime.ListEvidence(r.Context(), run.ID)
				if evidenceErr != nil {
					return nil, evidenceErr
				}
				refs := []map[string]any{}
				for _, record := range records {
					if record.CaseRunID == child.ID {
						refs = append(refs, map[string]any{"id": record.ID, "kind": record.Kind, "summary": record.Summary, "sha256": record.SHA256, "category": record.Category})
					}
				}
				item["evidenceRecords"] = refs
				item["requestSummary"] = assistantRequestSignals(child.RequestSummaryJSON)
				item["runSummary"] = assistantRequestSignals(run.SummaryJSON)
			}
			items = append(items, item)
			if len(items) >= 12 {
				return map[string]any{"runs": items, "truncated": true}, nil
			}
		}
	}
	if caseRunID != "" && len(items) == 0 {
		return nil, errors.New("case run is unavailable in the selected map/profile")
	}
	return map[string]any{"runs": items, "truncated": checked > 50}, nil
}

func assistantRequestSignals(raw string) map[string]any {
	source := jsonObject(raw)
	result := map[string]any{}
	for _, key := range []string{"method", "path", "statusCode", "httpStatus", "error", "durationMs", "failureKind"} {
		if value, ok := source[key]; ok {
			result[key] = value
		}
	}
	return result
}
