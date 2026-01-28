package handler

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/store"
)

// PackageStatsHandler serves the aggregated download stats used by the
// dashboard. One endpoint returns every section (totals, top packages,
// recent activity, daily series) so the dashboard loads in a single
// round trip instead of N.
//
// Responses are cached per-org for `cacheTTL` to collapse concurrent
// dashboard tabs into one DB hit per window. The six-query aggregation
// (including a DailySeries scan of the 30-day event window) is otherwise
// a convenient DoS surface for any authenticated user.
type PackageStatsHandler struct {
	downloads store.DownloadStore
	cacheTTL  time.Duration

	cacheMu sync.RWMutex
	cache   map[int64]cachedStats
}

type cachedStats struct {
	payload  packageStatsResponse
	expireAt time.Time
}

// NewPackageStatsHandler returns a stats handler with the given cache TTL.
// Pass 0 to disable caching (every request hits the DB).
func NewPackageStatsHandler(downloads store.DownloadStore, cacheTTL time.Duration) *PackageStatsHandler {
	return &PackageStatsHandler{
		downloads: downloads,
		cacheTTL:  cacheTTL,
		cache:     make(map[int64]cachedStats),
	}
}

type packageStatsResponse struct {
	TotalDownloads   int64                        `json:"total_downloads"`
	DownloadsLast7d  int64                        `json:"downloads_last_7d"`
	DownloadsLast30d int64                        `json:"downloads_last_30d"`
	TopPackages      []store.PackageDownloadCount `json:"top_packages"`
	RecentDownloads  []store.DownloadEventView    `json:"recent_downloads"`
	DailySeries30d   []store.DailyCount           `json:"daily_series_30d"`
}

// Stats serves GET /api/[orgs/{slug}/]packages/stats — org-scoped.
func (h *PackageStatsHandler) Stats(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	if payload, ok := h.fromCache(org.ID); ok {
		writeJSON(w, http.StatusOK, payload)
		return
	}

	payload, err := h.load(r.Context(), org.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed_read_stats", "failed to read stats")
		return
	}

	h.storeCache(org.ID, payload)
	writeJSON(w, http.StatusOK, payload)
}

// load runs the six-query aggregation. Errors are logged at the source
// and returned as a generic failure for the handler.
func (h *PackageStatsHandler) load(ctx context.Context, orgID int64) (packageStatsResponse, error) {
	now := time.Now().UTC()
	week := now.Add(-7 * 24 * time.Hour)
	month := now.Add(-30 * 24 * time.Hour)

	var out packageStatsResponse

	total, err := h.downloads.TotalSince(ctx, orgID, time.Time{})
	if err != nil {
		slog.Error("download total error", "error", err, "org_id", orgID)
		return out, err
	}
	last7, err := h.downloads.TotalSince(ctx, orgID, week)
	if err != nil {
		slog.Error("download last7 error", "error", err)
		return out, err
	}
	last30, err := h.downloads.TotalSince(ctx, orgID, month)
	if err != nil {
		slog.Error("download last30 error", "error", err)
		return out, err
	}
	top, err := h.downloads.TopPackages(ctx, orgID, time.Time{}, 5)
	if err != nil {
		slog.Error("download top error", "error", err)
		return out, err
	}
	if top == nil {
		top = []store.PackageDownloadCount{}
	}
	recent, err := h.downloads.Recent(ctx, orgID, 10)
	if err != nil {
		slog.Error("download recent error", "error", err)
		return out, err
	}
	if recent == nil {
		recent = []store.DownloadEventView{}
	}
	series, err := h.downloads.DailySeries(ctx, orgID, 30)
	if err != nil {
		slog.Error("download series error", "error", err)
		return out, err
	}
	if series == nil {
		series = []store.DailyCount{}
	}

	out = packageStatsResponse{
		TotalDownloads:   total,
		DownloadsLast7d:  last7,
		DownloadsLast30d: last30,
		TopPackages:      top,
		RecentDownloads:  recent,
		DailySeries30d:   series,
	}
	return out, nil
}

func (h *PackageStatsHandler) fromCache(orgID int64) (packageStatsResponse, bool) {
	if h.cacheTTL <= 0 {
		return packageStatsResponse{}, false
	}
	h.cacheMu.RLock()
	entry, ok := h.cache[orgID]
	h.cacheMu.RUnlock()
	if !ok || time.Now().After(entry.expireAt) {
		return packageStatsResponse{}, false
	}
	return entry.payload, true
}

func (h *PackageStatsHandler) storeCache(orgID int64, payload packageStatsResponse) {
	if h.cacheTTL <= 0 {
		return
	}
	h.cacheMu.Lock()
	h.cache[orgID] = cachedStats{payload: payload, expireAt: time.Now().Add(h.cacheTTL)}
	h.cacheMu.Unlock()
}
