package main

import (
	"fmt"
	"testing"

	"agent-testbench/internal/store"
)

func TestMapAtlasLayoutWrapsLongWorkflowRows(t *testing.T) {
	steps := make([]store.TestPlanPathStep, 0, 9)
	nodes := make([]store.TestPlanNode, 0, 9)
	for index := 1; index <= 9; index++ {
		nodeID := fmt.Sprintf("case.%02d", index)
		nodes = append(nodes, store.TestPlanNode{ID: nodeID, CaseID: nodeID, Role: "primary", StateEffect: "advance"})
		steps = append(steps, store.TestPlanPathStep{PathID: "workflow.long", StepIndex: index, NodeID: nodeID, CaseID: nodeID})
	}

	layout := mapAtlasLayout(steps, nodes)
	if layout["case.05"].X != layout["case.04"].X {
		t.Fatalf("fifth node should fold under the fourth node to keep one continuous line: %#v", layout)
	}
	if layout["case.05"].Y <= layout["case.01"].Y {
		t.Fatalf("fifth node should move to the next row: %#v", layout)
	}
	if layout["case.06"].X >= layout["case.05"].X || layout["case.07"].X >= layout["case.06"].X || layout["case.08"].X >= layout["case.07"].X {
		t.Fatalf("second row should continue the workflow right-to-left: %#v", layout)
	}
	if layout["case.08"].X != layout["case.01"].X {
		t.Fatalf("eighth node should end the folded row above the first column: %#v", layout)
	}
	if layout["case.09"].X != layout["case.08"].X || layout["case.09"].Y <= layout["case.05"].Y {
		t.Fatalf("ninth node should fold under the eighth node to keep one continuous line: %#v", layout)
	}
	if layout["case.04"].X <= layout["case.03"].X || layout["case.04"].Y != layout["case.01"].Y {
		t.Fatalf("first four nodes should stay on the same row: %#v", layout)
	}
}

func TestMapAtlasLayoutPlacesValidationCasesNearAnchor(t *testing.T) {
	steps := []store.TestPlanPathStep{
		{PathID: "workflow.main", StepIndex: 1, NodeID: "case.submit", CaseID: "case.submit"},
	}
	nodes := []store.TestPlanNode{
		{ID: "case.submit", CaseID: "case.submit", InterfaceNodeID: "node.submit", Role: "primary", StateEffect: "advance"},
	}
	for index := 0; index < 12; index++ {
		nodeID := fmt.Sprintf("case.unrelated.%02d", index)
		nodes = append(nodes, store.TestPlanNode{ID: nodeID, CaseID: nodeID, Role: "primary", StateEffect: "advance"})
	}
	nodes = append(nodes,
		store.TestPlanNode{ID: "case.submit.required", CaseID: "case.submit.required", InterfaceNodeID: "node.submit", BaseCaseID: "case.submit", Role: "validation", StateEffect: "unchanged"},
		store.TestPlanNode{ID: "case.submit.invalid", CaseID: "case.submit.invalid", InterfaceNodeID: "node.submit", BaseCaseID: "case.submit", Role: "validation", StateEffect: "unchanged"},
	)

	layout := mapAtlasLayout(steps, nodes)
	anchor := layout["case.submit"]
	required := layout["case.submit.required"]
	invalid := layout["case.submit.invalid"]
	if required.X < anchor.X || required.X > anchor.X+620 {
		t.Fatalf("validation node should stay near its anchor horizontally: anchor=%#v required=%#v", anchor, required)
	}
	if required.Y <= anchor.Y {
		t.Fatalf("validation node should be below its anchor: anchor=%#v required=%#v", anchor, required)
	}
	if invalid.X <= required.X {
		t.Fatalf("validation siblings should spread within the local anchor cluster: required=%#v invalid=%#v", required, invalid)
	}
}
