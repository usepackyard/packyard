package handler

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/provider"
	"github.com/usepackyard/packyard/internal/store"
)

type WebhookHandler struct {
	sources  store.SourceStore
	packages store.PackageStore
	jobs     store.JobStore
	cfg      *config.Config
}

// NewWebhookHandler wires a handler that receives provider webhooks and
// enqueues sync jobs. Sync is no longer run inline in a goroutine — the
// job queue owns persistence and recovery so a crashed process doesn't
// silently lose work.
func NewWebhookHandler(sources store.SourceStore, packages store.PackageStore, jobs store.JobStore, cfg *config.Config) *WebhookHandler {
	return &WebhookHandler{
		sources:  sources,
		packages: packages,
		jobs:     jobs,
		cfg:      cfg,
	}
}

func (h *WebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	providerType := r.PathValue("provider")

	prov, err := provider.NewProvider(providerType, "")
	if err != nil {
		writeError(w, http.StatusNotFound, "unsupported_provider", "unsupported provider")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed_read_body", "failed to read body")
		return
	}

	event, err := prov.ParseWebhook(body)
	if err != nil {
		slog.Error("webhook: parse error", "error", err, "provider", providerType)
		writeError(w, http.StatusBadRequest, "invalid_payload", "invalid payload")
		return
	}

	if event.Action != "published" || event.IsDraft {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	src, err := h.sources.GetByRepo(r.Context(), providerType, event.RepoOwner, event.RepoName)
	if err != nil {
		slog.Error("webhook: source lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if src == nil {
		writeError(w, http.StatusNotFound, "no_source_configured_for_repo", "no source configured for this repository")
		return
	}

	// Fail closed: a source without a webhook secret cannot prove the request
	// is genuine, so reject it. Operators must configure a secret when setting
	// up the source — the URL alone is not sufficient authentication.
	if src.WebhookSecret == "" {
		slog.Warn("webhook: rejected — source has no signing secret",
			"repo", event.RepoOwner+"/"+event.RepoName)
		writeError(w, http.StatusUnauthorized, "webhook_secret_not_configured", "webhook signing secret not configured for this source")
		return
	}
	if err := prov.ValidateWebhook(r, src.WebhookSecret, body); err != nil {
		slog.Warn("webhook: signature validation failed", "error", err,
			"repo", event.RepoOwner+"/"+event.RepoName)
		writeError(w, http.StatusUnauthorized, "invalid_signature", "invalid signature")
		return
	}

	// Use GetByIDGlobal since webhooks don't have org context.
	pkg, err := h.packages.GetByIDGlobal(r.Context(), src.PackageID)
	if err != nil || pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	// If a sync for this package is already queued or running, the
	// webhook is effectively a no-op — let it piggy-back on the
	// in-flight run. Avoids pileups when a repo pushes many releases
	// in rapid succession.
	if existing, err := h.jobs.ActiveForPackage(r.Context(), pkg.ID); err != nil {
		slog.Error("webhook: active-for-package lookup", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	} else if existing != nil {
		slog.Info("webhook: sync already active, skipping enqueue",
			"repo", src.RepoOwner+"/"+src.RepoName, "job", existing.ID)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "already-queued",
			"job":    existing,
		})
		return
	}

	job := &model.SyncJob{
		OrgID:     pkg.OrgID,
		PackageID: pkg.ID,
		Trigger:   "webhook",
	}
	if err := h.jobs.Enqueue(r.Context(), job); err != nil {
		slog.Error("webhook: enqueue sync job", "error", err,
			"repo", src.RepoOwner+"/"+src.RepoName)
		writeError(w, http.StatusInternalServerError, "failed_enqueue_sync", "failed to enqueue sync")
		return
	}

	slog.Info("webhook: sync enqueued",
		"repo", src.RepoOwner+"/"+src.RepoName, "job", job.ID)
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "queued", "job": job})
}
