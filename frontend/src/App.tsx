import { useState, useEffect, useMemo, useCallback } from "react";
import { BrowserRouter, Routes, Route, Navigate, useLocation } from "react-router-dom";
import { AuthContext } from "@/hooks/useAuth";
import { globalApi, createApi } from "@/api/client";
import { applyUserLanguage } from "@/i18n";
import type { User, Organization, AppConfig } from "@/types";
import Layout from "@/components/Layout";
import Login from "@/pages/Login";
import Dashboard from "@/pages/Dashboard";
import Packages from "@/pages/Packages";
import PackageDetail from "@/pages/PackageDetail";
import Connections from "@/pages/Connections";
import Tokens from "@/pages/Tokens";
import MembersPage from "@/pages/Members";
import OrgSelector from "@/pages/OrgSelector";
import Profile from "@/pages/Profile";
import SuperAdminRoute from "@/components/SuperAdminRoute";
import AdminOrgs from "@/pages/admin/AdminOrgs";
import AdminUsers from "@/pages/admin/AdminUsers";
import AdminPackages from "@/pages/admin/AdminPackages";
import AdminTokens from "@/pages/admin/AdminTokens";

// Persist the active org slug across page reloads so refresh doesn't kick
// the user back to the org selector. Survives browser restart. We store
// only the slug (not the full org) — `orgs` is refetched on every mount
// and the slug is what we match against. Tenants never change slugs, so
// mismatches only happen when a user's membership is revoked, in which
// case we fall through to the selector naturally.
const ORG_STORAGE_KEY = "packyard:orgSlug";

function readStoredOrgSlug(): string | null {
  try {
    return localStorage.getItem(ORG_STORAGE_KEY);
  } catch {
    return null;
  }
}

function writeStoredOrgSlug(slug: string | null) {
  try {
    if (slug) localStorage.setItem(ORG_STORAGE_KEY, slug);
    else localStorage.removeItem(ORG_STORAGE_KEY);
  } catch {
    // Storage can be blocked (private mode, disabled cookies). Non-fatal —
    // the user just re-picks their org on each hard refresh.
  }
}

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [org, setOrgState] = useState<Organization | null>(null);
  const [orgs, setOrgs] = useState<Organization[]>([]);
  const [loading, setLoading] = useState(true);
  const location = useLocation();

  // Wrap setOrg so the picked slug is persisted. setOrg(null) clears it,
  // so "log out" / membership removal don't leave a stale slug behind.
  const setOrg = useCallback((next: Organization | null) => {
    setOrgState(next);
    writeStoredOrgSlug(next?.slug ?? null);
  }, []);

  useEffect(() => {
    Promise.all([globalApi.me(), globalApi.config(), globalApi.listOrgs()])
      .then(([meRes, cfgRes, orgsRes]) => {
        setUser(meRes.user);
        applyUserLanguage(meRes.user.language);
        setConfig(cfgRes);
        const list = orgsRes.organizations || [];
        setOrgs(list);

        if (list.length > 0) {
          // Prefer the last-used org from storage, else fall back to the
          // only org if there's just one (zero-click UX for the common
          // case of a single membership).
          const storedSlug = readStoredOrgSlug();
          const stored = storedSlug ? list.find((o) => o.slug === storedSlug) : null;
          if (stored) {
            setOrgState(stored);
          } else if (list.length === 1) {
            setOrgState(list[0]);
            writeStoredOrgSlug(list[0].slug);
          }
        }
      })
      .catch(() => setUser(null))
      .finally(() => setLoading(false));
  }, []);

  const api = useMemo(
    () => createApi(org?.slug || "default"),
    [org?.slug]
  );

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center text-muted-foreground">
        Loading...
      </div>
    );
  }

  if (!user) return <Navigate to="/login" replace />;

  // Admin routes are org-agnostic — super-admins operate across all orgs.
  // Don't force the org selector here; SuperAdminRoute handles access.
  const isAdminRoute = location.pathname.startsWith("/admin");

  // If no org selected AND not heading to an admin route, show the org
  // selector.
  if (!org && !isAdminRoute) {
    return (
      <AuthContext.Provider value={{ user, setUser, config, org, setOrg, orgs, setOrgs, api }}>
        <OrgSelector />
      </AuthContext.Provider>
    );
  }

  return (
    <AuthContext.Provider value={{ user, setUser, config, org, setOrg, orgs, setOrgs, api }}>
      {children}
    </AuthContext.Provider>
  );
}

export default function App() {
  const [user, setUser] = useState<User | null>(null);
  const api = useMemo(() => createApi("default"), []);

  return (
    <AuthContext.Provider value={{ user, setUser, config: null, org: null, setOrg: () => {}, orgs: [], setOrgs: () => {}, api }}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route
            element={
              <ProtectedRoute>
                <Layout />
              </ProtectedRoute>
            }
          >
            <Route path="/" element={<Dashboard />} />
            <Route path="/packages" element={<Packages />} />
            <Route path="/packages/:id" element={<PackageDetail />} />
            <Route path="/connections" element={<Connections />} />
            <Route path="/tokens" element={<Tokens />} />
            <Route path="/members" element={<MembersPage />} />
            <Route path="/profile" element={<Profile />} />
            <Route path="/admin" element={<SuperAdminRoute><AdminOrgs /></SuperAdminRoute>} />
            <Route path="/admin/orgs" element={<SuperAdminRoute><AdminOrgs /></SuperAdminRoute>} />
            <Route path="/admin/users" element={<SuperAdminRoute><AdminUsers /></SuperAdminRoute>} />
            <Route path="/admin/packages" element={<SuperAdminRoute><AdminPackages /></SuperAdminRoute>} />
            <Route path="/admin/tokens" element={<SuperAdminRoute><AdminTokens /></SuperAdminRoute>} />
          </Route>
        </Routes>
      </BrowserRouter>
    </AuthContext.Provider>
  );
}
