package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/runner/apicase"
	"agent-testbench/internal/store"
)

type caseExecutionResult struct {
	ok            bool
	httpCode      int
	httpStatus    int
	baseURL       string
	failureReason string
	runID         string
	result        map[string]any
}

const testKitAPICaseRunResultKey = "agentTestBenchAPICaseRunResult"

func executeTestKitCase(ctx context.Context, bundle profile.Bundle, runtime store.Store, item runnableAPICase, payload map[string]any) caseExecutionResult {
	// This value is an in-process handoff from the file runner to the Store
	// recorder. Always clear caller input before deciding how this case runs.
	delete(payload, testKitAPICaseRunResultKey)
	overrides := mergeStringAnyMaps(item.Case.DefaultOverrides, mapFromAny(payload["overrides"]))
	if len(overrides) > 0 {
		payload["overrides"] = overrides
	}
	if missing := missingRequiredCaseInputs(item.Inputs, overrides); len(missing) > 0 {
		return failedCaseExecution(item.Case.ID, "missing required case input: "+strings.Join(missing, ", "))
	}
	timeout, err := testKitTimeout(payload, item.Case.TimeoutSeconds)
	if err != nil {
		result := failedCaseExecution(item.Case.ID, err.Error())
		result.httpStatus = http.StatusBadRequest
		return result
	}
	if strings.TrimSpace(item.Case.CasePath) != "" {
		return executeTestKitFileCase(ctx, item, payload, overrides, timeout)
	}
	return executeInlineTestKitCase(ctx, bundle, runtime, item, payload, timeout)
}

func executeInlineTestKitCase(ctx context.Context, bundle profile.Bundle, runtime store.Store, item runnableAPICase, payload map[string]any, timeout time.Duration) caseExecutionResult {
	if item.Execution == nil {
		if externalCaseSourceConfigured(item.Case) {
			return unsupportedExternalCaseExecution(item.Case)
		}
		return failedCaseExecution(item.Case.ID, "api case execution adapter is not configured")
	}
	request, err := buildCaseHTTPRequest(ctx, bundle, runtime, *item.Execution, item.CaseBaseURL, payload)
	if err != nil {
		return failedCaseExecution(item.Case.ID, err.Error())
	}
	if err := applyAPICaseRequestModel(&request, item.Case); err != nil {
		return failedCaseExecution(item.Case.ID, err.Error())
	}
	if request.requiresBody() && request.body == nil {
		return failedCaseExecution(item.Case.ID, fmt.Sprintf("%s caseExecution.body is required for %s; add caseExecution.body or a request template that renders a body", request.method, item.Case.ID))
	}
	httpRequest, err := newTestKitHTTPRequest(ctx, request)
	if err != nil {
		return failedCaseExecution(item.Case.ID, err.Error())
	}
	return executeTestKitHTTPRequest(item.Case.ID, request, httpRequest, timeout)
}

func newTestKitHTTPRequest(ctx context.Context, request caseHTTPRequest) (*http.Request, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, request.method, request.fullURL, request.bodyReader())
	if err != nil {
		return nil, err
	}
	for key, value := range request.headers {
		httpRequest.Header.Set(key, value)
	}
	if _, ok := request.headers["Content-Type"]; !ok && request.body != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	return httpRequest, nil
}

func executeTestKitHTTPRequest(caseID string, request caseHTTPRequest, httpRequest *http.Request, timeout time.Duration) caseExecutionResult {
	started := time.Now()
	client := http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(httpRequest)
	if err != nil {
		return failedCaseExecution(caseID, err.Error())
	}
	responseBody, err := readTestKitResponseBody(response)
	if err != nil {
		return failedCaseExecution(caseID, err.Error())
	}
	responseSummary := map[string]any{
		"statusCode": response.StatusCode,
		"headers":    responseHeaders(response.Header),
		"body":       string(responseBody),
		"elapsedMs":  time.Since(started).Milliseconds(),
	}
	passed, failureReason := evaluateTestKitResponse(response.StatusCode, string(responseBody), request)
	return caseExecutionResult{
		ok:            passed,
		httpCode:      response.StatusCode,
		baseURL:       request.baseURL,
		failureReason: failureReason,
		result: map[string]any{
			"request":  request.summary(),
			"response": responseSummary,
		},
	}
}

func readTestKitResponseBody(response *http.Response) ([]byte, error) {
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, apicase.MaxResponseBodyBytes+1))
	closeErr := response.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(responseBody)) > apicase.MaxResponseBodyBytes {
		return nil, fmt.Errorf("response body exceeds %d byte limit", apicase.MaxResponseBodyBytes)
	}
	return responseBody, nil
}

func evaluateTestKitResponse(statusCode int, responseBody string, request caseHTTPRequest) (bool, string) {
	if !expectedHTTPCode(statusCode, request.expectedHTTPCodes) {
		return false, fmt.Sprintf("unexpected http status %d", statusCode)
	}
	for _, expected := range request.expectedResponse {
		expected = strings.TrimSpace(expected)
		if expected != "" && !strings.Contains(responseBody, expected) {
			return false, fmt.Sprintf("response body missing %q", expected)
		}
	}
	for _, forbidden := range request.forbiddenResponse {
		forbidden = strings.TrimSpace(forbidden)
		if forbidden != "" && strings.Contains(responseBody, forbidden) {
			return false, fmt.Sprintf("response body must not contain %q", forbidden)
		}
	}
	return true, ""
}

func executeTestKitFileCase(ctx context.Context, item runnableAPICase, payload map[string]any, overrides map[string]any, timeout time.Duration) caseExecutionResult {
	evidenceDir := firstNonEmpty(valueString(payload[apiFieldEvidenceDir]), item.Case.EvidenceDir, filepath.Join(".runtime", "cases"))
	baseURL := firstNonEmpty(valueString(payload["baseUrl"]), item.CaseBaseURL, item.Case.BaseURL)
	executionCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	runID := firstNonEmpty(valueString(payload["runId"]), nextTestKitRunID(time.Now().UTC()))
	result, err := apicase.Run(executionCtx, apicase.RunOptions{
		CasePath:    item.Case.CasePath,
		EvidenceDir: evidenceDir,
		RunID:       runID,
		BaseURL:     baseURL,
		Overrides:   overrides,
	})
	if err != nil {
		return failedCaseExecution(item.Case.ID, err.Error())
	}
	payload[testKitAPICaseRunResultKey] = result
	payload["runId"] = result.RunID
	payload[apiFieldEvidenceDir] = evidenceDir
	request, _ := jsonFileObject(filepath.Join(result.EvidencePath, apiCaseEvidenceFileRequest))
	response, _ := jsonFileObject(filepath.Join(result.EvidencePath, apiCaseEvidenceFileResponse))
	if request == nil {
		request = map[string]any{"caseId": item.Case.ID}
	}
	if response == nil {
		response = map[string]any{"body": "{}"}
	}
	failureReason := strings.TrimSpace(result.Error)
	if result.Status != store.StatusPassed && failureReason == "" {
		failureReason = "api case assertions failed"
	}
	return caseExecutionResult{
		ok:            result.Status == store.StatusPassed,
		httpCode:      intValue(response["statusCode"]),
		baseURL:       baseURL,
		failureReason: failureReason,
		runID:         result.RunID,
		result: map[string]any{
			"request":  request,
			"response": response,
		},
	}
}

func externalCaseSourceConfigured(item profile.APICase) bool {
	return strings.TrimSpace(item.SourceKind) != "" || strings.TrimSpace(item.SourcePath) != "" || strings.TrimSpace(item.ExecutorID) != ""
}

func unsupportedExternalCaseExecution(item profile.APICase) caseExecutionResult {
	reason := fmt.Sprintf(
		"external executor execution is planning-only for source kind %q and executor %q",
		strings.TrimSpace(item.SourceKind),
		strings.TrimSpace(item.ExecutorID),
	)
	result := failedCaseExecution(item.ID, reason)
	result.httpStatus = http.StatusNotImplemented
	return result
}

func missingRequiredCaseInputs(inputs []map[string]any, overrides map[string]any) []string {
	missing := []string{}
	for _, input := range inputs {
		name := valueString(input["name"])
		if name == "" || !caseInputRequired(input) {
			continue
		}
		value, ok := overrides[name]
		if !ok || value == nil {
			missing = append(missing, name)
		}
	}
	return missing
}

func caseInputRequired(input map[string]any) bool {
	raw, ok := input["required"]
	if !ok {
		return true
	}
	return boolValue(raw)
}

type caseHTTPRequest struct {
	method            string
	baseURL           string
	fullURL           string
	path              string
	headers           map[string]string
	auth              map[string]string
	body              any
	expectedHTTPCodes []int
	expectedResponse  []string
	forbiddenResponse []string
	nodeID            string
	signed            bool
}

func (request caseHTTPRequest) requiresBody() bool {
	switch strings.ToUpper(strings.TrimSpace(request.method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

func buildCaseHTTPRequest(ctx context.Context, bundle profile.Bundle, runtime store.Store, execution caseExecutionConfig, caseBaseURL string, payload map[string]any) (caseHTTPRequest, error) {
	baseURL := strings.TrimRight(valueString(payload["baseUrl"]), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(caseBaseURL, "/")
	}
	if baseURL == "" {
		baseURL = catalogServiceBaseURL(ctx, runtime, execution.NodeID)
	}
	if baseURL == "" {
		baseURL = serviceBaseURL(ctx, bundle.Services, execution.NodeID)
	}
	if baseURL == "" {
		return caseHTTPRequest{}, fmt.Errorf("service runtime is not available for %s", execution.NodeID)
	}
	rendered := renderCaseExecution(execution, mapFromAny(payload["overrides"]))
	path := strings.TrimSpace(rendered.Path)
	if path == "" {
		path = "/"
	}
	fullURL, err := joinCaseURL(baseURL, path, rendered.Query)
	if err != nil {
		return caseHTTPRequest{}, err
	}
	request := caseHTTPRequest{
		method:            strings.ToUpper(firstNonEmpty(rendered.Method, "GET")),
		baseURL:           baseURL,
		fullURL:           fullURL,
		path:              path,
		headers:           headerStrings(rendered.Headers),
		auth:              headerStrings(rendered.Auth),
		body:              rendered.Body,
		expectedHTTPCodes: rendered.ExpectedHTTPCodes,
		expectedResponse:  rendered.ExpectedResponse,
		forbiddenResponse: rendered.ExpectedResponseAbsent,
		nodeID:            rendered.NodeID,
		signed:            rendered.Signed,
	}
	request.ensureForwardingHeaders()
	if request.signed {
		if err := request.applySigning(); err != nil {
			return caseHTTPRequest{}, err
		}
	}
	return request, nil
}

func serviceBaseURL(ctx context.Context, services []profile.Service, serviceID string) string {
	runtime := dockerRuntimeByService(ctx, services)[serviceID]
	if runtime.Port == 0 {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d", runtime.Port)
}

func catalogServiceBaseURL(ctx context.Context, runtime store.Store, serviceID string) string {
	if runtime == nil || strings.TrimSpace(serviceID) == "" {
		return ""
	}
	catalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil {
		return ""
	}
	resolvedServiceID := strings.TrimSpace(serviceID)
	for _, node := range catalog.InterfaceNodes {
		if node.ID == serviceID && strings.TrimSpace(node.ServiceID) != "" {
			resolvedServiceID = strings.TrimSpace(node.ServiceID)
			break
		}
	}
	for _, service := range catalog.Services {
		if service.ID != resolvedServiceID {
			continue
		}
		port := service.ServicePort
		if port <= 0 {
			return ""
		}
		return fmt.Sprintf("http://127.0.0.1:%d", port)
	}
	return ""
}

func (request caseHTTPRequest) bodyReader() io.Reader {
	if request.body == nil {
		return nil
	}
	raw, err := json.Marshal(request.body)
	if err != nil {
		raw = []byte("{}")
	}
	return bytes.NewReader(raw)
}

func (request caseHTTPRequest) summary() map[string]any {
	return map[string]any{
		"method":            request.method,
		"baseUrl":           request.baseURL,
		"fullUrl":           request.fullURL,
		"path":              request.path,
		"nodeId":            request.nodeID,
		"headers":           request.headers,
		"body":              request.body,
		"expectedHttpCodes": request.expectedHTTPCodes,
		"signed":            request.signed,
	}
}

func (request *caseHTTPRequest) ensureForwardingHeaders() {
	if request.headers == nil {
		request.headers = map[string]string{}
	}
	if _, ok := request.headers["X-Forwarded-For"]; !ok {
		request.headers["X-Forwarded-For"] = "192.168.1.100"
	}
	if _, ok := request.headers["X-Real-IP"]; !ok {
		request.headers["X-Real-IP"] = "192.168.1.100"
	}
}

func (request *caseHTTPRequest) applySigning() error {
	uri, err := request.requestURI()
	if err != nil {
		return err
	}
	body := ""
	if request.body != nil {
		raw, err := json.Marshal(request.body)
		if err != nil {
			return err
		}
		body = string(raw)
	}
	auth, err := requestSigningAuthorization(request.method, uri, body, request.auth)
	if err != nil {
		return err
	}
	request.headers["Authorization"] = auth
	return nil
}

func (request caseHTTPRequest) requestURI() (string, error) {
	parsed, err := url.Parse(request.fullURL)
	if err != nil {
		return "", err
	}
	if parsed.RequestURI() == "" {
		return "/", nil
	}
	return parsed.RequestURI(), nil
}
