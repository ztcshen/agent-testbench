package mapplanner

import (
	"fmt"
	"strings"

	"agent-testbench/internal/domain/plangraph"
)

func normalizeQuery(graph plangraph.Graph, query Query) Query {
	query.MapID = firstNonEmpty(strings.TrimSpace(query.MapID), graph.Map.ID)
	query.PlannerMode = firstNonEmpty(strings.TrimSpace(query.PlannerMode), ModeExplain)
	scopeWasEmpty := strings.TrimSpace(query.Scope) == ""
	query.Scope = strings.TrimSpace(query.Scope)
	if query.Scope == "workflow" {
		query.Scope = ScopeWorkflows
	}
	query.TargetKind = strings.TrimSpace(query.TargetKind)
	query.TargetID = strings.TrimSpace(query.TargetID)
	query.InterfaceNodeID = strings.TrimSpace(query.InterfaceNodeID)
	query.ValidationFamily = normalizeValidationFamily(query.ValidationFamily)
	query.Role = strings.TrimSpace(query.Role)
	switch {
	case strings.TrimSpace(query.CaseID) != "":
		query.Scope = ScopeCase
		query.TargetKind = TargetCase
		query.TargetID = strings.TrimSpace(query.CaseID)
	case strings.TrimSpace(query.NodeID) != "":
		query.Scope = ScopeCase
		query.TargetKind = TargetNode
		query.TargetID = strings.TrimSpace(query.NodeID)
	case strings.TrimSpace(query.PathID) != "":
		query.Scope = ScopeWorkflows
		query.TargetKind = TargetPath
		query.TargetID = strings.TrimSpace(query.PathID)
	case strings.TrimSpace(query.WorkflowID) != "":
		query.Scope = ScopeWorkflows
		query.TargetKind = TargetWorkflow
		query.TargetID = strings.TrimSpace(query.WorkflowID)
	default:
		if (query.InterfaceNodeID != "" || query.ValidationFamily != "" || query.Role != "") && (scopeWasEmpty || query.Scope == "") {
			query.Scope = ScopeCases
		}
		switch query.TargetKind {
		case TargetCase, TargetNode:
			if scopeWasEmpty || query.Scope == ScopeCases {
				query.Scope = ScopeCase
			}
		case TargetPath, TargetWorkflow:
			if scopeWasEmpty {
				query.Scope = ScopeWorkflows
			}
		}
		query.Scope = stringDefault(query.Scope, ScopeAll)
		query.TargetKind = stringDefault(query.TargetKind, TargetMap)
		query.TargetID = stringDefault(query.TargetID, query.MapID)
	}
	return query
}

func validateQueryScopeTarget(query Query) error {
	switch query.Scope {
	case ScopeWorkflows:
		if query.TargetKind == TargetCase || query.TargetKind == TargetNode {
			return fmt.Errorf("map planner scope %s conflicts with target kind %s", query.Scope, query.TargetKind)
		}
	case ScopeCases, ScopeCase:
		if query.TargetKind == TargetPath || query.TargetKind == TargetWorkflow {
			return fmt.Errorf("map planner scope %s conflicts with target kind %s", query.Scope, query.TargetKind)
		}
	}
	return nil
}

func (b *planBuilder) workflowTargeted() bool {
	return b.query.TargetKind == TargetPath || b.query.TargetKind == TargetWorkflow
}

func (b *planBuilder) caseTargeted() bool {
	return b.query.TargetKind == TargetCase || b.query.TargetKind == TargetNode
}

func (b *planBuilder) pathMatchesQuery(path plangraph.Path) bool {
	switch b.query.TargetKind {
	case TargetPath:
		return path.ID == b.query.TargetID
	case TargetWorkflow:
		return path.WorkflowID == b.query.TargetID
	default:
		return true
	}
}

func (b *planBuilder) findTargetNode() (plangraph.Node, bool) {
	for _, node := range b.graph.Nodes {
		if b.nodeMatchesQuery(node) {
			return node, true
		}
	}
	return plangraph.Node{}, false
}

func (b *planBuilder) nodeMatchesQuery(node plangraph.Node) bool {
	switch b.query.TargetKind {
	case TargetNode:
		return node.ID == b.query.TargetID
	case TargetCase:
		return node.CaseID == b.query.TargetID
	default:
		return true
	}
}

func (b *planBuilder) validationNodeMatchesFilters(node plangraph.Node) bool {
	if b.query.InterfaceNodeID != "" && node.InterfaceNodeID != b.query.InterfaceNodeID {
		return false
	}
	if b.query.ValidationFamily != "" && plangraph.ValidationFamilyForNode(node) != b.query.ValidationFamily {
		return false
	}
	if b.query.Role != "" && node.Role != b.query.Role {
		return false
	}
	return true
}

func isValidationNode(node plangraph.Node) bool {
	return node.Role == plangraph.NodeRoleValidation || node.StateEffect == plangraph.StateEffectUnchanged
}

func normalizeValidationFamily(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "_", "-")
	switch value {
	case "empty", "null", "blank", "required", "missing":
		return "empty/null"
	default:
		return value
	}
}
