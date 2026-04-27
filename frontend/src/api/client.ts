import type { AdminToken, AppConfig, Organization, OrgMember, OrgStatus, Package, PackageStats, ReleasePreview, Version, APIToken, PackageSource, SyncJob, User } from "@/types";
import i18n from "@/i18n";

// ApiError carries both the machine code and the raw English fallback
// from the server. Callers that want a translated string should render
// err.message; code-based branching (e.g. "show a retry button for
// failed_enqueue_sync") can inspect err.code.
export class ApiError extends Error {
  code: string;
  status: number;
  constructor(code: string, message: string, status: number) {
    super(message);
    this.code = code;
    this.status = status;
  }
}

// resolveErrorMessage translates a backend error envelope into a
// user-visible string. Priority: matching catalog entry → server's
// English `message` fallback → generic catch-all. The i18n fallbackLng
// chain handles catalogs that haven't translated this specific code
// yet (mk falls back to en automatically).
function resolveErrorMessage(code: string, englishFallback: string): string {
  const key = `errors:${code}`;
  const translated = i18n.t(key, { defaultValue: "" });
  if (translated) return translated;
  if (englishFallback) return englishFallback;
  return i18n.t("errors:generic");
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(path, {
    ...options,
    credentials: "same-origin",
    headers: {
      // CSRF defense: backend rejects unsafe methods without this header.
      // Browsers can't set custom headers cross-origin without CORS preflight.
      "X-Requested-With": "XMLHttpRequest",
      ...(options.body instanceof FormData ? {} : { "Content-Type": "application/json" }),
      ...options.headers,
    },
  });

  if (res.status === 401) {
    if (!path.includes("/api/auth/")) {
      window.location.href = "/login";
    }
    throw new ApiError("not_authenticated", resolveErrorMessage("not_authenticated", ""), 401);
  }

  if (res.status === 204) {
    return undefined as T;
  }

  const data = await res.json();
  if (!res.ok) {
    // Error envelope: prefer {code, message}; fall back to `error` for older
    // server builds that haven't been redeployed yet.
    const code = (data.code as string) || "";
    const english = (data.message as string) || (data.error as string) || "";
    throw new ApiError(code || "generic", resolveErrorMessage(code, english), res.status);
  }
  return data;
}

// Creates an org-scoped API client. All admin endpoints live under
// /api/orgs/{slug}; the slug is the only "context" the dashboard cares
// about.
export function createApi(orgSlug: string) {
  const prefix = `/api/orgs/${orgSlug}`;

  return {
    // Packages
    listPackages: () => request<{ packages: Package[] }>(`${prefix}/packages`),
    getPackageStats: () => request<PackageStats>(`${prefix}/packages/stats`),
    getPackage: (id: string) => request<{ package: Package }>(`${prefix}/packages/${id}`),
    createPackage: (data: { name: string; type: string; description: string }) =>
      request<{ package: Package }>(`${prefix}/packages`, {
        method: "POST", body: JSON.stringify(data),
      }),
    deletePackage: (id: string) => request(`${prefix}/packages/${id}`, { method: "DELETE" }),

    // Versions
    //
    // `version` is optional when the source's metadata_source is
    // from_zip (the backend reads it from composer.json) and required
    // when metadata_source is manual (no composer.json to read from).
    //
    // `requireOverride` is a JSON string {"pkg":"constraint",...} that
    // the backend merges onto the source's baseline ManualRequire. Only
    // meaningful in manual mode; harmless extra field otherwise.
    uploadVersion: (
      packageId: string,
      file: File,
      opts: { version?: string; requireOverride?: string } = {},
    ) => {
      const form = new FormData();
      form.append("file", file);
      if (opts.version) form.append("version", opts.version);
      if (opts.requireOverride) form.append("require_override", opts.requireOverride);
      return request<{ version: Version }>(`${prefix}/packages/${packageId}/versions`, {
        method: "POST", body: form,
      });
    },
    deleteVersion: (id: string) => request(`${prefix}/versions/${id}`, { method: "DELETE" }),

    // Tokens
    listTokens: () => request<{ tokens: APIToken[] }>(`${prefix}/tokens`),
    createToken: (name: string, expiresAt?: string) =>
      request<{ token: string; password: string; api_token: APIToken }>(`${prefix}/tokens`, {
        method: "POST", body: JSON.stringify({ name, expires_at: expiresAt }),
      }),
    deleteToken: (id: string) => request(`${prefix}/tokens/${id}`, { method: "DELETE" }),

    // Sources
    getSource: (packageId: string) =>
      request<{ source: PackageSource; webhook_url: string }>(`${prefix}/packages/${packageId}/source`).catch(() => null),
    setSource: (packageId: string, data: {
      provider: string; repo_owner: string; repo_name: string;
      strategy: string; asset_pattern?: string;
      metadata_source?: string; version_source?: string; manual_require?: string;
      auth_token?: string;
    }) =>
      request<{ source: PackageSource; webhook_url: string; webhook_secret?: string }>(
        `${prefix}/packages/${packageId}/source`, { method: "PUT", body: JSON.stringify(data) }
      ),
    deleteSource: (packageId: string) =>
      request(`${prefix}/packages/${packageId}/source`, { method: "DELETE" }),
    // syncSource enqueues a sync job and returns the job record — newly
    // queued (202) or the existing active one (409) with `existing:true`.
    // Both are successful outcomes from the UI's perspective, so we
    // bypass the generic request() helper (which would throw on 409) and
    // read the body directly.
    syncSource: async (packageId: string): Promise<{ job: SyncJob; existing: boolean }> => {
      const res = await fetch(`${prefix}/packages/${packageId}/source/sync`, {
        method: "POST",
        credentials: "same-origin",
        headers: {
          "Content-Type": "application/json",
          "X-Requested-With": "XMLHttpRequest",
        },
      });
      if (res.status === 401) {
        window.location.href = "/login";
        throw new Error("Unauthorized");
      }
      const data = await res.json();
      if (res.status === 202 || res.status === 409) {
        return data;
      }
      throw new Error(data.error || `sync failed: ${res.status}`);
    },
    getSyncJob: (packageId: string, jobId: string) =>
      request<{ job: SyncJob }>(`${prefix}/packages/${packageId}/sync/${jobId}`),
    listSyncJobs: (packageId: string) =>
      request<{ jobs: SyncJob[] }>(`${prefix}/packages/${packageId}/sync`),
    previewSource: (provider: string, owner: string, repo: string, authToken?: string) =>
      request<{ releases: ReleasePreview[] }>(`${prefix}/sources/preview`, {
        method: "POST",
        body: JSON.stringify({ provider, owner, repo, auth_token: authToken ?? "" }),
      }),

    // Members
    listMembers: () => request<{ members: OrgMember[] }>(`${prefix}/members`),
    addMember: (data: { email: string; password?: string; name?: string; role: string; permissions: string[] }) =>
      request<{ member: OrgMember }>(`${prefix}/members`, {
        method: "POST", body: JSON.stringify(data),
      }),
    updateMember: (id: string, data: { role: string; permissions: string[] }) =>
      request<{ member: OrgMember }>(`${prefix}/members/${id}`, {
        method: "PUT", body: JSON.stringify(data),
      }),
    removeMember: (id: string) => request(`${prefix}/members/${id}`, { method: "DELETE" }),
  };
}

// Global (non-org-scoped) API calls.
export const globalApi = {
  login: (email: string, password: string) =>
    request<{ user: User }>("/api/auth/login", {
      method: "POST", body: JSON.stringify({ email, password }),
    }),
  logout: () => request("/api/auth/logout", { method: "POST" }),
  me: () => request<{ user: User }>("/api/auth/me"),
  updateMe: (data: { name?: string; language?: string }) =>
    request<{ user: User }>("/api/auth/me", {
      method: "PUT", body: JSON.stringify(data),
    }),
  changePassword: (currentPassword: string, newPassword: string) =>
    request<void>("/api/auth/password", {
      method: "PUT",
      body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
    }),
  config: () => request<AppConfig>("/api/config"),
  listOrgs: () => request<{ organizations: Organization[] }>("/api/orgs"),
};

// Super-admin API calls (require user.is_super_admin or a Bearer admin
// token; the dashboard uses session auth, external services use Bearer).
export const adminApi = {
  // Orgs
  listOrgs: () => request<{ organizations: Organization[] }>("/api/admin/orgs"),
  getOrg: (slug: string) => request<{ organization: Organization }>(`/api/admin/orgs/${slug}`),
  createOrg: (data: { slug: string; name: string }) =>
    request<{ organization: Organization }>("/api/admin/orgs", {
      method: "POST", body: JSON.stringify(data),
    }),
  setOrgStatus: (slug: string, status: OrgStatus) =>
    request<{ organization: Organization }>(`/api/admin/orgs/${slug}/status`, {
      method: "PUT", body: JSON.stringify({ status }),
    }),
  deleteOrg: (slug: string, force: boolean) =>
    request(`/api/admin/orgs/${slug}${force ? "?force=true" : ""}`, { method: "DELETE" }),

  // Members of any org
  listOrgMembers: (slug: string) =>
    request<{ members: OrgMember[] }>(`/api/admin/orgs/${slug}/members`),
  addOrgMember: (slug: string, data: { email: string; password?: string; name?: string; role: string; permissions?: string[] }) =>
    request<{ member: OrgMember }>(`/api/admin/orgs/${slug}/members`, {
      method: "POST", body: JSON.stringify(data),
    }),
  removeOrgMember: (slug: string, memberId: string) =>
    request(`/api/admin/orgs/${slug}/members/${memberId}`, { method: "DELETE" }),

  // Global users
  listUsers: () => request<{ users: User[] }>("/api/admin/users"),
  createUser: (data: { email: string; password: string; name: string; is_super_admin?: boolean }) =>
    request<{ user: User }>("/api/admin/users", { method: "POST", body: JSON.stringify(data) }),
  deleteUser: (id: string) => request(`/api/admin/users/${id}`, { method: "DELETE" }),
  setSuperAdmin: (id: string, isSuperAdmin: boolean) =>
    request<{ user: User }>(`/api/admin/users/${id}/super-admin`, {
      method: "PUT", body: JSON.stringify({ is_super_admin: isSuperAdmin }),
    }),
  setUserPassword: (id: string, password: string) =>
    request<void>(`/api/admin/users/${id}/password`, {
      method: "PUT", body: JSON.stringify({ password }),
    }),

  // Cross-org packages
  listPackages: () => request<{ packages: Package[] }>("/api/admin/packages"),
  deletePackage: (id: string) => request(`/api/admin/packages/${id}`, { method: "DELETE" }),

  // Admin Bearer tokens
  listAdminTokens: () => request<{ tokens: AdminToken[] }>("/api/admin/tokens"),
  createAdminToken: (name: string) =>
    request<{ token: string; admin_token: AdminToken }>("/api/admin/tokens", {
      method: "POST", body: JSON.stringify({ name }),
    }),
  deleteAdminToken: (id: string) => request(`/api/admin/tokens/${id}`, { method: "DELETE" }),
};
