package httputil

import "net/http"

func VersionHandler(version string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		ResponseJSON(struct {
			Version string `json:"version"`
		}{Version: version}, http.StatusOK, w)
	})
}
