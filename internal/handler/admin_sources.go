package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/provider"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/store"
)

type AdminSourceHandler struct {
	sources  store.SourceStore
	packages store.PackageStore
	jobs     store.JobStore
	storage  storage.Storage
	cache    *composer.Cache
	cfg      *config.Config
}

func NewAdminSourceHandler(sources store.SourceStore, packages store.PackageStore, jobs store.JobStore, strg storage.Storage, cache *composer.Cache, cfg *config.Config) *AdminSourceHandler {
	return &AdminSourceHandler{
		sources:  sources,
		packages: packages,
		jobs:     jobs,
		storage:  strg,
		cache:    cache,
		cfg:      cfg,
	}
}

func (h *AdminSourceHandler) Get(w http.ResponseWriter, r *http.Request) {
	pkgID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_package_id", "invalid package id")
		return
	}

	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	// Verify the package belongs to this org.
	pkg, err := h.packages.GetByID(r.Context(), org.ID, pkgID)
	if err != nil || pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	src, err := h.sources.GetByPackageID(r.Context(), pkg.ID)
	if err != nil {
		slog.Error("get source error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if src == nil {
		writeError(w, http.StatusNotFound, "no_source_configured", "no source configured")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"source":      src,
		"webhook_url": h.cfg.BaseURL + "/hooks/" + src.Provider,
	})
}

func (h *AdminSourceHandler) Set(w http.ResponseWriter, r *http.Request) {
	pkgID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_package_id", "invalid package id")
		return
	}

	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	pkg, err := h.packages.GetByID(r.Context(), org.ID, pkgID)
	if err != nil || pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	var req struct {
		Provider       string `json:"provider"`
		RepoOwner      string `json:"repo_owner"`
		RepoName       string `json:"repo_name"`
		Strategy       string `json:"strategy"`
		AssetPattern   string `json:"asset_pattern"`
		MetadataSource string `json:"metadata_source"`
		VersionSource  string `json:"version_source"`
		ManualRequire  string `json:"manual_require"`
		AuthToken      string `json:"auth_token"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}

	if req.Provider == "" {
		req.Provider = "github"
	}
	if _, err := provider.NewProvider(req.Provider, ""); err != nil {
		writeError(w, http.StatusBadRequest, "unsupported_provider", err.Error())
		return
	}
	if req.RepoOwner == "" || req.RepoName == "" {
		writeError(w, http.StatusBadRequest, "repo_owner_and_name_required", "repo_owner and repo_name are required")
		return
	}
	if req.Strategy == "" {
		req.Strategy = "release_asset"
	}
	switch req.Strategy {
	case "release_asset", "source_archive":
		// valid
	default:
		writeError(w, http.StatusBadRequest, "invalid_strategy", "strategy must be release_asset or source_archive")
		return
	}
	if req.AssetPattern == "" {
		req.AssetPattern = "*.zip"
	}
	if _, err := filepath.Match(req.AssetPattern, "test.zip"); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_asset_pattern", "invalid asset_pattern glob")
		return
	}
	if req.MetadataSource == "" {
		req.MetadataSource = "from_zip"
	}
	switch req.MetadataSource {
	case "from_zip", "manual":
		// valid
	default:
		writeError(w, http.StatusBadRequest, "invalid_metadata_source", "metadata_source must be from_zip or manual")
		return
	}
	// manual metadata with source_archive is pointless — the source zipball
	// always has composer.json. Reject so users don't footgun themselves.
	if req.MetadataSource == "manual" && req.Strategy == "source_archive" {
		writeError(w, http.StatusBadRequest, "manual_metadata_requires_release_asset", "manual metadata only applies to release_asset strategy")
		return
	}
	if req.VersionSource == "" {
		req.VersionSource = "auto"
	}
	switch req.VersionSource {
	case "auto", "git_tag", "composer_json":
		// valid
	default:
		writeError(w, http.StatusBadRequest, "invalid_version_source", "version_source must be auto, git_tag, or composer_json")
		return
	}
	// manual metadata can only use git_tag — there's no composer.json to
	// read from. Silently coerce so the frontend's disabled select matches
	// backend behavior exactly.
	if req.MetadataSource == "manual" {
		req.VersionSource = "git_tag"
	}
	// Validate ManualRequire parses as a JSON object of string→string when
	// supplied. Empty is allowed (no require).
	if req.MetadataSource == "manual" && strings.TrimSpace(req.ManualRequire) != "" {
		var tmp map[string]string
		if err := json.Unmarshal([]byte(req.ManualRequire), &tmp); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_manual_require", "manual_require must be a JSON object mapping package names to version constraints")
			return
		}
	}

	existing, err := h.sources.GetByPackageID(r.Context(), pkg.ID)
	if err != nil {
		slog.Error("source lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	isNew := existing == nil
	var webhookSecret string

	if isNew {
		secret, err := generateWebhookSecret()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed_generate_webhook_secret", "failed to generate webhook secret")
			return
		}
		webhookSecret = secret

		src := &model.PackageSource{
			PackageID:      pkg.ID,
			Provider:       req.Provider,
			RepoOwner:      req.RepoOwner,
			RepoName:       req.RepoName,
			Strategy:       req.Strategy,
			AssetPattern:   req.AssetPattern,
			MetadataSource: req.MetadataSource,
			VersionSource:  req.VersionSource,
			ManualRequire:  req.ManualRequire,
			AuthToken:      req.AuthToken,
			WebhookSecret:  webhookSecret,
		}
		if err := h.sources.Create(r.Context(), src); err != nil {
			slog.Error("create source error", "error", err)
			writeError(w, http.StatusInternalServerError, "failed_create_source", "failed to create source")
			return
		}
	} else {
		existing.Provider = req.Provider
		existing.RepoOwner = req.RepoOwner
		existing.RepoName = req.RepoName
		existing.Strategy = req.Strategy
		existing.AssetPattern = req.AssetPattern
		existing.MetadataSource = req.MetadataSource
		existing.VersionSource = req.VersionSource
		existing.ManualRequire = req.ManualRequire
		if req.AuthToken != "" {
			existing.AuthToken = req.AuthToken
		}
		if err := h.sources.Update(r.Context(), existing); err != nil {
			slog.Error("update source error", "error", err)
			writeError(w, http.StatusInternalServerError, "failed_update_source", "failed to update source")
			return
		}
	}

	src, _ := h.sources.GetByPackageID(r.Context(), pkg.ID)

	resp := map[string]interface{}{
		"source":      src,
		"webhook_url": h.cfg.BaseURL + "/hooks/" + src.Provider,
	}
	if isNew {
		resp["webhook_secret"] = webhookSecret
	}

	status := http.StatusOK
	if isNew {
		status = http.StatusCreated
	}
	writeJSON(w, status, resp)
}

func (h *AdminSourceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	pkgID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_package_id", "invalid package id")
		return
	}

	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	// Verify the package belongs to this org.
	pkg, err := h.packages.GetByID(r.Context(), org.ID, pkgID)
	if err != nil || pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	if err := h.sources.Delete(r.Context(), pkg.ID); err != nil {
		slog.Error("delete source error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_delete_source", "failed to delete source")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Sync enqueues a sync job for this package and returns its ID. The
// actual sync runs in a worker (see internal/jobs). The client should
// poll GET .../sync/{job_id} for progress and final status.
//
// If a job is already queued or running for this package, responds with
// 409 + the existing job, letting the UI hook into the live run instead
// of scheduling a duplicate. Webhooks and manual triggers both go
// through this path for unified concurrency control.
func (h *AdminSourceHandler) Sync(w http.ResponseWriter, r *http.Request) {
	pkgID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_package_id", "invalid package id")
		return
	}

	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	pkg, err := h.packages.GetByID(r.Context(), org.ID, pkgID)
	if err != nil || pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	// Require a configured source — can't sync without one.
	src, err := h.sources.GetByPackageID(r.Context(), pkg.ID)
	if err != nil || src == nil {
		writeError(w, http.StatusNotFound, "no_source_configured", "no source configured")
		return
	}
	_ = src

	// Enforce one-active-sync-per-package. The worker claim query would
	// eventually serialize these anyway, but we want the UI to see the
	// existing job rather than silently queue a duplicate.
	if existing, err := h.jobs.ActiveForPackage(r.Context(), pkg.ID); err != nil {
		slog.Error("active-for-package lookup", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	} else if existing != nil {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"job":      existing,
			"existing": true,
		})
		return
	}

	job := &model.SyncJob{
		OrgID:     org.ID,
		PackageID: pkg.ID,
		Trigger:   "manual",
	}
	if err := h.jobs.Enqueue(r.Context(), job); err != nil {
		slog.Error("enqueue sync job", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_enqueue_sync", "failed to enqueue sync")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"job":      job,
		"existing": false,
	})
}

// GetSyncJob returns the state of a single sync job scoped to this org
// and package — used by the frontend's poll loop.
func (h *AdminSourceHandler) GetSyncJob(w http.ResponseWriter, r *http.Request) {
	pkgID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_package_id", "invalid package id")
		return
	}
	jobID, err := pathID(r, "job_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_job_id", "invalid job id")
		return
	}

	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	// Confirm the package belongs to this org — same cross-tenant guard
	// as every other org-scoped admin handler.
	pkg, err := h.packages.GetByID(r.Context(), org.ID, pkgID)
	if err != nil || pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	job, err := h.jobs.GetByID(r.Context(), org.ID, jobID)
	if err != nil {
		slog.Error("get sync job", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if job == nil || job.PackageID != pkg.ID {
		writeError(w, http.StatusNotFound, "job_not_found", "job not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"job": job})
}

// ListSyncJobs returns the most recent sync jobs for this package.
// Powers the (future) sync history UI; endpoint lands with the rest of
// the plumbing so the API surface is stable.
func (h *AdminSourceHandler) ListSyncJobs(w http.ResponseWriter, r *http.Request) {
	pkgID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_package_id", "invalid package id")
		return
	}

	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	pkg, err := h.packages.GetByID(r.Context(), org.ID, pkgID)
	if err != nil || pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	jobs, err := h.jobs.ListForPackage(r.Context(), org.ID, pkg.ID, 20)
	if err != nil {
		slog.Error("list sync jobs", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if jobs == nil {
		jobs = []model.SyncJob{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"jobs": jobs})
}

func generateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// releasePreview is a trimmed projection of a release — just what the
// source-config form needs to let users pick a sensible asset pattern
// without guessing what their release actually looks like.
type releasePreview struct {
	Tag    string               `json:"tag"`
	Assets []releasePreviewAsset `json:"assets"`
}

type releasePreviewAsset struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// PreviewReleases serves POST /api/[orgs/{org}/]sources/preview with a
// JSON body {provider, owner, repo, auth_token}. Returns up to 3 recent
// releases with their asset filenames so the user can build a matching
// pattern before saving. An auth_token in the body overrides the org's
// default provider token for this call only — lets users test private
// repos from the form without persisting the token first. Body shape
// keeps the token out of URLs and access logs.
func (h *AdminSourceHandler) PreviewReleases(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	var req struct {
		Provider  string `json:"provider"`
		Owner     string `json:"owner"`
		Repo      string `json:"repo"`
		AuthToken string `json:"auth_token"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}
	providerName := req.Provider
	if providerName == "" {
		providerName = "github"
	}
	if req.Owner == "" || req.Repo == "" {
		writeError(w, http.StatusBadRequest, "owner_and_repo_required", "owner and repo are required")
		return
	}

	// Prefer the ad-hoc token from the request, fall back to the org's
	// configured global. Empty is fine — public repos work unauthenticated.
	token := req.AuthToken
	if token == "" {
		token = h.cfg.Providers.TokenFor(providerName)
	}
	prov, err := provider.NewProvider(providerName, token)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unsupported_provider", err.Error())
		return
	}

	releases, err := prov.ListReleases(r.Context(), req.Owner, req.Repo)
	if err != nil {
		slog.Warn("source preview list releases failed", "provider", providerName, "owner", req.Owner, "repo", req.Repo, "error", err)
		// Surface upstream failures distinctly. 404 typically means either
		// wrong coordinates OR a private repo we can't see — phrase both.
		writeError(w, http.StatusBadGateway, "failed_list_releases",
			"failed to list releases: "+err.Error()+
				". Check that owner/repo are correct; for private repos, provide an Auth Token.")
		return
	}

	const maxReleases = 3
	if len(releases) > maxReleases {
		releases = releases[:maxReleases]
	}

	out := make([]releasePreview, 0, len(releases))
	for _, rel := range releases {
		rp := releasePreview{
			Tag:    rel.TagName,
			Assets: make([]releasePreviewAsset, 0, len(rel.Assets)),
		}
		for _, a := range rel.Assets {
			rp.Assets = append(rp.Assets, releasePreviewAsset{Name: a.Name, Size: a.Size})
		}
		out = append(out, rp)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"releases": out})
}
