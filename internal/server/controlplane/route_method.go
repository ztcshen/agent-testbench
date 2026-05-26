package controlplane

import "net/http"

func handleMethod(mux *http.ServeMux, pattern string, method string, handler http.HandlerFunc) {
	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, method) {
			return
		}
		handler(w, r)
	})
}
