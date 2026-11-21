package gui

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
)

func isEmbedded() bool {
	return os.Getenv("MAGPIE_EMBEDDED") == "1"
}

type managedProviderSyncRequest struct {
	Prefix    string              `json:"prefix"`
	Providers []provider.Provider `json:"providers"`
}

func embeddedManagedRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/managed/health", func(rw http.ResponseWriter, r *http.Request) {
		if !isEmbedded() {
			http.NotFound(rw, r)
			return
		}
		writeJSON(rw, map[string]any{
			"ok":         true,
			"embedded":   true,
			"gatewayUrl": gateway.URL(),
			"version":    Version,
		})
	})

	mux.HandleFunc("POST /api/managed/providers/sync", func(rw http.ResponseWriter, r *http.Request) {
		if !isEmbedded() {
			http.NotFound(rw, r)
			return
		}
		var req managedProviderSyncRequest
		if err := json.NewDecoder(http.MaxBytesReader(rw, r.Body, 2<<20)).Decode(&req); err != nil {
			http.Error(rw, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}
		prefix := strings.TrimSpace(req.Prefix)
		if prefix == "" {
			prefix = "lov-env-"
		}
		if !strings.HasPrefix(prefix, "lov-") {
			http.Error(rw, "managed prefix must start with lov-", http.StatusBadRequest)
			return
		}

		desired := make(map[string]struct{}, len(req.Providers))
		for _, p := range req.Providers {
			id := strings.TrimSpace(p.ID)
			if id == "" || !strings.HasPrefix(id, prefix) {
				http.Error(rw, "every managed provider id must start with prefix "+prefix, http.StatusBadRequest)
				return
			}
			desired[id] = struct{}{}
		}

		var removed []string
		for _, existing := range provider.All() {
			if strings.HasPrefix(existing.ID, prefix) {
				if _, keep := desired[existing.ID]; !keep {
					if err := provider.Delete(existing.ID); err != nil {
						fail(rw, err)
						return
					}
					removed = append(removed, existing.ID)
				}
			}
		}

		var synced []string
		for _, p := range req.Providers {
			if p.Proxy == "" {
				p.Proxy = "direct"
			}
			if err := provider.Save(p); err != nil {
				fail(rw, err)
				return
			}
			synced = append(synced, p.ID)
		}

		writeJSON(rw, map[string]any{
			"ok":      true,
			"prefix":  prefix,
			"synced":  synced,
			"removed": removed,
		})
	})
}
