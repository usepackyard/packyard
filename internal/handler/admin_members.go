package handler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/store"
)

type AdminMemberHandler struct {
	orgs       store.OrgStore
	users      store.UserStore
	bcryptCost int
}

func NewAdminMemberHandler(orgs store.OrgStore, users store.UserStore, bcryptCost int) *AdminMemberHandler {
	return &AdminMemberHandler{orgs: orgs, users: users, bcryptCost: bcryptCost}
}

func (h *AdminMemberHandler) List(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	members, err := h.orgs.ListMembers(r.Context(), org.ID)
	if err != nil {
		slog.Error("list members error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_list_members", "failed to list members")
		return
	}
	if members == nil {
		members = []model.OrgMember{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"members": members,
	})
}

func (h *AdminMemberHandler) Add(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	var req struct {
		Email       string   `json:"email"`
		Password    string   `json:"password"`
		Name        string   `json:"name"`
		Role        string   `json:"role"`
		Permissions []string `json:"permissions"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email_required", "email is required")
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}
	if req.Role != "owner" && req.Role != "member" {
		writeError(w, http.StatusBadRequest, "invalid_role", "role must be owner or member")
		return
	}

	// Find or create the user.
	user, err := h.users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		slog.Error("member add: user lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if user == nil {
		if req.Password == "" {
			writeError(w, http.StatusBadRequest, "password_required_for_new_users", "password is required for new users")
			return
		}
		hash, err := auth.HashPassword(req.Password, h.bcryptCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed_hash_password", "failed to hash password")
			return
		}
		user = &model.User{
			Email:    req.Email,
			Password: hash,
			Name:     req.Name,
			IsActive: true,
		}
		if err := h.users.Create(r.Context(), user); err != nil {
			slog.Error("member add: create user error", "error", err)
			writeError(w, http.StatusInternalServerError, "failed_create_user", "failed to create user")
			return
		}
	}

	// Check if already a member.
	existing, _ := h.orgs.GetMember(r.Context(), org.ID, user.ID)
	if existing != nil {
		writeError(w, http.StatusConflict, "user_already_member", "user is already a member")
		return
	}

	member := &model.OrgMember{
		OrgID:       org.ID,
		UserID:      user.ID,
		Role:        req.Role,
		Permissions: req.Permissions,
	}

	if err := h.orgs.AddMember(r.Context(), member); err != nil {
		slog.Error("member add error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_add_member", "failed to add member")
		return
	}

	member.User = user

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"member": member,
	})
}

func (h *AdminMemberHandler) Update(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	userID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_member_id", "invalid member id")
		return
	}

	member, err := h.orgs.GetMember(r.Context(), org.ID, userID)
	if err != nil {
		slog.Error("update member: lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if member == nil {
		writeError(w, http.StatusNotFound, "member_not_found", "member not found")
		return
	}

	var req struct {
		Role        *string  `json:"role"`
		Permissions []string `json:"permissions"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}

	if req.Role != nil {
		if *req.Role != "owner" && *req.Role != "member" {
			writeError(w, http.StatusBadRequest, "invalid_role", "role must be owner or member")
			return
		}
		member.Role = *req.Role
	}
	if req.Permissions != nil {
		member.Permissions = req.Permissions
	}

	if err := h.orgs.UpdateMember(r.Context(), member); err != nil {
		slog.Error("update member error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_update_member", "failed to update member")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"member": member,
	})
}

func (h *AdminMemberHandler) Remove(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	userID, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_member_id", "invalid member id")
		return
	}

	// Prevent removing yourself.
	currentUserID, _ := auth.UserIDFromContext(r.Context())
	if currentUserID == userID {
		writeError(w, http.StatusBadRequest, "cannot_remove_self", "cannot remove yourself")
		return
	}

	if err := h.orgs.RemoveMember(r.Context(), org.ID, userID); err != nil {
		slog.Error("remove member error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_remove_member", "failed to remove member")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
