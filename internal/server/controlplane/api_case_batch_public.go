package controlplane

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

const maxPublicAPICaseBatchTimeoutSeconds = 600
const defaultAPICaseBatchTimeoutSeconds = 90

type publicAPICaseBatchPayloadError struct {
	code    string
	message string
}

func (e publicAPICaseBatchPayloadError) Error() string {
	return e.message
}

func validatePublicAPICaseBatchRunPayload(payload map[string]any) error {
	if err := validatePublicAPICaseBatchRunFields(payload, []string{"baseUrl", "environmentId", apiFieldEvidenceDir}); err != nil {
		return err
	}
	value, present := payload[apiFieldTimeoutSeconds]
	return validateAPICaseBatchTimeoutSeconds(value, present)
}

func readPublicAPICaseBatchRunPayload(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	payload, err := readJSONPayload(r)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return nil, false
	}
	if writePublicAPICaseBatchPayloadError(w, validatePublicAPICaseBatchRunPayload(payload)) {
		return nil, false
	}
	return payload, true
}

func validatePublicEnvironmentAcceptanceRunPayload(payload map[string]any) error {
	if err := validatePublicAPICaseBatchRunFields(payload, []string{"baseUrl", apiFieldEvidenceDir}); err != nil {
		return err
	}
	value, present := payload[apiFieldTimeoutSeconds]
	return validateAPICaseBatchTimeoutSeconds(value, present)
}

func validatePublicAPICaseBatchRunFields(payload map[string]any, fields []string) error {
	rejected := make([]string, 0, len(fields))
	for _, field := range fields {
		if _, ok := payload[field]; ok {
			rejected = append(rejected, field)
		}
	}
	if len(rejected) == 0 {
		return nil
	}
	return publicAPICaseBatchPayloadError{
		code:    "trusted_execution_context_rejected",
		message: "public batch execution cannot set trusted fields: " + strings.Join(rejected, ", ") + "; configure targets and Evidence paths in the Store catalog",
	}
}

func validateAPICaseBatchTimeoutSeconds(value any, present bool) error {
	if !present || value == nil {
		return nil
	}
	seconds, ok := apiCaseBatchTimeoutSeconds(value)
	if ok && seconds >= 0 && seconds <= maxPublicAPICaseBatchTimeoutSeconds {
		return nil
	}
	return publicAPICaseBatchPayloadError{
		code:    "invalid_timeout_seconds",
		message: "timeoutSeconds must be an integer between 0 and 600",
	}
}

func normalizeAPICaseBatchPlanTimeouts(plans []apiCaseBatchCasePlan) error {
	for index := range plans {
		timeoutSeconds, err := normalizedAPICaseBatchTimeoutSeconds(plans[index].TimeoutSeconds)
		if err != nil {
			return err
		}
		plans[index].TimeoutSeconds = timeoutSeconds
	}
	return nil
}

func normalizedAPICaseBatchTimeoutSeconds(timeoutSeconds int) (int, error) {
	if timeoutSeconds == 0 {
		return defaultAPICaseBatchTimeoutSeconds, nil
	}
	if err := validateAPICaseBatchTimeoutSeconds(timeoutSeconds, true); err != nil {
		return 0, err
	}
	return timeoutSeconds, nil
}

func safeAPICaseBatchExecutionTimeoutSeconds(timeoutSeconds int) int {
	normalized, err := normalizedAPICaseBatchTimeoutSeconds(timeoutSeconds)
	if err != nil {
		return defaultAPICaseBatchTimeoutSeconds
	}
	return normalized
}

func apiCaseBatchTimeoutSeconds(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case json.Number:
		out, err := typed.Int64()
		return out, err == nil
	default:
		return 0, false
	}
}

func writePublicAPICaseBatchPayloadError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	payloadErr := publicAPICaseBatchPayloadError{
		code:    "invalid_batch_request",
		message: err.Error(),
	}
	_ = errors.As(err, &payloadErr)
	writeJSONStatus(w, http.StatusBadRequest, map[string]any{
		"ok":         false,
		"error":      payloadErr.message,
		apiFieldCode: payloadErr.code,
	})
	return true
}

func writeAPICaseBatchStartError(w http.ResponseWriter, status int, err error, details map[string]any) bool {
	if err == nil {
		return false
	}
	if status == http.StatusBadRequest && writePublicAPICaseBatchPayloadError(w, err) {
		return true
	}
	payload := map[string]any{"ok": false, "error": err.Error()}
	for key, value := range details {
		payload[key] = value
	}
	writeJSONStatus(w, status, payload)
	return true
}
