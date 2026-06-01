package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

type notifyResult struct {
	OK      bool   `json:"ok"`
	Channel string `json:"channel"`
	Target  string `json:"target"`
	Error   string `json:"error,omitempty"`
}

type taskNotificationOptions struct {
	File    string
	Webhook string
}

type notificationEvent struct {
	TaskID    string `json:"taskId,omitempty"`
	TaskName  string `json:"taskName,omitempty"`
	RunID     string `json:"runId,omitempty"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	CreatedAt string `json:"createdAt"`
}

const notificationWebhookTimeout = 10 * time.Second

var notificationHTTPClient = &http.Client{Timeout: notificationWebhookTimeout}

func runNotify(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing notify command")
	}
	switch args[0] {
	case "test":
		return runNotifyTest(ctx, args[1:])
	default:
		return fmt.Errorf("unknown notify command: %s", args[0])
	}
}

func runNotifyTest(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("notify test", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	file := flags.String("file", "", "Append test notification to a JSONL file")
	webhook := flags.String("webhook", "", "POST test notification to a webhook")
	message := flags.String("message", "AgentTestBench notification test", "Notification message")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable notification report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	results := sendNotificationEvent(ctx, taskNotificationOptions{File: *file, Webhook: *webhook}, notificationEvent{
		Status:    "test",
		Message:   *message,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	report := map[string]any{"ok": notificationResultsOK(results), "notify": results}
	if len(results) == 1 {
		report["channel"] = results[0].Channel
		report["target"] = results[0].Target
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	for _, result := range results {
		fmt.Printf("Notify %s %s ok=%t\n", result.Channel, result.Target, result.OK)
	}
	if !notificationResultsOK(results) {
		return errors.New("notification test failed")
	}
	return nil
}

func notifyJSON(opts taskNotificationOptions) string {
	payload := map[string]string{}
	if strings.TrimSpace(opts.File) != "" {
		payload["file"] = strings.TrimSpace(opts.File)
	}
	if strings.TrimSpace(opts.Webhook) != "" {
		payload["webhook"] = strings.TrimSpace(opts.Webhook)
	}
	if len(payload) == 0 {
		return "{}"
	}
	return mustCompactJSON(payload)
}

func sendTaskNotifications(ctx context.Context, opts taskNotificationOptions, task store.AgentTask, run store.AgentTaskRun, message string) []notifyResult {
	if strings.TrimSpace(opts.File) == "" && strings.TrimSpace(opts.Webhook) == "" {
		return nil
	}
	return sendNotificationEvent(ctx, opts, notificationEvent{
		TaskID:    task.ID,
		TaskName:  task.Name,
		RunID:     run.ID,
		Status:    run.Status,
		Message:   message,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func sendNotificationEvent(ctx context.Context, opts taskNotificationOptions, event notificationEvent) []notifyResult {
	results := []notifyResult{}
	if strings.TrimSpace(opts.File) != "" {
		results = append(results, appendNotificationFile(opts.File, event))
	}
	if strings.TrimSpace(opts.Webhook) != "" {
		results = append(results, postNotificationWebhook(ctx, opts.Webhook, event))
	}
	if len(results) == 0 {
		results = append(results, notifyResult{OK: false, Channel: "none", Error: "no notification target configured"})
	}
	return results
}

func appendNotificationFile(path string, event notificationEvent) notifyResult {
	path = strings.TrimSpace(path)
	result := notifyResult{Channel: "file", Target: path}
	raw, err := json.Marshal(event)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		result.Error = err.Error()
		return result
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	_, writeErr := file.Write(append(raw, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		result.Error = writeErr.Error()
		return result
	}
	if closeErr != nil {
		result.Error = closeErr.Error()
		return result
	}
	result.OK = true
	return result
}

func postNotificationWebhook(ctx context.Context, rawURL string, event notificationEvent) notifyResult {
	result := notifyResult{Channel: "webhook", Target: rawURL}
	raw, err := json.Marshal(event)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(raw))
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := notificationHTTPClient.Do(req)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	closeErr := resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Error = resp.Status
		if closeErr != nil {
			result.Error += "; close response body: " + closeErr.Error()
		}
		return result
	}
	if closeErr != nil {
		result.Error = closeErr.Error()
		return result
	}
	result.OK = true
	return result
}

func notificationResultsOK(results []notifyResult) bool {
	if len(results) == 0 {
		return true
	}
	for _, result := range results {
		if !result.OK {
			return false
		}
	}
	return true
}

func notificationResultsError(results []notifyResult) error {
	if notificationResultsOK(results) {
		return nil
	}
	messages := make([]string, 0, len(results))
	for _, result := range results {
		if result.OK {
			continue
		}
		target := result.Target
		if target == "" {
			target = result.Channel
		}
		if result.Error == "" {
			messages = append(messages, fmt.Sprintf("%s notification to %s failed", result.Channel, target))
			continue
		}
		messages = append(messages, fmt.Sprintf("%s notification to %s failed: %s", result.Channel, target, result.Error))
	}
	return errors.New(strings.Join(messages, "; "))
}
