package openapi

import (
	"encoding/json"
	"strings"

	"agent-testbench/internal/domain/apicasespec"
	"agent-testbench/internal/domain/openapispec"
	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profiledraft"
)

type Options struct {
	ServiceID   string
	EvidenceDir string
}

type PlanResult struct {
	OK             bool                    `json:"ok"`
	Service        profile.Service         `json:"service"`
	InterfaceNodes []profile.InterfaceNode `json:"interfaceNodes"`
	APICases       []profile.APICase       `json:"apiCases"`
	CaseFiles      []GeneratedCaseFile     `json:"caseFiles"`
	Candidates     []Candidate             `json:"candidates"`
	Warnings       []string                `json:"warnings,omitempty"`
}

type Candidate struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Field  string `json:"field,omitempty"`
	CaseID string `json:"caseId"`
	NodeID string `json:"nodeId"`
	Reason string `json:"reason"`
}

type GeneratedCaseFile struct {
	Path string          `json:"path"`
	Body json.RawMessage `json:"body"`
}

func Plan(raw []byte, options Options) (PlanResult, error) {
	doc, err := openapispec.Decode(raw)
	if err != nil {
		return PlanResult{}, err
	}
	title := doc.Title()
	serviceID := openapispec.ServiceID(options.ServiceID, title)
	result := PlanResult{
		OK: true,
		Service: profile.Service{
			ID:          serviceID,
			DisplayName: title,
			Kind:        "http",
			Status:      "draft",
		},
		Warnings: []string{"generated cases are draft candidates and must be reviewed before activation"},
	}
	for _, opRef := range doc.AlphabeticalOperations() {
		op := opRef.Operation
		requestSchema, ok := openapispec.JSONRequestSchema(op)
		if !ok || len(requestSchema.Required) == 0 {
			continue
		}
		opSlug := opRef.Slug()
		nodeID := "node." + serviceID + "." + opSlug
		displayName := opRef.DisplayName()
		nodeAdded := false
		for _, field := range requestSchema.Required {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			if !nodeAdded {
				result.InterfaceNodes = append(result.InterfaceNodes, profile.InterfaceNode{
					ID:          nodeID,
					DisplayName: displayName,
					ServiceID:   serviceID,
					Operation:   opRef.OperationName(),
					Method:      strings.ToUpper(opRef.Method),
					Path:        opRef.Path,
					Status:      "draft",
					Tags:        profiledraft.CompactTags(append([]string{"generated", "schema"}, op.Tags...)),
					Description: "Draft interface for schema-generated candidate cases.",
					SortOrder:   len(result.InterfaceNodes) + 1,
				})
				nodeAdded = true
			}
			caseSlug := opSlug + ".missing-" + profiledraft.Slug(field)
			caseID := "case." + serviceID + "." + caseSlug
			candidateID := "candidate." + serviceID + "." + caseSlug
			casePath := "api-cases/" + caseID + ".json"
			body := exampleBodyWithoutField(requestSchema, field)
			statusCode := openapispec.FirstClientErrorStatus(op.Responses)
			if statusCode == 0 {
				statusCode = 400
			}
			tags := profiledraft.CompactTags(append([]string{"generated", "schema", "negative"}, op.Tags...))
			result.Candidates = append(result.Candidates, Candidate{
				ID:     candidateID,
				Kind:   "missing-required-field",
				Field:  field,
				CaseID: caseID,
				NodeID: nodeID,
				Reason: "required request field is omitted to test schema validation",
			})
			result.APICases = append(result.APICases, profile.APICase{
				ID:          caseID,
				DisplayName: displayName + " missing " + field,
				Description: "Draft negative case generated from OpenAPI required-field schema.",
				NodeID:      nodeID,
				Tags:        tags,
				Status:      "draft",
				CasePath:    casePath,
				EvidenceDir: strings.TrimSpace(options.EvidenceDir),
				SortOrder:   len(result.APICases) + 1,
			})
			result.CaseFiles = append(result.CaseFiles, GeneratedCaseFile{
				Path: casePath,
				Body: runnableCaseBody(caseID, displayName+" missing "+field, opRef.Method, opRef.Path, body, statusCode),
			})
		}
	}
	if len(result.Candidates) == 0 {
		result.OK = false
		result.Warnings = append(result.Warnings, "no OpenAPI request schemas with required fields were found")
	}
	return result, nil
}

func exampleBodyWithoutField(requestSchema openapispec.Schema, omitted string) map[string]any {
	body := map[string]any{}
	for name, prop := range requestSchema.Properties {
		if name == omitted {
			continue
		}
		body[name] = exampleValue(prop)
	}
	return body
}

func exampleValue(prop openapispec.PropertySchema) any {
	if prop.Example != nil {
		return prop.Example
	}
	switch strings.ToLower(strings.TrimSpace(prop.Type)) {
	case "integer", "number":
		return 1
	case "boolean":
		return true
	case "array":
		return []any{}
	case "object":
		return map[string]any{}
	default:
		return "example"
	}
}

func runnableCaseBody(caseID string, title string, method string, path string, body map[string]any, statusCode int) json.RawMessage {
	item := apicasespec.NewHTTPCase(caseID, title, strings.ToUpper(method), path, map[string]string{"Content-Type": "application/json"}, body, statusCode)
	return apicasespec.JSON(item)
}
