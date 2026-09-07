package controlplane

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"agent-testbench/internal/runner/direct"
	"agent-testbench/internal/store"
)

// NewDirectHandler serves the explicitly selected local direct-execution mode.
// Its route set does not perform managed-environment or Docker discovery.
func NewDirectHandler(runtime store.Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/direct", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "mode": direct.Mode, "runtime": "native-process", "managesTarget": false,
			"operations": []string{"/api/direct/bindings", "/api/direct/plan", "/api/direct/runs"}})
	})
	mux.HandleFunc("GET /api/direct/bindings", func(w http.ResponseWriter, r *http.Request) {
		items, err := direct.List(r.Context(), runtime)
		if writeDirectError(w, err) {
			return
		}
		writeJSON(w, map[string]any{"ok": true, "mode": direct.Mode, "bindings": items})
	})
	mux.HandleFunc("POST /api/direct/bindings", func(w http.ResponseWriter, r *http.Request) {
		var request direct.RegisterRequest
		if !readDirectRequest(w, r, &request) {
			return
		}
		result, err := direct.Register(r.Context(), runtime, request)
		if !writeDirectError(w, err) {
			writeJSON(w, result)
		}
	})
	mux.HandleFunc("POST /api/direct/plan", func(w http.ResponseWriter, r *http.Request) {
		var request direct.RunRequest
		if !readDirectRequest(w, r, &request) {
			return
		}
		plan, err := direct.BuildPlan(r.Context(), runtime, request)
		if !writeDirectError(w, err) {
			writeJSON(w, plan)
		}
	})
	mux.HandleFunc("POST /api/direct/runs", func(w http.ResponseWriter, r *http.Request) {
		var request direct.RunRequest
		if !readDirectRequest(w, r, &request) {
			return
		}
		report, err := direct.Run(r.Context(), runtime, request)
		if writeDirectError(w, err) {
			return
		}
		status := http.StatusOK
		if report.Status == store.StatusRunning {
			status = http.StatusAccepted
		}
		writeJSONStatus(w, status, report)
	})
	mux.HandleFunc("GET /api/direct/runs/{runID}", func(w http.ResponseWriter, r *http.Request) {
		report, err := direct.Inspect(r.Context(), runtime, r.PathValue("runID"))
		if !writeDirectError(w, err) {
			writeJSON(w, report)
		}
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !directLocalRequest(r) {
			writeJSONStatus(w, http.StatusForbidden, map[string]any{"ok": false, "error": "direct mode accepts same-origin loopback requests only"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func directLocalRequest(r *http.Request) bool {
	host := r.Host
	if value, _, err := net.SplitHostPort(host); err == nil {
		host = value
	}
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return false
		}
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Host != r.Host {
			return false
		}
	}
	return true
}

func readDirectRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeJSONStatus(w, http.StatusUnsupportedMediaType, map[string]any{"ok": false, "error": "application/json is required"})
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(target)
	if err == nil && decoder.Decode(new(any)) != io.EOF {
		err = errors.New("expected one JSON object")
	}
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return false
	}
	return true
}

func writeDirectError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, direct.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, direct.ErrConflict), errors.Is(err, store.ErrProfileCatalogRevisionConflict):
		status = http.StatusConflict
	case errors.Is(err, store.ErrNotFound):
		status = http.StatusNotFound
	}
	writeJSONStatus(w, status, map[string]any{"ok": false, "mode": direct.Mode, "error": err.Error()})
	return true
}
