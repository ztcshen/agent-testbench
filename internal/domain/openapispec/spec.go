// Package openapispec contains the lightweight OpenAPI document model used by profile planners.
package openapispec

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"agent-testbench/internal/domain/profiledraft"
)

type Document struct {
	OpenAPI string                    `json:"openapi"`
	Info    Info                      `json:"info"`
	Paths   map[string]PathOperations `json:"paths"`
}

type Info struct {
	Title string `json:"title"`
}

type PathOperations map[string]Operation

type Operation struct {
	OperationID string              `json:"operationId"`
	Summary     string              `json:"summary"`
	Description string              `json:"description"`
	Tags        []string            `json:"tags"`
	RequestBody RequestBody         `json:"requestBody"`
	Responses   map[string]Response `json:"responses"`
}

type RequestBody struct {
	Content map[string]MediaType `json:"content"`
}

type MediaType struct {
	Example any    `json:"example"`
	Schema  Schema `json:"schema"`
}

type Schema struct {
	Type       string                    `json:"type"`
	Required   []string                  `json:"required"`
	Properties map[string]PropertySchema `json:"properties"`
}

type PropertySchema struct {
	Type    string `json:"type"`
	Example any    `json:"example"`
}

type Response struct {
	Description string `json:"description"`
}

type OperationRef struct {
	Method    string
	Path      string
	Operation Operation
}

func Decode(raw []byte) (Document, error) {
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Document{}, fmt.Errorf("decode openapi document: %w", err)
	}
	if strings.TrimSpace(doc.OpenAPI) == "" {
		return Document{}, errors.New("openapi version is required")
	}
	if len(doc.Paths) == 0 {
		return Document{}, errors.New("openapi paths are required")
	}
	return doc, nil
}

func (d Document) Title() string {
	title := strings.TrimSpace(d.Info.Title)
	if title == "" {
		return "OpenAPI Service"
	}
	return title
}

func ServiceID(explicit string, title string) string {
	serviceID := strings.TrimSpace(explicit)
	if serviceID != "" {
		return serviceID
	}
	return "service." + profiledraft.Slug(title)
}

func (d Document) Operations() []OperationRef {
	return d.operationRefs(SortedHTTPMethods)
}

func (d Document) AlphabeticalOperations() []OperationRef {
	return d.operationRefs(AlphabeticalHTTPMethods)
}

func (d Document) operationRefs(methodsFor func(PathOperations) []string) []OperationRef {
	operations := []OperationRef{}
	for _, path := range profiledraft.SortedKeys(d.Paths) {
		pathOperations := d.Paths[path]
		for _, method := range methodsFor(pathOperations) {
			operations = append(operations, OperationRef{
				Method:    method,
				Path:      path,
				Operation: pathOperations[method],
			})
		}
	}
	return operations
}

func (ref OperationRef) DisplayName() string {
	return profiledraft.FirstNonEmpty(ref.Operation.Summary, ref.Operation.OperationID, strings.ToUpper(ref.Method)+" "+ref.Path)
}

func (ref OperationRef) OperationName() string {
	return profiledraft.FirstNonEmpty(ref.Operation.OperationID, ref.DisplayName())
}

func (ref OperationRef) Slug() string {
	if strings.TrimSpace(ref.Operation.OperationID) != "" {
		return profiledraft.Slug(ref.Operation.OperationID)
	}
	return profiledraft.Slug(strings.ToLower(ref.Method) + "-" + ref.Path)
}

func SortedHTTPMethods(values PathOperations) []string {
	methods := HTTPMethods(values)
	sort.SliceStable(methods, func(i, j int) bool {
		return methodRank(methods[i]) < methodRank(methods[j])
	})
	return methods
}

func AlphabeticalHTTPMethods(values PathOperations) []string {
	methods := HTTPMethods(values)
	sort.Strings(methods)
	return methods
}

func HTTPMethods(values PathOperations) []string {
	methods := []string{}
	for method := range values {
		method = strings.ToLower(strings.TrimSpace(method))
		if allowedHTTPMethod(method) {
			methods = append(methods, method)
		}
	}
	return methods
}

func allowedHTTPMethod(method string) bool {
	switch method {
	case "get", "post", "put", "patch", "delete", "head", "options":
		return true
	default:
		return false
	}
}

func JSONRequestSchema(op Operation) (Schema, bool) {
	if op.RequestBody.Content == nil {
		return Schema{}, false
	}
	media, ok := op.RequestBody.Content["application/json"]
	if !ok {
		return Schema{}, false
	}
	return media.Schema, len(media.Schema.Properties) > 0
}

func FirstClientErrorStatus(responses map[string]Response) int {
	codes := []int{}
	for code := range responses {
		if len(code) != 3 || !strings.HasPrefix(code, "4") {
			continue
		}
		parsed, err := strconv.Atoi(code)
		if err == nil {
			codes = append(codes, parsed)
		}
	}
	sort.Ints(codes)
	if len(codes) == 0 {
		return 0
	}
	return codes[0]
}

func FirstSuccessStatus(responses map[string]Response) int {
	codes := []int{}
	for code := range responses {
		parsed, err := strconv.Atoi(code)
		if err == nil && parsed >= 200 && parsed < 300 {
			codes = append(codes, parsed)
		}
	}
	sort.Ints(codes)
	if len(codes) == 0 {
		return 0
	}
	return codes[0]
}

func methodRank(method string) int {
	order := map[string]int{"get": 10, "post": 20, "put": 30, "patch": 40, "delete": 50, "head": 60, "options": 70}
	if rank, ok := order[method]; ok {
		return rank
	}
	return 100
}
