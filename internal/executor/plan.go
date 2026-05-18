package executor

import (
	"context"
	"sort"
	"strings"
	"time"

	"open-test-sandbox/internal/profile"
)

type Counts struct {
	Total   int `json:"total"`
	Ready   int `json:"ready"`
	Blocked int `json:"blocked"`
}

type PlanItem struct {
	ID             string   `json:"id"`
	DisplayName    string   `json:"displayName,omitempty"`
	Kind           string   `json:"kind"`
	Tool           string   `json:"tool,omitempty"`
	SourcePath     string   `json:"sourcePath,omitempty"`
	Command        string   `json:"command,omitempty"`
	Args           []string `json:"args,omitempty"`
	WorkingDir     string   `json:"workingDir,omitempty"`
	Status         string   `json:"status"`
	RunMode        string   `json:"runMode"`
	ArtifactPaths  []string `json:"artifactPaths,omitempty"`
	TimeoutSeconds int      `json:"timeoutSeconds,omitempty"`
	Ready          bool     `json:"ready"`
	Issues         []string `json:"issues,omitempty"`
}

type PlanReport struct {
	OK          bool       `json:"ok"`
	ProfileID   string     `json:"profileId"`
	GeneratedAt string     `json:"generatedAt"`
	Counts      Counts     `json:"counts"`
	Items       []PlanItem `json:"items"`
	Warnings    []string   `json:"warnings,omitempty"`
}

func Plan(_ context.Context, bundle profile.Bundle) PlanReport {
	report := PlanReport{
		OK:          true,
		ProfileID:   bundle.ID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Items:       []PlanItem{},
	}
	if len(bundle.Executors) == 0 {
		report.Warnings = append(report.Warnings, "profile has no executor descriptors")
	}
	for _, descriptor := range sortedDescriptors(bundle.Executors) {
		item := planItem(descriptor)
		report.Counts.Total++
		if item.Ready {
			report.Counts.Ready++
		} else {
			report.Counts.Blocked++
			report.OK = false
		}
		report.Items = append(report.Items, item)
	}
	return report
}

func sortedDescriptors(values []profile.ExecutorDescriptor) []profile.ExecutorDescriptor {
	out := append([]profile.ExecutorDescriptor(nil), values...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func planItem(descriptor profile.ExecutorDescriptor) PlanItem {
	item := PlanItem{
		ID:             strings.TrimSpace(descriptor.ID),
		DisplayName:    strings.TrimSpace(descriptor.DisplayName),
		Kind:           normalizeKind(descriptor.Kind),
		Tool:           strings.TrimSpace(descriptor.Tool),
		SourcePath:     strings.TrimSpace(descriptor.SourcePath),
		Command:        strings.TrimSpace(descriptor.Command),
		Args:           append([]string(nil), descriptor.Args...),
		WorkingDir:     strings.TrimSpace(descriptor.WorkingDir),
		Status:         normalizeStatus(descriptor.Status),
		RunMode:        "dry-run",
		ArtifactPaths:  append([]string(nil), descriptor.ArtifactPaths...),
		TimeoutSeconds: descriptor.TimeoutSeconds,
	}
	item.Issues = descriptorIssues(item)
	item.Ready = len(item.Issues) == 0
	return item
}

func descriptorIssues(item PlanItem) []string {
	issues := []string{}
	if item.ID == "" {
		issues = append(issues, "missing-id")
	}
	if item.Kind == "" {
		issues = append(issues, "missing-kind")
	} else if !supportedKind(item.Kind) {
		issues = append(issues, "unsupported-kind")
	}
	if item.Status != "active" {
		issues = append(issues, "inactive")
	}
	switch item.Kind {
	case "custom-command":
		if item.Command == "" {
			issues = append(issues, "missing-command")
		}
	case "playwright", "postman", "k6", "pytest", "karate":
		if item.SourcePath == "" {
			issues = append(issues, "missing-source-path")
		}
	}
	return issues
}

func normalizeKind(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeStatus(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "active"
	}
	return value
}

func supportedKind(value string) bool {
	switch normalizeKind(value) {
	case "http-case", "playwright", "postman", "k6", "pytest", "karate", "custom-command":
		return true
	default:
		return false
	}
}
