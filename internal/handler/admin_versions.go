package handler

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/store"
)

const maxUploadSize = 100 << 20 // 100MB

type AdminVersionHandler struct {
	packages store.PackageStore
	storage  storage.Storage
	cache    *composer.Cache
}

func NewAdminVersionHandler(packages store.PackageStore, storage storage.Storage, cache *composer.Cache) *AdminVersionHandler {
	return &AdminVersionHandler{packages: packages, storage: storage, cache: cache}
}

func (h *AdminVersionHandler) Upload(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	pkgID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_package_id", "invalid package id")
		return
	}

	pkg, err := h.packages.GetByID(r.Context(), org.ID, pkgID)
	if err != nil || pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "file_too_large", "file too large or invalid multipart form")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file_field_required", "file field required")
		return
	}
	defer file.Close()

	// Save to temp file for ZIP processing.
	tmpPath, err := composer.SaveTempFile(file, maxUploadSize)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upload", err.Error())
		return
	}
	defer os.Remove(tmpPath)

	// Parse composer.json from ZIP.
	cj, err := composer.ParseZIP(tmpPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_package_zip", fmt.Sprintf("invalid package: %s", err))
		return
	}

	// Validate package name matches.
	if cj.Name != pkg.Name {
		writeError(w, http.StatusBadRequest, "package_name_mismatch",
			fmt.Sprintf("composer.json name %q does not match package %q", cj.Name, pkg.Name))
		return
	}

	if cj.Version == "" {
		cj.Version = r.FormValue("version")
	}
	if err := composer.ValidateVersion(cj.Version); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_version", err.Error())
		return
	}

	// Check for duplicate version.
	versions, _ := h.packages.ListVersions(r.Context(), org.ID, pkgID)
	for _, v := range versions {
		if v.Version == cj.Version {
			writeError(w, http.StatusConflict, "version_already_exists",
				fmt.Sprintf("version %s already exists", cj.Version))
			return
		}
	}

	// Compute SHA-1 checksum. Composer v2's `shasum` field is SHA-1, not
	// SHA-256 — this is the protocol-mandated hash, not a security choice.
	tmpFile, err := os.Open(tmpPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed_read_uploaded_file", "failed to read uploaded file")
		return
	}
	defer tmpFile.Close()

	hasher := sha1.New()
	fileSize, err := io.Copy(hasher, tmpFile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed_compute_checksum", "failed to compute checksum")
		return
	}
	if fileSize == 0 {
		// A zero-byte artifact would serve Content-Length: 0 to Composer
		// and poison the metadata cache with a useless dist. Reject at
		// the boundary — nothing downstream handles this gracefully.
		writeError(w, http.StatusBadRequest, "uploaded_file_empty", "uploaded file is empty")
		return
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))

	// Upload to storage. Storage keys are org-prefixed so two tenants can
	// publish packages with the same name without colliding.
	storagePath := fmt.Sprintf("%d/%s/%s.zip", pkg.OrgID, pkg.Name, cj.Version)
	tmpFile.Seek(0, io.SeekStart)
	if err := h.storage.Put(r.Context(), storagePath, tmpFile, fileSize); err != nil {
		slog.Error("storage put error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_store_package", "failed to store package")
		return
	}

	// Marshal require JSON.
	var requireJSON string
	if cj.Require != nil {
		data, _ := json.Marshal(cj.Require)
		requireJSON = string(data)
	}

	version := &model.Version{
		PackageID:         pkg.ID,
		Version:           cj.Version,
		VersionNormalized: composer.NormalizeVersion(cj.Version),
		DistType:          "zip",
		DistSHA1:          checksum,
		StoragePath:       storagePath,
		FileSize:          fileSize,
		ComposerJSON:      cj.RawJSON,
		RequireJSON:       requireJSON,
	}

	if err := h.packages.CreateVersion(r.Context(), version); err != nil {
		slog.Error("create version error", "error", err)
		h.storage.Delete(r.Context(), storagePath)
		writeError(w, http.StatusInternalServerError, "failed_create_version", "failed to create version")
		return
	}

	h.cache.Invalidate(r.Context(), org.ID)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"version": version,
	})
}

func (h *AdminVersionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_version_id", "invalid version id")
		return
	}

	version, err := h.packages.GetVersionByID(r.Context(), id)
	if err != nil {
		slog.Error("get version error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if version == nil {
		writeError(w, http.StatusNotFound, "version_not_found", "version not found")
		return
	}

	// Verify the version's package belongs to this org.
	pkg, err := h.packages.GetByID(r.Context(), org.ID, version.PackageID)
	if err != nil || pkg == nil {
		writeError(w, http.StatusNotFound, "version_not_found", "version not found")
		return
	}

	// Delete file from storage.
	if err := h.storage.Delete(r.Context(), version.StoragePath); err != nil {
		slog.Error("delete version file error", "error", err, "version_id", version.ID)
	}

	if err := h.packages.DeleteVersion(r.Context(), id); err != nil {
		slog.Error("delete version error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_delete_version", "failed to delete version")
		return
	}

	h.cache.Invalidate(r.Context(), org.ID)
	w.WriteHeader(http.StatusNoContent)
}
