package auth

import (
	"context"
	"net/http"

	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/store"
)

// enforceOrgStatus returns true if the request should proceed. For suspended
// orgs it writes a 402, for archived orgs a 404; both preserve data, neither
// reveals whether a member exists in the org.
func enforceOrgStatus(w http.ResponseWriter, org *model.Organization) bool {
	switch org.Status {
	case "", model.OrgStatusActive:
		return true
	case model.OrgStatusSuspended:
		http.Error(w, "organization suspended", http.StatusPaymentRequired)
		return false
	case model.OrgStatusArchived:
		http.Error(w, "organization not found", http.StatusNotFound)
		return false
	default:
		http.Error(w, "organization unavailable", http.StatusForbidden)
		return false
	}
}

const (
	orgContextKey    contextKey = "org"
	memberContextKey contextKey = "org_member"
)

// SetOrgInContext stores the organization and member in the context.
func SetOrgInContext(ctx context.Context, org *model.Organization, member *model.OrgMember) context.Context {
	ctx = context.WithValue(ctx, orgContextKey, org)
	if member != nil {
		ctx = context.WithValue(ctx, memberContextKey, member)
	}
	return ctx
}

// OrgFromContext extracts the organization from the context.
func OrgFromContext(ctx context.Context) *model.Organization {
	org, _ := ctx.Value(orgContextKey).(*model.Organization)
	return org
}

// MemberFromContext extracts the org member from the context.
func MemberFromContext(ctx context.Context) *model.OrgMember {
	m, _ := ctx.Value(memberContextKey).(*model.OrgMember)
	return m
}

// OrgMiddleware resolves the current organization and membership.
// In "single" mode it sets org_id=1 and skips member checks.
// In "multi" mode it reads {org} from the path, looks up the org, and verifies membership.
func OrgMiddleware(orgs store.OrgStore, mode string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if mode == "single" {
				org, err := orgs.GetByID(r.Context(), 1)
				if err != nil || org == nil {
					http.Error(w, "organization not found", http.StatusInternalServerError)
					return
				}
				if !enforceOrgStatus(w, org) {
					return
				}
				ctx := SetOrgInContext(r.Context(), org, nil)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			slug := r.PathValue("org")
			if slug == "" {
				http.Error(w, "organization required", http.StatusBadRequest)
				return
			}

			org, err := orgs.GetBySlug(r.Context(), slug)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if org == nil {
				http.Error(w, "organization not found", http.StatusNotFound)
				return
			}
			if !enforceOrgStatus(w, org) {
				return
			}

			userID, ok := UserIDFromContext(r.Context())
			if !ok {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}

			member, err := orgs.GetMember(r.Context(), org.ID, userID)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if member == nil {
				http.Error(w, "not a member of this organization", http.StatusForbidden)
				return
			}

			ctx := SetOrgInContext(r.Context(), org, member)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminOrgFromSlug returns middleware that resolves the org from the {slug}
// path value and injects it into the context — without checking membership
// (super-admin access was already enforced upstream). Lifecycle status IS
// checked so suspended/archived orgs aren't accidentally mutated through the
// admin API; super-admins must reactivate first if they need to.
func AdminOrgFromSlug(orgs store.OrgStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slug := r.PathValue("slug")
			if slug == "" {
				http.Error(w, "organization slug required", http.StatusBadRequest)
				return
			}
			org, err := orgs.GetBySlug(r.Context(), slug)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if org == nil {
				http.Error(w, "organization not found", http.StatusNotFound)
				return
			}
			// Super-admins reach archived orgs through /api/admin/orgs/{slug}
			// (the un-nested handler) for visibility, but mutating nested
			// resources of an archived org is suspect — block it.
			if org.Status == model.OrgStatusArchived {
				http.Error(w, "organization is archived", http.StatusNotFound)
				return
			}
			ctx := SetOrgInContext(r.Context(), org, nil)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequirePermission returns middleware that checks if the member has a given permission.
// Owners have all permissions implicitly.
func RequirePermission(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			member := MemberFromContext(r.Context())
			if member == nil {
				// Single mode has no member — allow all.
				next.ServeHTTP(w, r)
				return
			}

			if member.Role == "owner" {
				next.ServeHTTP(w, r)
				return
			}

			for _, p := range member.Permissions {
				if p == perm {
					next.ServeHTTP(w, r)
					return
				}
			}

			http.Error(w, "insufficient permissions", http.StatusForbidden)
		})
	}
}

