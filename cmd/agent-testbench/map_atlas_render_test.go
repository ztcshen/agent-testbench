package main

import (
	"strings"
	"testing"

	"agent-testbench/internal/store"
)

func TestMapAtlasDefaultsValidationCasesToCollectiveInterfaceSummary(t *testing.T) {
	document := mapAtlasDocument{
		Version: "1.0",
		Map: store.TestPlanMap{
			ID:          "map.validation",
			ProfileID:   "profile.validation",
			DisplayName: "Validation Map",
			Status:      "active",
		},
		Nodes: []mapAtlasNode{
			{
				ID:              "case.submit",
				CaseID:          "case.submit",
				DisplayName:     "Submit",
				InterfaceNodeID: "node.submit",
				Role:            "primary",
				StateEffect:     "advance",
				Layout:          mapAtlasNodeLayout{X: 100, Y: 100},
			},
			{
				ID:              "case.submit.field.required",
				CaseID:          "case.submit.field.required",
				DisplayName:     "Field required",
				InterfaceNodeID: "node.submit",
				BaseCaseID:      "case.submit",
				Role:            "validation",
				StateEffect:     "unchanged",
				RenderMode:      "template_patch",
				Tags:            []string{"negative", "schema"},
				Layout:          mapAtlasNodeLayout{X: 380, Y: 100},
			},
		},
	}

	html, err := renderMapAtlasHTML(document)
	if err != nil {
		t.Fatalf("render atlas: %v", err)
	}
	for _, want := range []string{
		`let showValidationNodes=false`,
		`function nodeDrawn`,
		`let activeView="map"`,
		`function renderInterfaceView`,
		`id="language-select"`,
		`function tr(key)`,
		`function caseFamilySummaries`,
		`function validationEdgeInScope`,
		`function edgePath`,
		`function edgePorts`,
		`fromBottom`,
		`toTop`,
		`function renderArrowMarkers`,
		`marker-end`,
		`map-atlas-arrow`,
		`function setInterfaceMode`,
		`function openValidationDetail`,
		`interface-main`,
		`selectedCaseSummary`,
		`Test families`,
		`接口详情`,
		`运行任务`,
		`validationBadge`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("atlas should include collective validation UI %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, `Interface reverse cases`) {
		t.Fatalf("atlas should not default the interface detail to a raw reverse-case list:\n%s", html)
	}
	if strings.Contains(html, `showValidationNodes&&!!key&&interfaceKey(n)===key`) {
		t.Fatalf("atlas should not draw all validation children on the main graph:\n%s", html)
	}
	if !strings.Contains(html, `function toggleValidationNodes(){openValidationDetail()}`) {
		t.Fatalf("validation toolbar action should open interface detail instead of flooding the main graph:\n%s", html)
	}
}

func TestMapAtlasDerivesValidationAnchorEdges(t *testing.T) {
	graph := store.TestPlanGraph{
		Nodes: []store.TestPlanNode{
			{
				ID:              "case.submit",
				CaseID:          "case.submit",
				InterfaceNodeID: "node.submit",
				Role:            "primary",
				StateEffect:     "advance",
			},
			{
				ID:              "case.submit.field.required",
				CaseID:          "case.submit.field.required",
				InterfaceNodeID: "node.submit",
				BaseCaseID:      "case.submit",
				Role:            "validation",
				StateEffect:     "unchanged",
			},
		},
	}

	edges := mapAtlasEdges(graph)
	for _, edge := range edges {
		if edge.FromNodeID == "case.submit" && edge.ToNodeID == "case.submit.field.required" {
			if edge.Kind != "validation" || !edge.Generated {
				t.Fatalf("validation edge should be generated and typed: %#v", edge)
			}
			return
		}
	}
	t.Fatalf("expected generated validation edge, got %#v", edges)
}

func TestMapAtlasTreatsPrimaryPatchCaseAsMainPathNode(t *testing.T) {
	graph := store.TestPlanGraph{
		Nodes: []store.TestPlanNode{
			{
				ID:          "case.smoke.patch",
				CaseID:      "case.smoke.patch",
				Role:        "primary",
				StateEffect: "advance",
				RenderMode:  "template_patch",
			},
		},
		PathSteps: []store.TestPlanPathStep{
			{PathID: "workflow.smoke", StepIndex: 1, NodeID: "case.smoke.patch", CaseID: "case.smoke.patch", Required: true},
		},
	}

	if mapAtlasNodeIsValidation(graph.Nodes[0]) {
		t.Fatalf("primary workflow patch case should not be classified as validation: %#v", graph.Nodes[0])
	}
	if warnings := strings.Join(mapAtlasWarnings(graph), "\n"); strings.Contains(warnings, "case.smoke.patch") {
		t.Fatalf("primary workflow patch case should be treated as path-connected:\n%s", warnings)
	}

	html, err := renderMapAtlasHTML(mapAtlasDocument{Nodes: []mapAtlasNode{{
		ID:          "case.smoke.patch",
		CaseID:      "case.smoke.patch",
		Role:        "primary",
		StateEffect: "advance",
		RenderMode:  "template_patch",
	}}})
	if err != nil {
		t.Fatalf("render atlas: %v", err)
	}
	if !strings.Contains(html, `mode==="template_patch"&&role!=="primary"`) {
		t.Fatalf("browser-side reverse-case classification should not hide primary patch nodes:\n%s", html)
	}
}

func TestMapAtlasWarningsIgnoreStandaloneValidationNodes(t *testing.T) {
	graph := store.TestPlanGraph{
		Nodes: []store.TestPlanNode{
			{ID: "case.primary", Role: "primary", StateEffect: "advance"},
			{ID: "case.validation", Role: "validation", StateEffect: "unchanged", BaseCaseID: "case.primary"},
			{ID: "case.orphan", Role: "primary", StateEffect: "advance"},
		},
		PathSteps: []store.TestPlanPathStep{
			{PathID: "workflow.main", StepIndex: 1, NodeID: "case.primary", CaseID: "case.primary", Required: true},
		},
	}

	warnings := strings.Join(mapAtlasWarnings(graph), "\n")
	if strings.Contains(warnings, "case.validation is not used by any workflow path") {
		t.Fatalf("standalone validation nodes should be summarized as interface test families, not warnings:\n%s", warnings)
	}
	if !strings.Contains(warnings, "case.orphan is not used by any workflow path") {
		t.Fatalf("primary orphan node should still warn:\n%s", warnings)
	}
}
