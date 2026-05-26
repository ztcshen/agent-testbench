package openapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"agent-testbench/internal/domain/apicasespec"
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

type document struct {
	OpenAPI string                    `json:"openapi"`
	Info    info                      `json:"info"`
	Paths   map[string]pathOperations `json:"paths"`
}

type info struct {
	Title string `json:"title"`
}

type pathOperations map[string]operation

type operation struct {
	OperationID string              `json:"operationId"`
	Summary     string              `json:"summary"`
	Tags        []string            `json:"tags"`
	RequestBody requestBody         `json:"requestBody"`
	Responses   map[string]response `json:"responses"`
}

type requestBody struct {
	Content map[string]mediaType `json:"content"`
}

type mediaType struct {
	Schema schema `json:"schema"`
}

type schema struct {
	Type       string                    `json:"type"`
	Required   []string                  `json:"required"`
	Properties map[string]propertySchema `json:"properties"`
}

type propertySchema struct {
	Type    string `json:"type"`
	Example any    `json:"example"`
}

type response struct {
	Description string `json:"description"`
}

func Plan(raw []byte, options Options) (PlanResult, error) {
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return PlanResult{}, fmt.Errorf("decode openapi document: %w", err)
	}
	if strings.TrimSpace(doc.OpenAPI) == "" {
		return PlanResult{}, errors.New("openapi version is required")
	}
	if len(doc.Paths) == 0 {
		return PlanResult{}, errors.New("openapi paths are required")
	}
	title := strings.TrimSpace(doc.Info.Title)
	if title == "" {
		title = "OpenAPI Service"
	}
	serviceID := strings.TrimSpace(options.ServiceID)
	if serviceID == "" {
		serviceID = "service." + profiledraft.Slug(title)
	}
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
	for _, path := range profiledraft.SortedKeys(doc.Paths) {
		ops := doc.Paths[path]
		for _, method := range sortedHTTPMethods(ops) {
			op := ops[method]
			requestSchema, ok := jsonRequestSchema(op)
			if !ok || len(requestSchema.Required) == 0 {
				continue
			}
			opSlug := operationSlug(method, path, op)
			nodeID := "node." + serviceID + "." + opSlug
			nodeAdded := false
			for _, field := range requestSchema.Required {
				field = strings.TrimSpace(field)
				if field == "" {
					continue
				}
				if !nodeAdded {
					result.InterfaceNodes = append(result.InterfaceNodes, profile.InterfaceNode{
						ID:          nodeID,
						DisplayName: profiledraft.FirstNonEmpty(op.Summary, op.OperationID, strings.ToUpper(method)+" "+path),
						ServiceID:   serviceID,
						Operation:   profiledraft.FirstNonEmpty(op.OperationID, strings.ToUpper(method)+" "+path),
						Method:      strings.ToUpper(method),
						Path:        path,
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
				statusCode := firstClientErrorStatus(op.Responses)
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
					DisplayName: profiledraft.FirstNonEmpty(op.Summary, op.OperationID, strings.ToUpper(method)+" "+path) + " missing " + field,
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
					Body: runnableCaseBody(caseID, profiledraft.FirstNonEmpty(op.Summary, op.OperationID, strings.ToUpper(method)+" "+path)+" missing "+field, method, path, body, statusCode),
				})
			}
		}
	}
	if len(result.Candidates) == 0 {
		result.OK = false
		result.Warnings = append(result.Warnings, "no OpenAPI request schemas with required fields were found")
	}
	return result, nil
}

func jsonRequestSchema(op operation) (schema, bool) {
	if op.RequestBody.Content == nil {
		return schema{}, false
	}
	media, ok := op.RequestBody.Content["application/json"]
	if !ok {
		return schema{}, false
	}
	return media.Schema, len(media.Schema.Properties) > 0
}

func exampleBodyWithoutField(requestSchema schema, omitted string) map[string]any {
	body := map[string]any{}
	for name, prop := range requestSchema.Properties {
		if name == omitted {
			continue
		}
		body[name] = exampleValue(prop)
	}
	return body
}

func exampleValue(prop propertySchema) any {
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

func firstClientErrorStatus(responses map[string]response) int {
	codes := []int{}
	for code := range responses {
		if len(code) == 3 && strings.HasPrefix(code, "4") {
			var parsed int
			if _, err := fmt.Sscanf(code, "%d", &parsed); err == nil {
				codes = append(codes, parsed)
			}
		}
	}
	sort.Ints(codes)
	if len(codes) == 0 {
		return 0
	}
	return codes[0]
}

func runnableCaseBody(caseID string, title string, method string, path string, body map[string]any, statusCode int) json.RawMessage {
	item := apicasespec.NewHTTPCase(caseID, title, strings.ToUpper(method), path, map[string]string{"Content-Type": "application/json"}, body, statusCode)
	return apicasespec.JSON(item)
}

func sortedHTTPMethods(values pathOperations) []string {
	allowed := map[string]bool{"get": true, "post": true, "put": true, "patch": true, "delete": true, "head": true, "options": true}
	methods := []string{}
	for method := range values {
		method = strings.ToLower(strings.TrimSpace(method))
		if allowed[method] {
			methods = append(methods, method)
		}
	}
	sort.Strings(methods)
	return methods
}

func operationSlug(method string, path string, op operation) string {
	if strings.TrimSpace(op.OperationID) != "" {
		return profiledraft.Slug(op.OperationID)
	}
	return profiledraft.Slug(strings.ToLower(method) + "-" + path)
}
