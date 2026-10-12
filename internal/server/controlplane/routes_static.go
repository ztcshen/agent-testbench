package controlplane

import (
	"net/http"
)

// The control plane exposes APIs. The desktop application owns the frontend.
// Old browser entrypoints return a clear retirement response rather than serving
// a second UI whose configuration could diverge from the Electron workspace.
func registerStaticRoutes(mux *http.ServeMux, deps routeDeps) {
	retired := func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSONStatus(w, http.StatusGone, map[string]any{"ok": false, "code": "desktop_frontend_required", "error": "The browser frontend has been retired. Open AgentTestBench Desktop.", "frontend": "electron"})
	}
	for _, name := range append([]string{"dashboard.html", "workflows.html"}, staticFileNames...) {
		mux.HandleFunc("/"+name, retired)
	}
	mux.HandleFunc("/assets/react/", retired)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "product": "AgentTestBench", "frontend": "electron", "message": "Open AgentTestBench Desktop to manage this workspace."})
	})
}
