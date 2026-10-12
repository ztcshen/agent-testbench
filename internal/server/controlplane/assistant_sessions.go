package controlplane

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/domain/redaction"
	"agent-testbench/internal/store"
)

type assistantSessionMutation struct {
	MapID    string         `json:"mapId"`
	OwnerID  string         `json:"ownerId"`
	Title    string         `json:"title"`
	Status   string         `json:"status"`
	Revision int64          `json:"revision"`
	State    map[string]any `json:"state"`
}

func registerAssistantRoutes(mux *http.ServeMux, runtime store.Store) {
	mux.HandleFunc("/api/assistant/sessions", func(w http.ResponseWriter, r *http.Request) {
		sessions, ok := runtime.(store.AssistantSessionStore)
		if !ok {
			assistantError(w, http.StatusServiceUnavailable, errors.New("assistant Store is unavailable"))
			return
		}
		switch r.Method {
		case http.MethodGet:
			items, err := sessions.ListAssistantSessions(r.Context(), r.URL.Query().Get("ownerId"), r.URL.Query().Get("mapId"))
			if assistantReadError(w, err) {
				return
			}
			payload := make([]map[string]any, 0, len(items))
			for _, item := range items {
				summary := assistantSessionPayload(item)
				delete(summary, "state")
				payload = append(payload, summary)
			}
			writeJSON(w, map[string]any{"ok": true, "sessions": payload})
		case http.MethodPost:
			createAssistantSession(w, r, runtime, sessions)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/assistant/session", func(w http.ResponseWriter, r *http.Request) {
		sessions, ok := runtime.(store.AssistantSessionStore)
		if !ok {
			assistantError(w, http.StatusServiceUnavailable, errors.New("assistant Store is unavailable"))
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, err := sessions.GetAssistantSession(r.Context(), r.URL.Query().Get("id"))
		if assistantReadError(w, err) {
			return
		}
		if r.Method == http.MethodPut {
			input, ok := readAssistantMutation(w, r)
			if !ok {
				return
			}
			if !assistantSessionStatus(input.Status) || input.Revision < 1 || input.State == nil {
				assistantError(w, 400, errors.New("revision, state and a valid assistant status are required"))
				return
			}
			previousState := jsonObject(item.StateJSON)
			if input.Status == "running" && item.Status != "running" {
				graph, graphErr := runtime.GetTestPlanGraph(r.Context(), item.MapID)
				if assistantReadError(w, graphErr) {
					return
				}
				if graph.Map.ProfileID != item.ProfileID {
					assistantError(w, 409, errors.New("map profile changed; start a new conversation"))
					return
				}
				input.State["graphFingerprint"] = mapplanner.GraphFingerprint(graph)
			} else {
				input.State["graphFingerprint"] = previousState["graphFingerprint"]
			}
			if input.Status == "completed" {
				graph, graphErr := runtime.GetTestPlanGraph(r.Context(), item.MapID)
				if assistantReadError(w, graphErr) {
					return
				}
				if valueString(input.State["graphFingerprint"]) != mapplanner.GraphFingerprint(graph) {
					assistantError(w, 409, errors.New("map changed during analysis; analyze the current revision again"))
					return
				}
			}
			item.Status, item.StateJSON, item.UpdatedAt = input.Status, assistantStateJSON(input.State), time.Now().UTC()
			if input.Title != "" {
				item.Title = redaction.Text(input.Title)
			}
			changed, updateErr := sessions.UpdateAssistantSession(r.Context(), item, input.Revision)
			if assistantReadError(w, updateErr) {
				return
			}
			if !changed {
				assistantError(w, http.StatusConflict, errors.New("assistant session revision changed; reload before continuing"))
				return
			}
			item.Revision = input.Revision + 1
		}
		writeJSON(w, map[string]any{"ok": true, "session": assistantSessionPayload(item)})
	})
	handleMethod(mux, "/api/assistant/tool", http.MethodPost, func(w http.ResponseWriter, r *http.Request) { handleAssistantTool(w, r, runtime) })
}

func createAssistantSession(w http.ResponseWriter, r *http.Request, runtime store.Store, sessions store.AssistantSessionStore) {
	input, ok := readAssistantMutation(w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(input.OwnerID) == "" || len(input.OwnerID) > 128 || len(input.Title) > 240 {
		assistantError(w, 400, errors.New("ownerId and a bounded title are required"))
		return
	}
	graph, err := runtime.GetTestPlanGraph(r.Context(), input.MapID)
	if assistantReadError(w, err) {
		return
	}
	identifier := make([]byte, 16)
	if _, err := rand.Read(identifier); assistantReadError(w, err) {
		return
	}
	now := time.Now().UTC()
	item := store.AssistantSession{ID: "assistant." + hex.EncodeToString(identifier), OwnerID: input.OwnerID, MapID: graph.Map.ID, ProfileID: graph.Map.ProfileID, Title: redaction.Text(input.Title), Status: "idle", Revision: 1, StateJSON: `{"messages":[],"events":[]}`, CreatedAt: now, UpdatedAt: now}
	if assistantReadError(w, sessions.CreateAssistantSession(r.Context(), item)) {
		return
	}
	writeJSON(w, map[string]any{"ok": true, "session": assistantSessionPayload(item)})
}

func readAssistantMutation(w http.ResponseWriter, r *http.Request) (assistantSessionMutation, bool) {
	var input assistantSessionMutation
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		assistantError(w, 400, errors.New("invalid assistant session payload"))
		return input, false
	}
	return input, true
}

func assistantSessionPayload(item store.AssistantSession) map[string]any {
	return map[string]any{"id": item.ID, "ownerId": item.OwnerID, "mapId": item.MapID, "profileId": item.ProfileID, "title": item.Title, "status": item.Status, "revision": item.Revision, "state": redaction.Value(jsonObject(item.StateJSON)), "createdAt": item.CreatedAt, "updatedAt": item.UpdatedAt}
}

func assistantStateJSON(state map[string]any) string {
	data, _ := json.Marshal(redaction.Value(state))
	return string(data)
}

func assistantSessionStatus(status string) bool {
	switch status {
	case "idle", "running", "completed", "failed", "cancelled", "interrupted":
		return true
	}
	return false
}

func assistantError(w http.ResponseWriter, status int, err error) {
	writeJSONStatus(w, status, map[string]any{"ok": false, "error": fmt.Sprint(err)})
}
func assistantReadError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	status := http.StatusInternalServerError
	if errors.Is(err, store.ErrNotFound) {
		status = http.StatusNotFound
	}
	assistantError(w, status, err)
	return true
}
