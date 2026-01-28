package handler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/store"
)

type ComposerHandler struct {
	cache     *composer.Cache
	storage   storage.Storage
	packages  store.PackageStore
	downloads store.DownloadStore
}

func NewComposerHandler(cache *composer.Cache, storage storage.Storage, packages store.PackageStore, downloads store.DownloadStore) *ComposerHandler {
	return &ComposerHandler{
		cache:     cache,
		storage:   storage,
		packages:  packages,
		downloads: downloads,
	}
}

// PackagesJSON serves GET /packages.json
func (h *ComposerHandler) PackagesJSON(w http.ResponseWriter, r *http.Request) {
	orgID, ok := auth.OrgIDFromToken(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing_org_context", "missing org context")
		return
	}

	data := h.cache.GetPackagesJSON(orgID)
	if data == nil {
		writeError(w, http.StatusInternalServerError, "metadata_not_available", "metadata not available")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// ProviderJSON serves GET /p2/{vendor}/{package}.json
func (h *ComposerHandler) ProviderJSON(w http.ResponseWriter, r *http.Request) {
	orgID, ok := auth.OrgIDFromToken(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing_org_context", "missing org context")
		return
	}

	vendor := r.PathValue("vendor")
	pkg := strings.TrimSuffix(r.PathValue("package"), ".json")
	name := vendor + "/" + pkg

	data := h.cache.GetProviderJSON(orgID, name)
	if data == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// Dist serves GET /dist/{vendor}/{package}/{version}
func (h *ComposerHandler) Dist(w http.ResponseWriter, r *http.Request) {
	orgID, ok := auth.OrgIDFromToken(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing_org_context", "missing org context")
		return
	}

	vendor := r.PathValue("vendor")
	pkg := r.PathValue("package")
	version := r.PathValue("version")
	name := vendor + "/" + pkg

	p, err := h.packages.GetByName(r.Context(), orgID, name)
	if err != nil {
		slog.Error("package lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	versions, err := h.packages.ListVersions(r.Context(), orgID, p.ID)
	if err != nil {
		slog.Error("version lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	for _, v := range versions {
		if v.Version == version {
			reader, err := h.storage.Get(r.Context(), v.StoragePath)
			if err != nil {
				slog.Error("storage get error", "error", err, "version_id", v.ID)
				writeError(w, http.StatusInternalServerError, "failed_retrieve_package", "failed to retrieve package")
				return
			}
			defer reader.Close()

			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.zip"`, pkg, version))
			if v.FileSize > 0 {
				w.Header().Set("Content-Length", fmt.Sprintf("%d", v.FileSize))
			}
			if _, err := io.Copy(w, reader); err != nil {
				// Client hung up or the storage stream died mid-copy. Don't
				// record a download event — we can't claim the client got the
				// zip. Best we can do is log.
				slog.Warn("dist copy interrupted", "error", err, "version_id", v.ID)
				return
			}

			// Fire-and-forget: record the download. Uses context.Background()
			// so a client disconnect after the transfer doesn't cancel the
			// write. Matches the pattern used by token UpdateLastUsed.
			h.recordDownload(orgID, p.ID, v.ID)
			return
		}
	}

	writeError(w, http.StatusNotFound, "version_not_found", "version not found")
}

// recordDownload bumps the per-version counter and appends a download event.
// Both writes go through one goroutine with a short timeout; errors are
// logged but never propagated — a download is successful from the client's
// perspective regardless of stat-keeping outcomes.
func (h *ComposerHandler) recordDownload(orgID, pkgID, versionID int64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		now := time.Now().UTC()
		if err := h.packages.IncrementDownload(ctx, versionID, now); err != nil {
			slog.Error("increment download counter", "error", err, "version_id", versionID)
		}
		if h.downloads == nil {
			return
		}
		if err := h.downloads.Record(ctx, &model.DownloadEvent{
			OrgID:     orgID,
			PackageID: pkgID,
			VersionID: versionID,
			At:        now,
		}); err != nil {
			slog.Error("record download event", "error", err, "version_id", versionID)
		}
	}()
}
