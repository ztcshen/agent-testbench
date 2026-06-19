package mapplanner

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"agent-testbench/internal/domain/plangraph"
)

func (b *planBuilder) prepareReplayGroup(plan *Plan, node plangraph.Node, task PhysicalTask) (string, string, bool) {
	if task.Kind != TaskReuseMaterialized && task.Kind != TaskRunPathPrefix {
		return "", "", false
	}
	family := plangraph.ValidationFamilyForNode(node)
	key := strings.Join([]string{
		node.InterfaceNodeID,
		node.AnchorNodeID,
		family,
		task.PathID,
		task.UntilNodeID,
		task.MaterializationID,
	}, "\x00")
	groupIndex, ok := b.replayGroupByKey[key]
	if !ok {
		group := ReplayGroup{
			ID:                replayGroupID(node, family, task),
			InterfaceNodeID:   node.InterfaceNodeID,
			AnchorNodeID:      node.AnchorNodeID,
			AnchorCaseID:      node.BaseCaseID,
			ValidationFamily:  family,
			PathID:            task.PathID,
			WorkflowID:        task.WorkflowID,
			UntilNodeID:       task.UntilNodeID,
			MaterializationID: task.MaterializationID,
			Decision:          replayGroupDecision(task),
			Reason:            replayGroupReason(task),
		}
		plan.ReplayGroups = append(plan.ReplayGroups, group)
		groupIndex = len(plan.ReplayGroups) - 1
		b.replayGroupByKey[key] = groupIndex
	}
	group := &plan.ReplayGroups[groupIndex]
	appendReplayGroupMember(group, node)
	reusable := task.Kind == TaskReuseMaterialized && strings.TrimSpace(task.MaterializationID) != ""
	reusedTaskID := ""
	if reusable {
		reusedTaskID = b.replayTaskByGroup[group.ID]
	}
	return group.ID, reusedTaskID, reusable
}

func replayGroupID(node plangraph.Node, family string, task PhysicalTask) string {
	return strings.Join([]string{
		"replay",
		replayGroupIDComponent(firstNonEmpty(node.InterfaceNodeID, "interface")),
		replayGroupIDComponent(firstNonEmpty(node.AnchorNodeID, task.UntilNodeID, "anchor")),
		replayGroupIDComponent(firstNonEmpty(family, "family")),
		replayGroupIDComponent(firstNonEmpty(task.PathID, "path")),
		replayGroupIDComponent(firstNonEmpty(task.UntilNodeID, "until")),
		replayGroupIDComponent(firstNonEmpty(task.MaterializationID, "materialization")),
	}, ".")
}

func replayGroupIDComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "unknown"
	}
	sum := sha256.Sum256([]byte(value))
	return safeID(value) + "_" + hex.EncodeToString(sum[:6])
}

func (b *planBuilder) addReplayGroupTask(plan *Plan, groupID string, taskID string) {
	if strings.TrimSpace(groupID) == "" || strings.TrimSpace(taskID) == "" {
		return
	}
	for index := range plan.ReplayGroups {
		if plan.ReplayGroups[index].ID != groupID {
			continue
		}
		if !stringInList(plan.ReplayGroups[index].TaskIDs, taskID) {
			plan.ReplayGroups[index].TaskIDs = append(plan.ReplayGroups[index].TaskIDs, taskID)
		}
		if b.replayTaskByGroup[groupID] == "" {
			b.replayTaskByGroup[groupID] = taskID
		}
		return
	}
}

func replayGroupDecision(task PhysicalTask) string {
	if task.Kind == TaskReuseMaterialized && strings.TrimSpace(task.MaterializationID) != "" {
		return "reused"
	}
	if task.Kind == TaskRunPathPrefix {
		return "duplicated"
	}
	return "unavailable"
}

func replayGroupReason(task PhysicalTask) string {
	switch replayGroupDecision(task) {
	case "reused":
		return "validation cases share a Store-backed materialized replay checkpoint"
	case "duplicated":
		return "validation cases share a replay prefix, but no compatible materialization is available"
	default:
		return "validation case has no replay checkpoint"
	}
}

func appendReplayGroupMember(group *ReplayGroup, node plangraph.Node) {
	if !stringInList(group.NodeIDs, node.ID) {
		group.NodeIDs = append(group.NodeIDs, node.ID)
	}
	if strings.TrimSpace(node.CaseID) != "" && !stringInList(group.CaseIDs, node.CaseID) {
		group.CaseIDs = append(group.CaseIDs, node.CaseID)
	}
	group.Count = len(group.CaseIDs)
	if group.Count == 0 {
		group.Count = len(group.NodeIDs)
	}
}
