package main

import (
	"fmt"
	"sort"

	"agent-testbench/internal/store"
)

const mapAtlasLayoutColumns = 4
const mapAtlasValidationColumns = 4

func mapAtlasLayout(steps []store.TestPlanPathStep, nodes []store.TestPlanNode) map[string]mapAtlasNodeLayout {
	pathOrder := mapAtlasPathOrder(steps)
	rowBase := mapAtlasPathRowBase(steps, pathOrder)
	slots := mapAtlasLayoutSlots(steps, rowBase)
	return mapAtlasLayoutFromSlots(nodes, slots, len(pathOrder))
}

func mapAtlasPathOrder(steps []store.TestPlanPathStep) map[string]int {
	out := map[string]int{}
	for _, step := range steps {
		if _, ok := out[step.PathID]; !ok {
			out[step.PathID] = len(out)
		}
	}
	return out
}

func mapAtlasPathRowBase(steps []store.TestPlanPathStep, pathOrder map[string]int) map[string]int {
	maxStepByPath := map[string]int{}
	for _, step := range steps {
		if step.StepIndex > maxStepByPath[step.PathID] {
			maxStepByPath[step.PathID] = step.StepIndex
		}
	}
	paths := make([]string, 0, len(pathOrder))
	for pathID := range pathOrder {
		paths = append(paths, pathID)
	}
	sort.Slice(paths, func(i int, j int) bool {
		return pathOrder[paths[i]] < pathOrder[paths[j]]
	})

	out := map[string]int{}
	nextRow := 0
	for _, pathID := range paths {
		out[pathID] = nextRow
		rows := (maxStepByPath[pathID] + mapAtlasLayoutColumns - 1) / mapAtlasLayoutColumns
		if rows < 1 {
			rows = 1
		}
		nextRow += rows
	}
	return out
}

func mapAtlasLayoutSlots(steps []store.TestPlanPathStep, rowBase map[string]int) map[string]mapAtlasLayoutSlot {
	slots := map[string]mapAtlasLayoutSlot{}
	for _, step := range steps {
		current, ok := slots[step.NodeID]
		level := mapAtlasStepLevel(step.StepIndex)
		row := float64(rowBase[step.PathID] + mapAtlasStepRow(step.StepIndex))
		if !ok {
			slots[step.NodeID] = mapAtlasLayoutSlot{level: level, row: row, count: 1}
			continue
		}
		if level < current.level {
			current.level = level
		}
		current.row += row
		current.count++
		slots[step.NodeID] = current
	}
	return slots
}

func mapAtlasStepLevel(stepIndex int) int {
	if stepIndex <= 1 {
		return 0
	}
	index := stepIndex - 1
	column := index % mapAtlasLayoutColumns
	if (index/mapAtlasLayoutColumns)%2 == 1 {
		return mapAtlasLayoutColumns - 1 - column
	}
	return column
}

func mapAtlasStepRow(stepIndex int) int {
	if stepIndex <= 1 {
		return 0
	}
	return (stepIndex - 1) / mapAtlasLayoutColumns
}

func mapAtlasLayoutFromSlots(nodes []store.TestPlanNode, slots map[string]mapAtlasLayoutSlot, pathCount int) map[string]mapAtlasNodeLayout {
	out := map[string]mapAtlasNodeLayout{}
	collisions := map[string]int{}
	primaryByInterface := map[string]string{}
	for index, node := range nodes {
		if mapAtlasNodeIsValidation(node) {
			continue
		}
		current, ok := slots[node.ID]
		if !ok || current.count == 0 {
			current = mapAtlasLayoutSlot{level: index, row: float64(pathCount + 1), count: 1}
		}
		row := current.row / float64(current.count)
		key := fmt.Sprintf("%d:%.1f", current.level, row)
		offset := collisions[key]
		collisions[key]++
		out[node.ID] = mapAtlasNodeLayout{
			X: 80 + current.level*290,
			Y: 90 + int(row*165) + offset*42,
		}
		if node.InterfaceNodeID != "" && primaryByInterface[node.InterfaceNodeID] == "" {
			primaryByInterface[node.InterfaceNodeID] = node.ID
		}
	}
	validationOffsets := map[string]int{}
	for index, node := range nodes {
		if !mapAtlasNodeIsValidation(node) {
			continue
		}
		anchorID := mapAtlasValidationAnchorID(node, primaryByInterface)
		anchorLayout, ok := out[anchorID]
		if !ok {
			current := mapAtlasLayoutSlot{level: index, row: float64(pathCount + 1), count: 1}
			row := current.row / float64(current.count)
			out[node.ID] = mapAtlasNodeLayout{
				X: 80 + current.level*290,
				Y: 90 + int(row*165),
			}
			continue
		}
		offset := validationOffsets[anchorID]
		validationOffsets[anchorID]++
		out[node.ID] = mapAtlasNodeLayout{
			X: anchorLayout.X + 290 + (offset%mapAtlasValidationColumns)*250,
			Y: anchorLayout.Y + 150 + (offset/mapAtlasValidationColumns)*118,
		}
	}
	return out
}

func mapAtlasValidationAnchorID(node store.TestPlanNode, primaryByInterface map[string]string) string {
	if node.AnchorNodeID != "" {
		return node.AnchorNodeID
	}
	if node.BaseCaseID != "" {
		return node.BaseCaseID
	}
	return primaryByInterface[node.InterfaceNodeID]
}
