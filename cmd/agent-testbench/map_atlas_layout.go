package main

import (
	"fmt"

	"agent-testbench/internal/store"
)

func mapAtlasLayout(steps []store.TestPlanPathStep, nodes []store.TestPlanNode) map[string]mapAtlasNodeLayout {
	pathOrder := mapAtlasPathOrder(steps)
	slots := mapAtlasLayoutSlots(steps, pathOrder)
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

func mapAtlasLayoutSlots(steps []store.TestPlanPathStep, pathOrder map[string]int) map[string]mapAtlasLayoutSlot {
	slots := map[string]mapAtlasLayoutSlot{}
	for _, step := range steps {
		current, ok := slots[step.NodeID]
		level := mapAtlasStepLevel(step.StepIndex)
		row := float64(pathOrder[step.PathID])
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
	return stepIndex - 1
}

func mapAtlasLayoutFromSlots(nodes []store.TestPlanNode, slots map[string]mapAtlasLayoutSlot, pathCount int) map[string]mapAtlasNodeLayout {
	out := map[string]mapAtlasNodeLayout{}
	collisions := map[string]int{}
	for index, node := range nodes {
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
	}
	return out
}
