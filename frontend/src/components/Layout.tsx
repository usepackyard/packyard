import { useEffect, useState } from "react";
import { Link, useLocation, useNavigate, Outlet } from "react-router-dom";
import {
  ArrowLeft,
  Building2,
  Key,
  LayoutDashboard,
  LogOut,
  Menu,
  Package,
  Shield,
  UserCircle,
  UsersRound,
  Users,
  X,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import { useAuth } from "@/hooks/useAuth";
import { globalApi } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { LogoIcon } from "@/components/Logo";
import ContextSwitcher from "@/components/ContextSwitcher";

type NavItem = { to: string; label: string; icon: React.ComponentType<{ className?: string }> };

export default function Layout() {
  const { user, setUser, config } = useAuth();
  const { t } = useTranslation(["common", "layout"]);
  const location = useLocation();
  const navigate = useNavigate();
  const isMulti = config?.mode === "multi";
  const isAdminView = location.pathname.startsWith("/admin");

  // Mobile sidebar state. Below the md breakpoint (768px) the sidebar is
  // hidden behind a hamburger; above it, always visible. Route changes
  // auto-close the mobile drawer so clicking a nav item dismisses it.
  const [mobileOpen, setMobileOpen] = useState(false);
  useEffect(() => {
    setMobileOpen(false);
  }, [location.pathname]);

  const orgNav: NavItem[] = [
    { to: "/", label: t("layout:nav.dashboard"), icon: LayoutDashboard },
    { to: "/packages", label: t("layout:nav.packages"), icon: Package },
    { to: "/tokens", label: t("layout:nav.tokens"), icon: Key },
    isMulti
      ? { to: "/members", label: t("layout:nav.members"), icon: UsersRound }
      : { to: "/users", label: t("layout:nav.users"), icon: Users },
  ];

  const adminNav: NavItem[] = [
    { to: "/admin/orgs", label: t("layout:nav.orgs"), icon: Building2 },
    { to: "/admin/users", label: t("layout:nav.users"), icon: Users },
    { to: "/admin/packages", label: t("layout:nav.packages"), icon: Package },
    { to: "/admin/tokens", label: t("layout:nav.adminTokens"), icon: Key },
  ];

  const nav = isAdminView ? adminNav : orgNav;

  const handleLogout = async () => {
    await globalApi.logout();
    setUser(null);
    navigate("/login");
  };

  return (
    <div className="flex h-screen">
      {/* Mobile backdrop — dims the page and closes the drawer on tap. */}
      {mobileOpen && (
        <button
          type="button"
          aria-label="Close menu"
          className="md:hidden fixed inset-0 z-30 bg-black/40"
          onClick={() => setMobileOpen(false)}
        />
      )}

      <aside
        className={`
          fixed md:static inset-y-0 left-0 z-40 w-64 border-r flex flex-col
          bg-background md:bg-muted/30
          transform transition-transform duration-200
          ${mobileOpen ? "translate-x-0" : "-translate-x-full"}
          md:translate-x-0
        `}
      >
        <div className="p-4 md:p-6 flex items-center gap-3">
          <LogoIcon size={28} />
          <div className="flex-1">
            <h1 className="text-xl font-bold tracking-tight">Packyard</h1>
            <p className="text-xs text-muted-foreground">{t("layout:tagline")}</p>
          </div>
          <button
            type="button"
            aria-label={t("layout:closeMenu")}
            className="md:hidden inline-flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:bg-muted"
            onClick={() => setMobileOpen(false)}
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Context switcher — multi mode only. Handles both org switching
            and entering/exiting super-admin. */}
        {isMulti && (
          <>
            <Separator />
            <div className="p-3">
              <ContextSwitcher />
            </div>
          </>
        )}

        <Separator />
        <nav className="flex-1 p-3 space-y-1 overflow-y-auto">
          {nav.map(({ to, label, icon: Icon }) => {
            // /admin (bare) redirects/aliases to /admin/orgs conceptually,
            // so highlight "Organizations" for both paths.
            const pathForActive = location.pathname === "/admin" ? "/admin/orgs" : location.pathname;
            const active = to === "/"
              ? location.pathname === "/"
              : pathForActive.startsWith(to);
            return (
              <Link
                key={to}
                to={to}
                className={`flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors ${
                  active
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground"
                }`}
              >
                <Icon className="h-4 w-4" />
                {label}
              </Link>
            );
          })}
        </nav>

        {/* Single-mode super-admin entry/exit. Multi mode does this via the
            ContextSwitcher at the top; single mode has no switcher, so we
            surface the toggle here. */}
        {!isMulti && user?.is_super_admin && (
          <>
            <Separator />
            <div className="p-3">
              {isAdminView ? (
                <Link
                  to="/"
                  className="flex items-center gap-3 px-3 py-2 rounded-md text-sm text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
                >
                  <ArrowLeft className="h-4 w-4" />
                  {t("layout:backToDashboard")}
                </Link>
              ) : (
                <Link
                  to="/admin/orgs"
                  className="flex items-center gap-3 px-3 py-2 rounded-md text-sm border border-amber-500/40 bg-amber-50/50 dark:bg-amber-500/5 hover:bg-amber-100/60 dark:hover:bg-amber-500/10 transition-colors"
                >
                  <Shield className="h-4 w-4 text-amber-600" />
                  {t("layout:enterSuperAdmin")}
                </Link>
              )}
            </div>
          </>
        )}

        <Separator />
        <div className="p-3">
          <div className="px-3 py-2 text-sm text-muted-foreground truncate">{user?.email}</div>
          <Link
            to="/profile"
            className={`flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors ${
              location.pathname === "/profile"
                ? "bg-primary text-primary-foreground"
                : "text-muted-foreground hover:bg-muted hover:text-foreground"
            }`}
          >
            <UserCircle className="h-4 w-4" />
            {t("layout.profile")}
          </Link>
          <Button variant="ghost" size="sm" className="w-full justify-start gap-3 mt-1" onClick={handleLogout}>
            <LogOut className="h-4 w-4" />
            {t("layout.signOut")}
          </Button>
        </div>
      </aside>

      <main className="flex-1 overflow-auto min-w-0">
        {/* Mobile top bar with hamburger. Hidden on md+ where the sidebar
            is always visible. Sticky so it's reachable while scrolling. */}
        <div className="md:hidden sticky top-0 z-20 flex items-center gap-3 border-b bg-background/95 backdrop-blur px-4 h-12">
          <button
            type="button"
            aria-label={t("layout:openMenu")}
            className="inline-flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:bg-muted"
            onClick={() => setMobileOpen(true)}
          >
            <Menu className="h-4 w-4" />
          </button>
          <div className="flex items-center gap-2">
            <LogoIcon size={20} />
            <span className="text-sm font-semibold">Packyard</span>
          </div>
        </div>

        <div className="p-4 sm:p-6 lg:p-8 max-w-7xl mx-auto">
          <Outlet />
        </div>
      </main>
    </div>
  );
}
