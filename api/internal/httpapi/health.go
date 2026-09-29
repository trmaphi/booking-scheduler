package httpapi

import (
	"context"
	"net/http"
	"time"
)

func live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Status string `json:"status"`
	}{Status: "ok"})
}

func ready(check func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if check == nil {
			writeAPIError(w, r, http.StatusServiceUnavailable, "NOT_READY", "service unavailable", nil)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := check(ctx); err != nil {
			writeAPIError(w, r, http.StatusServiceUnavailable, "NOT_READY", "service unavailable", nil)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Status string `json:"status"`
		}{Status: "ok"})
	}
}
