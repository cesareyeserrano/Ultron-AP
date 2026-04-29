// Package api hosts HTTP handlers for /network/* (HTML) and /api/network/*
// (JSON + SSE).
//
// SKELETON-ONLY. Handlers are registered but return 501 Not Implemented — see
// spec/04_IMPLEMENTATION_MANIFEST.json technical_debt.
package api

import (
	"net/http"
)

// Handler bundles all HTTP handlers for the feature. The parent server mounts
// these via Register.
//
// @aitri-trace FR-ID: FR-019
type Handler struct{}

// New returns a skeleton Handler.
func New() *Handler { return &Handler{} }

// Register attaches all /network/* and /api/network/* routes to mux.
//
// @aitri-trace FR-ID: FR-019
func (h *Handler) Register(mux *http.ServeMux) {
	if mux == nil {
		return
	}
	mux.HandleFunc("/network", h.notImplemented)
	mux.HandleFunc("/network/", h.notImplemented)
	mux.HandleFunc("/api/network/", h.notImplemented)
}

func (h *Handler) notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, `{"error":{"code":"not_implemented","message":"network-monitoring: skeleton-only"}}`, http.StatusNotImplemented)
}
