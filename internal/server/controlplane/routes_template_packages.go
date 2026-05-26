package controlplane

import "net/http"

func registerTemplatePackageRoutes(mux *http.ServeMux, deps routeDeps) {
	runtime := deps.runtime
	profiles := deps.profiles
	profileHome := deps.profileHome
	handleMethod(mux, "/api/template-packages/import-plan/openapi", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleOpenAPIImportPlan(w, r)
	})
	handleMethod(mux, "/api/template-packages/import-plan/http-capture", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleHTTPCaptureImportPlan(w, r)
	})
	handleMethod(mux, "/api/template-packages/generation-plan/openapi", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleOpenAPIGenerationPlan(w, r)
	})
	handleMethod(mux, "/api/template-packages/import", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleProfileImport(w, r, runtime, profiles.Replace, profileHome)
	})
	handleMethod(mux, "/api/template-packages/verify", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleProfileVerify(w, r, runtime, profiles.Replace, profileHome)
	})
	handleMethod(mux, "/api/template-packages/audit-plan", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleProfileAuditPlan(w, r, runtime, profileHome)
	})
	handleMethod(mux, "/api/template-packages/install", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleProfileInstall(w, r, profileHome)
	})
	handleMethod(mux, "/api/template-packages/installed", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleInstalledProfiles(w, r, profileHome)
	})
	handleMethod(mux, "/api/template-packages/catalog-index", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleProfileCatalogIndex(w, r, runtime)
	})
	handleMethod(mux, "/api/template-packages/current", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		writeProfileSummary(w, profiles.Current())
	})
	handleMethod(mux, "/api/template-packages/assets", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		writeProfileAssets(w, profiles.Current())
	})
	handleMethod(mux, "/api/profile/import", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleProfileImport(w, r, runtime, profiles.Replace, profileHome)
	})
	handleMethod(mux, "/api/profile/verify", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleProfileVerify(w, r, runtime, profiles.Replace, profileHome)
	})
	handleMethod(mux, "/api/profile/audit-plan", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleProfileAuditPlan(w, r, runtime, profileHome)
	})
	handleMethod(mux, "/api/profile/install", http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		handleProfileInstall(w, r, profileHome)
	})
	handleMethod(mux, "/api/profile/installed", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleInstalledProfiles(w, r, profileHome)
	})
	handleMethod(mux, "/api/profile", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		writeProfileSummary(w, profiles.Current())
	})
	handleMethod(mux, "/api/profile/assets", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		writeProfileAssets(w, profiles.Current())
	})
	handleMethod(mux, "/api/profile/catalog-index", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleProfileCatalogIndex(w, r, runtime)
	})
}
