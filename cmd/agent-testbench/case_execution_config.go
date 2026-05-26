package main

import (
	"encoding/json"
	"strings"

	"agent-testbench/internal/domain/profile"
)

func caseExecutionConfigIDs(configs []profile.TemplateConfig) map[string]string {
	out := map[string]string{}
	for _, config := range configs {
		if config.Status != "" && config.Status != "active" {
			continue
		}
		caseID, ok := caseExecutionConfigCaseID(config.ConfigJSON)
		if ok {
			out[caseID] = config.ID
		}
	}
	return out
}

func caseExecutionConfigCaseID(configJSON string) (string, bool) {
	var parsed struct {
		CaseID        string `json:"caseId"`
		CaseExecution struct {
			Method string `json:"method"`
			NodeID string `json:"nodeId"`
			Path   string `json:"path"`
		} `json:"caseExecution"`
	}
	if err := json.Unmarshal([]byte(configJSON), &parsed); err != nil {
		return "", false
	}
	if strings.TrimSpace(parsed.CaseID) == "" {
		return "", false
	}
	if parsed.CaseExecution.Method == "" && parsed.CaseExecution.NodeID == "" && parsed.CaseExecution.Path == "" {
		return "", false
	}
	return parsed.CaseID, true
}
