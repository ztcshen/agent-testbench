// Package apicase loads and executes HTTP API cases while recording bounded
// local Evidence for requests, responses, assertions, and run outcomes.
package apicase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agent-testbench/internal/domain/apicasespec"
)

type Case = apicasespec.Case
type Request = apicasespec.Request
type Assertions = apicasespec.Assertions

type RunOptions struct {
	CasePath            string
	EvidenceDir         string
	RunID               string
	BaseURL             string
	Overrides           map[string]any
	BeforeEvidenceWrite func(context.Context) error
}

type RunResult struct {
	OK              bool   `json:"ok"`
	RunID           string `json:"runId"`
	CaseID          string `json:"caseId"`
	Status          string `json:"status"`
	FailurePhase    string `json:"failurePhase,omitempty"`
	FailureCategory string `json:"failureCategory,omitempty"`
	Error           string `json:"error,omitempty"`
	EvidencePath    string `json:"evidencePath"`
	StartedAt       string `json:"startedAt"`
	FinishedAt      string `json:"finishedAt"`
	ElapsedMs       int64  `json:"elapsedMs"`
	CreatedAt       string `json:"createdAt"`
}

type ErrorEvidence struct {
	Status   string `json:"status"`
	Phase    string `json:"phase"`
	Category string `json:"category"`
	Message  string `json:"message"`
}

// MaxResponseBodyBytes bounds the response body retained as local Evidence.
const MaxResponseBodyBytes int64 = 1 << 20

type executionError struct {
	phase    string
	category string
	err      error
}

func (e *executionError) Error() string { return e.err.Error() }
func (e *executionError) Unwrap() error { return e.err }

type DryRunPlan struct {
	OK         bool                `json:"ok"`
	DryRun     bool                `json:"dryRun"`
	RunID      string              `json:"runId"`
	CaseID     string              `json:"caseId"`
	Title      string              `json:"title,omitempty"`
	Request    DryRunRequestPlan   `json:"request"`
	Assertions DryRunAssertionPlan `json:"assertions"`
	Effects    DryRunEffectsPlan   `json:"effects"`
	Warnings   []string            `json:"warnings"`
}

type DryRunRequestPlan struct {
	Method     string   `json:"method"`
	Path       string   `json:"path"`
	URL        string   `json:"url,omitempty"`
	HeaderKeys []string `json:"headerKeys"`
	HasBody    bool     `json:"hasBody"`
	BodyKeys   []string `json:"bodyKeys"`
}

type DryRunAssertionPlan struct {
	ExpectedStatusCodes      []int `json:"expectedStatusCodes"`
	ResponseContainsCount    int   `json:"responseContainsCount"`
	ResponseNotContainsCount int   `json:"responseNotContainsCount"`
}

type DryRunEffectsPlan struct {
	HTTPRequest         bool   `json:"httpRequest"`
	WritesEvidence      bool   `json:"writesEvidence"`
	WritesStore         bool   `json:"writesStore"`
	PlannedEvidencePath string `json:"plannedEvidencePath"`
}

func Plan(options RunOptions) (DryRunPlan, error) {
	item, err := Load(options.CasePath)
	if err != nil {
		return DryRunPlan{}, err
	}
	applyOverrides(&item, options.Overrides)

	warnings := []string{}
	endpoint := ""
	if strings.TrimSpace(options.BaseURL) == "" {
		warnings = append(warnings, "base url is not set; live runs need --base-url")
	} else {
		endpoint, err = buildURL(options.BaseURL, item.Request.Path)
		if err != nil {
			return DryRunPlan{}, err
		}
	}
	runID := plannedCaseRunID(options.RunID)

	return DryRunPlan{
		OK:     true,
		DryRun: true,
		RunID:  runID,
		CaseID: item.ID,
		Title:  item.Title,
		Request: DryRunRequestPlan{
			Method:     strings.ToUpper(strings.TrimSpace(item.Request.Method)),
			Path:       item.Request.Path,
			URL:        endpoint,
			HeaderKeys: sortedStringMapKeys(item.Request.Headers),
			HasBody:    item.Request.Body != nil,
			BodyKeys:   sortedAnyMapKeys(item.Request.Body),
		},
		Assertions: DryRunAssertionPlan{
			ExpectedStatusCodes:      append([]int(nil), item.Assertions.ExpectedStatusCodes...),
			ResponseContainsCount:    len(item.Assertions.ResponseContains),
			ResponseNotContainsCount: len(item.Assertions.ResponseNotContains),
		},
		Effects: DryRunEffectsPlan{
			HTTPRequest:         false,
			WritesEvidence:      false,
			WritesStore:         false,
			PlannedEvidencePath: caseRunEvidencePath(options.EvidenceDir, runID),
		},
		Warnings: warnings,
	}, nil
}

func Run(ctx context.Context, options RunOptions) (RunResult, error) {
	started := time.Now().UTC()
	if err := validateExplicitRunID(options.RunID); err != nil {
		return RunResult{}, err
	}
	item, err := Load(options.CasePath)
	if err != nil {
		return RunResult{}, err
	}
	applyOverrides(&item, options.Overrides)
	runID := plannedCaseRunID(options.RunID)
	evidencePath := caseRunEvidencePath(options.EvidenceDir, runID)
	result := RunResult{
		OK:           true,
		RunID:        runID,
		CaseID:       item.ID,
		Status:       "passed",
		EvidencePath: evidencePath,
		StartedAt:    started.Format(time.RFC3339Nano),
		CreatedAt:    started.Format(time.RFC3339Nano),
	}
	if err := beforeEvidenceWrite(ctx, options.BeforeEvidenceWrite); err != nil {
		return evidencePersistenceFailure(started, result, err)
	}
	if err := os.MkdirAll(evidencePath, 0o755); err != nil {
		return evidencePersistenceFailure(started, result, fmt.Errorf("create evidence directory: %w", err))
	}
	if err := beforeEvidenceWrite(ctx, options.BeforeEvidenceWrite); err != nil {
		return evidencePersistenceFailure(started, result, err)
	}
	if err := writeJSON(filepath.Join(evidencePath, "case.json"), item); err != nil {
		return evidencePersistenceFailure(started, result, err)
	}
	if err := beforeEvidenceWrite(ctx, options.BeforeEvidenceWrite); err != nil {
		return evidencePersistenceFailure(started, result, err)
	}
	if err := writeJSON(filepath.Join(evidencePath, "request.json"), item.Request); err != nil {
		return evidencePersistenceFailure(started, result, err)
	}
	response, assertions, err := executeHTTP(ctx, item, options.BaseURL)
	if err != nil {
		phase, category := executionFailureDetails(err)
		result.OK = false
		result.Status = "failed"
		result.FailurePhase = phase
		result.FailureCategory = category
		result.Error = err.Error()
		return writeFailureOutcome(ctx, started, result, options.BeforeEvidenceWrite)
	}
	if assertions.Status != "passed" {
		result.OK = false
		result.Status = "failed"
		result.FailurePhase = "assertion"
		result.FailureCategory = "assertion-mismatch"
		result.Error = strings.Join(assertions.Errors, "; ")
	}
	if err := beforeEvidenceWrite(ctx, options.BeforeEvidenceWrite); err != nil {
		return evidencePersistenceFailure(started, result, err)
	}
	if err := writeJSON(filepath.Join(evidencePath, "response.json"), response); err != nil {
		return evidencePersistenceFailure(started, result, err)
	}
	if err := beforeEvidenceWrite(ctx, options.BeforeEvidenceWrite); err != nil {
		return evidencePersistenceFailure(started, result, err)
	}
	if err := writeJSON(filepath.Join(evidencePath, "assertions.json"), assertions); err != nil {
		return evidencePersistenceFailure(started, result, err)
	}
	return finishRun(ctx, started, result, options.BeforeEvidenceWrite)
}

func finishRun(ctx context.Context, started time.Time, result RunResult, beforeWrite func(context.Context) error) (RunResult, error) {
	finished := time.Now().UTC()
	result.FinishedAt = finished.Format(time.RFC3339Nano)
	result.ElapsedMs = finished.Sub(started).Milliseconds()
	if err := beforeEvidenceWrite(ctx, beforeWrite); err != nil {
		return evidencePersistenceFailure(started, result, err)
	}
	if err := writeJSON(filepath.Join(result.EvidencePath, "summary.json"), result); err != nil {
		return evidencePersistenceFailure(started, result, err)
	}
	return result, nil
}

func executionFailureDetails(err error) (string, string) {
	var runErr *executionError
	if errors.As(err, &runErr) {
		return runErr.phase, runErr.category
	}
	return "execution", executionFailureCategory(err)
}

func executionFailureCategory(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	return "transport-error"
}

func wrapExecutionError(phase string, category string, err error) error {
	if category == "" {
		category = executionFailureCategory(err)
	}
	return &executionError{phase: phase, category: category, err: err}
}

func applyOverrides(item *Case, overrides map[string]any) {
	if len(overrides) == 0 {
		return
	}
	if item.Request.Body == nil {
		item.Request.Body = map[string]any{}
	}
	for key, value := range overrides {
		item.Request.Body[key] = value
	}
}

func Load(path string) (Case, error) {
	if strings.TrimSpace(path) == "" {
		return Case{}, errors.New("case path is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Case{}, fmt.Errorf("read api case: %w", err)
	}
	var item Case
	if err := json.Unmarshal(raw, &item); err != nil {
		return Case{}, fmt.Errorf("decode api case: %w", err)
	}
	if err := validate(item); err != nil {
		return Case{}, err
	}
	return item, nil
}

func validate(item Case) error {
	if strings.TrimSpace(item.ID) == "" {
		return errors.New("case id is required")
	}
	if strings.TrimSpace(item.Request.Method) == "" {
		return errors.New("request method is required")
	}
	if strings.TrimSpace(item.Request.Path) == "" {
		return errors.New("request path is required")
	}
	return nil
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func plannedCaseRunID(runID string) string {
	value := strings.TrimSpace(runID)
	if value != "" {
		return value
	}
	return "case-run-" + time.Now().UTC().Format("20060102T150405")
}

func validateExplicitRunID(runID string) error {
	value := strings.TrimSpace(runID)
	if value == "" {
		return nil
	}
	if value == "." || value == ".." || filepath.IsAbs(value) {
		return fmt.Errorf("run id %q must be a single path segment", runID)
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			continue
		}
		switch char {
		case '.', '-', '_':
			continue
		default:
			return fmt.Errorf("run id %q must be a single path segment using ASCII letters, digits, dot, dash, or underscore", runID)
		}
	}
	return nil
}

func caseRunEvidencePath(root string, runID string) string {
	if strings.TrimSpace(root) == "" {
		root = filepath.Join(".runtime", "cases")
	}
	return filepath.Join(root, plannedCaseRunID(runID))
}

func sortedStringMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedAnyMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type ResponseEvidence struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

type AssertionEvidence struct {
	Status string   `json:"status"`
	Errors []string `json:"errors,omitempty"`
}

func executeHTTP(ctx context.Context, item Case, baseURL string) (ResponseEvidence, AssertionEvidence, error) {
	endpoint, err := buildURL(baseURL, item.Request.Path)
	if err != nil {
		return ResponseEvidence{}, AssertionEvidence{}, wrapExecutionError("request-materialization", "configuration-error", err)
	}
	var body io.Reader
	if item.Request.Body != nil {
		raw, err := json.Marshal(item.Request.Body)
		if err != nil {
			return ResponseEvidence{}, AssertionEvidence{}, wrapExecutionError("request-materialization", "request-materialization", fmt.Errorf("encode request body: %w", err))
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(item.Request.Method), endpoint, body)
	if err != nil {
		return ResponseEvidence{}, AssertionEvidence{}, wrapExecutionError("request-materialization", "request-materialization", fmt.Errorf("create request: %w", err))
	}
	for key, value := range item.Request.Headers {
		req.Header.Set(key, value)
	}
	if item.Request.Body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		return ResponseEvidence{}, AssertionEvidence{}, wrapExecutionError("request-send", "", fmt.Errorf("send request: %w", err))
	}
	defer resp.Body.Close()
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodyBytes+1))
	if err != nil {
		return ResponseEvidence{}, AssertionEvidence{}, wrapExecutionError("response-read", "transport-error", fmt.Errorf("read response: %w", err))
	}
	if int64(len(rawBody)) > MaxResponseBodyBytes {
		return ResponseEvidence{}, AssertionEvidence{}, wrapExecutionError(
			"response-read",
			"response-too-large",
			fmt.Errorf("response body exceeds %d byte limit", MaxResponseBodyBytes),
		)
	}
	response := ResponseEvidence{
		StatusCode: resp.StatusCode,
		Headers:    map[string]string{},
		Body:       string(rawBody),
	}
	for key, values := range resp.Header {
		response.Headers[key] = strings.Join(values, ", ")
	}
	return response, assertResponse(item.Assertions, response), nil
}

func buildURL(baseURL string, path string) (string, error) {
	if strings.TrimSpace(baseURL) == "" {
		return "", errors.New("base url is required for live api case runs")
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}
	relative, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("parse request path: %w", err)
	}
	if relative.IsAbs() || relative.Host != "" || relative.Opaque != "" {
		return "", errors.New("api case request path must not override the configured target origin")
	}
	return base.ResolveReference(relative).String(), nil
}

func assertResponse(assertions Assertions, response ResponseEvidence) AssertionEvidence {
	var failures []string
	if len(assertions.ExpectedStatusCodes) > 0 {
		found := false
		for _, code := range assertions.ExpectedStatusCodes {
			if response.StatusCode == code {
				found = true
				break
			}
		}
		if !found {
			failures = append(failures, fmt.Sprintf("status code %d was not expected", response.StatusCode))
		}
	}
	for _, fragment := range assertions.ResponseContains {
		if !strings.Contains(response.Body, fragment) {
			failures = append(failures, fmt.Sprintf("response did not contain %q", fragment))
		}
	}
	for _, fragment := range assertions.ResponseNotContains {
		if strings.TrimSpace(fragment) == "" {
			continue
		}
		if strings.Contains(response.Body, fragment) {
			failures = append(failures, fmt.Sprintf("response must not contain %q", fragment))
		}
	}
	if len(failures) > 0 {
		return AssertionEvidence{Status: "failed", Errors: failures}
	}
	return AssertionEvidence{Status: "passed"}
}
