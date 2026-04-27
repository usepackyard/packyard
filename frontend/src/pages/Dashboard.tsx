import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Trans, useTranslation } from "react-i18next";
import {
  Download,
  Key,
  Package as PackageIcon,
  Upload,
  UsersRound,
} from "lucide-react";

import { useAuth } from "@/hooks/useAuth";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import { CopyButton } from "@/components/CopyButton";
import { formatNumber, relativeTime } from "@/lib/time";
import type {
  APIToken,
  OrgMember,
  Package,
  PackageStats,
} from "@/types";

// ---------- data loading ----------

interface DashboardData {
  packages: Package[];
  tokens: APIToken[];
  members: OrgMember[];
  stats: PackageStats | null;
}

const EMPTY_STATS: PackageStats = {
  total_downloads: 0,
  downloads_last_7d: 0,
  downloads_last_30d: 0,
  top_packages: [],
  recent_downloads: [],
  daily_series_30d: [],
};

export default function Dashboard() {
  const { t } = useTranslation("dashboard");
  const { api, org, config } = useAuth();
  const [data, setData] = useState<DashboardData | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    Promise.all([
      api.listPackages(),
      api.listTokens(),
      api.listMembers(),
      api.getPackageStats().catch(() => EMPTY_STATS),
    ])
      .then(([p, t, m, s]) => {
        if (cancelled) return;
        setData({
          packages: p.packages,
          tokens: t.tokens,
          members: m.members,
          stats: s,
        });
      })
      .catch((err) => !cancelled && setError(err?.message ?? t("failedToLoad")));
    return () => {
      cancelled = true;
    };
  }, [api, t]);

  // Composer repository URL — always slug-prefixed.
  const repoURL = useMemo(() => {
    const base = config?.base_url?.replace(/\/+$/, "") ?? "";
    return org ? `${base}/${org.slug}` : base;
  }, [config?.base_url, org]);

  return (
    <div className="space-y-6">
      <DashboardHeader orgStatus={org?.status ?? "active"} />

      {error && (
        <Card>
          <CardContent className="pt-4 text-sm text-destructive">{error}</CardContent>
        </Card>
      )}

      <StatCards data={data} />

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <div className="lg:col-span-2">
          {data ? (
            data.packages.length > 0 ? (
              <RecentPackagesCard packages={data.packages} />
            ) : (
              <GettingStartedCard repoURL={repoURL} />
            )
          ) : (
            <SkeletonCard rows={4} title={t("recentPackages")} />
          )}
        </div>
        <div>
          <ComposerSnippetCard repoURL={repoURL} />
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <TopPackagesCard stats={data?.stats} />
        <RecentDownloadsCard stats={data?.stats} />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <TokenActivityCard tokens={data?.tokens ?? null} />
        <OrganizationCard org={org} members={data?.members ?? null} />
      </div>
    </div>
  );
}

// ---------- header ----------

function DashboardHeader({ orgStatus }: { orgStatus: string }) {
  const { t } = useTranslation("dashboard");
  return (
    <div className="flex items-center justify-between gap-4 flex-wrap">
      <div className="flex items-center gap-3">
        <h2 className="text-2xl font-bold tracking-tight">{t("title")}</h2>
        {orgStatus !== "active" && (
          <Badge variant="destructive" className="uppercase tracking-wide">
            {orgStatus}
          </Badge>
        )}
      </div>
      <div className="flex items-center gap-2">
        <Link
          to="/packages?new=1"
          className="inline-flex h-8 items-center gap-1.5 rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground hover:bg-primary/80 transition-colors"
        >
          <PackageIcon className="h-4 w-4" />
          {t("newPackage")}
        </Link>
        <Link
          to="/tokens?new=1"
          className="inline-flex h-8 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium bg-background hover:bg-muted transition-colors"
        >
          <Key className="h-4 w-4" />
          {t("newToken")}
        </Link>
      </div>
    </div>
  );
}

// ---------- stat cards ----------

function StatCards({ data }: { data: DashboardData | null }) {
  const { t } = useTranslation("dashboard");
  if (!data) {
    return (
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        {[0, 1, 2, 3].map((i) => (
          <Card key={i} aria-busy="true">
            <CardContent className="pt-4">
              <Skeleton className="h-10 w-10 rounded-lg mb-3" />
              <Skeleton className="h-4 w-24 mb-2" />
              <Skeleton className="h-8 w-16 mb-2" />
              <Skeleton className="h-3 w-28" />
            </CardContent>
          </Card>
        ))}
      </div>
    );
  }

  const packagesThisWeek = data.packages.filter(
    (p) => Date.now() - new Date(p.created_at).getTime() < 7 * 24 * 3600 * 1000
  ).length;

  const now = Date.now();
  const activeTokens = data.tokens.filter(
    (tok) => tok.is_active && (!tok.expires_at || new Date(tok.expires_at).getTime() > now)
  ).length;
  const expiredTokens = data.tokens.length - activeTokens;

  const owners = data.members.filter((m) => m.role === "owner").length;
  const membersCount = data.members.length - owners;

  const stats = data.stats ?? EMPTY_STATS;

  const cards = [
    {
      label: t("stats.packages"),
      value: data.packages.length,
      secondary:
        data.packages.length === 0
          ? t("stats.noPackages")
          : packagesThisWeek > 0
          ? t("stats.addedThisWeek", { count: packagesThisWeek })
          : t("stats.acrossTypes"),
      icon: PackageIcon,
      to: "/packages",
    },
    {
      label: t("stats.downloads"),
      value: stats.total_downloads,
      secondary:
        stats.total_downloads === 0
          ? t("stats.noDownloads")
          : t("stats.lastSevenDays", { count: stats.downloads_last_7d }),
      icon: Download,
      to: "#top-packages",
    },
    {
      label: t("stats.tokens"),
      value: data.tokens.length,
      secondary:
        data.tokens.length === 0
          ? t("stats.noTokens")
          : expiredTokens > 0
          ? t("stats.activeAndExpired", { active: activeTokens, expired: expiredTokens })
          : t("stats.activeOnly", { active: activeTokens }),
      icon: Key,
      to: "/tokens",
    },
    {
      label: t("stats.members"),
      value: data.members.length,
      secondary:
        data.members.length === 0
          ? t("stats.noMembers")
          : t("stats.ownersAndMembers", {
              owners: t("stats.owners", { count: owners }),
              members: t("stats.membersCount", { count: membersCount }),
            }),
      icon: UsersRound,
      to: "/members",
    },
  ];

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
      {cards.map(({ label, value, secondary, icon: Icon, to }) => (
        <Link key={label} to={to} className="block">
          <Card className="hover:ring-foreground/20 transition-shadow h-full">
            <CardContent className="pt-4">
              <div className="flex items-center justify-center h-10 w-10 rounded-lg bg-muted mb-3">
                <Icon className="h-5 w-5 text-foreground" />
              </div>
              <div className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                {label}
              </div>
              <div className="text-3xl font-bold tracking-tight mt-1">
                {formatNumber(value)}
              </div>
              <div className="text-xs text-muted-foreground mt-1">{secondary}</div>
            </CardContent>
          </Card>
        </Link>
      ))}
    </div>
  );
}

// ---------- recent packages (table or empty state) ----------

function RecentPackagesCard({ packages }: { packages: Package[] }) {
  const { t } = useTranslation("dashboard");
  const recent = [...packages]
    .sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime())
    .slice(0, 5);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("recentPackages")}</CardTitle>
      </CardHeader>
      <CardContent className="pb-4">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("table.name")}</TableHead>
              <TableHead>{t("table.type")}</TableHead>
              <TableHead className="text-right">{t("table.versions")}</TableHead>
              <TableHead className="text-right">{t("table.downloads")}</TableHead>
              <TableHead className="text-right">{t("table.updated")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {recent.map((pkg) => {
              const totalDl = (pkg.versions ?? []).reduce(
                (sum, v) => sum + (v.download_count ?? 0),
                0
              );
              return (
                <TableRow key={pkg.id}>
                  <TableCell>
                    <Link to={`/packages/${pkg.id}`} className="font-mono text-xs hover:underline">
                      {pkg.name}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <Badge variant="outline">{pkg.type}</Badge>
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {pkg.versions?.length ?? 0}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {totalDl > 0 ? (
                      <span className="inline-flex items-center gap-1 text-muted-foreground">
                        <Download className="h-3 w-3" />
                        {formatNumber(totalDl)}
                      </span>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell className="text-right text-xs text-muted-foreground">
                    {relativeTime(pkg.updated_at)}
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}

function GettingStartedCard({ repoURL }: { repoURL: string }) {
  const { t } = useTranslation("dashboard");
  const snippet = `composer config repositories.packyard composer ${repoURL || "https://repo.example.com"}`;
  const steps = [
    {
      icon: PackageIcon,
      title: t("steps.create.title"),
      body: t("steps.create.body"),
      cta: { label: t("newPackage"), to: "/packages?new=1" },
    },
    {
      icon: Upload,
      title: t("steps.upload.title"),
      body: t("steps.upload.body"),
    },
    {
      icon: Download,
      title: t("steps.install.title"),
      body: t("steps.install.body"),
    },
  ];
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("getStarted")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4 pb-4">
        {steps.map((step, idx) => {
          const Icon = step.icon;
          return (
            <div key={step.title} className="flex gap-4">
              <div className="flex flex-col items-center">
                <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary text-primary-foreground text-sm font-medium">
                  {idx + 1}
                </div>
                {idx < steps.length - 1 && <div className="flex-1 w-px bg-border mt-2" />}
              </div>
              <div className="flex-1 pb-4">
                <div className="flex items-center gap-2">
                  <Icon className="h-4 w-4 text-muted-foreground" />
                  <div className="font-medium text-sm">{step.title}</div>
                </div>
                <p className="text-sm text-muted-foreground mt-1">{step.body}</p>
                {step.cta && (
                  <Link
                    to={step.cta.to}
                    className="inline-flex h-7 items-center gap-1.5 rounded-md bg-primary px-3 mt-2 text-xs font-medium text-primary-foreground hover:bg-primary/80 transition-colors"
                  >
                    {step.cta.label}
                  </Link>
                )}
                {idx === 2 && (
                  <div className="mt-2 p-2 rounded-md bg-muted font-mono text-xs flex items-center justify-between gap-2">
                    <code className="truncate">{snippet}</code>
                    <CopyButton value={snippet} aria-label={t("copy.composerConfig")} />
                  </div>
                )}
              </div>
            </div>
          );
        })}
      </CardContent>
    </Card>
  );
}

// ---------- Composer snippet ----------

function ComposerSnippetCard({ repoURL }: { repoURL: string }) {
  const { t } = useTranslation("dashboard");
  const url = repoURL || "https://repo.example.com";
  const snippet = JSON.stringify(
    { repositories: [{ type: "composer", url }] },
    null,
    2
  );

  return (
    <Card className="lg:sticky lg:top-4">
      <CardHeader>
        <CardTitle>{t("composer.title")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 pb-4">
        <div>
          <div className="text-xs font-medium text-muted-foreground uppercase tracking-wide mb-1">
            {t("composer.url")}
          </div>
          <div className="flex items-center gap-2 rounded-md bg-muted px-2 py-1.5 font-mono text-xs">
            <span className="truncate">{url}</span>
            <CopyButton value={url} aria-label={t("copy.repoURL")} />
          </div>
        </div>
        <div>
          <div className="text-xs font-medium text-muted-foreground uppercase tracking-wide mb-1">
            {t("composer.composerJson")}
          </div>
          <div className="relative rounded-md bg-muted p-2">
            <pre className="font-mono text-xs whitespace-pre leading-relaxed overflow-x-auto">
              {snippet}
            </pre>
            <div className="absolute top-1 right-1">
              <CopyButton value={snippet} aria-label={t("copy.composerJson")} />
            </div>
          </div>
        </div>
        <p className="text-xs text-muted-foreground">
          <Trans
            i18nKey="dashboard:composer.authNote"
            components={[
              <Link key="tokens" to="/tokens" className="underline" />,
              <code key="sat" className="font-mono" />,
            ]}
          />
        </p>
      </CardContent>
    </Card>
  );
}

// ---------- top packages ----------

function TopPackagesCard({ stats }: { stats: PackageStats | null | undefined }) {
  const { t } = useTranslation("dashboard");
  if (stats === undefined) {
    return <SkeletonCard rows={3} title={t("topPackages.title")} />;
  }
  const max = stats?.top_packages?.[0]?.count ?? 0;
  return (
    <Card id="top-packages">
      <CardHeader>
        <CardTitle>{t("topPackages.title")}</CardTitle>
      </CardHeader>
      <CardContent className="pb-4">
        {!stats || stats.top_packages.length === 0 ? (
          <p className="text-sm text-muted-foreground py-2">{t("topPackages.empty")}</p>
        ) : (
          <div className="space-y-2.5">
            {stats.top_packages.map((row, idx) => {
              const pct = max > 0 ? (row.count / max) * 100 : 0;
              return (
                <div key={row.package_id} className="space-y-1">
                  <div className="flex items-center justify-between gap-2 text-sm">
                    <div className="flex items-center gap-2 min-w-0">
                      <span className="flex h-5 w-5 items-center justify-center rounded-full bg-muted text-xs font-medium text-muted-foreground shrink-0">
                        {idx + 1}
                      </span>
                      <Link
                        to={`/packages/${row.package_id}`}
                        className="font-mono text-xs truncate hover:underline"
                      >
                        {row.package_name}
                      </Link>
                    </div>
                    <span className="tabular-nums text-muted-foreground">
                      {formatNumber(row.count)}
                    </span>
                  </div>
                  <div className="h-1.5 rounded-full bg-muted overflow-hidden">
                    <div
                      className="h-full bg-primary/60"
                      style={{ width: `${pct}%` }}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

// ---------- recent downloads ----------

function RecentDownloadsCard({ stats }: { stats: PackageStats | null | undefined }) {
  const { t } = useTranslation("dashboard");
  if (stats === undefined) {
    return <SkeletonCard rows={4} title={t("recentDownloads.title")} />;
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("recentDownloads.title")}</CardTitle>
      </CardHeader>
      <CardContent className="pb-4">
        {!stats || stats.recent_downloads.length === 0 ? (
          <p className="text-sm text-muted-foreground py-2">{t("recentDownloads.empty")}</p>
        ) : (
          <ul className="space-y-1.5 text-sm">
            {stats.recent_downloads.map((ev, i) => (
              <li key={`${ev.at}-${i}`} className="flex items-center justify-between gap-2">
                <span className="truncate min-w-0">
                  <Link to={`/packages/${ev.package_id}`} className="font-mono text-xs hover:underline">
                    {ev.package_name}
                  </Link>
                  <span className="font-mono text-xs text-muted-foreground ml-1">@{ev.version}</span>
                </span>
                <span className="text-xs text-muted-foreground shrink-0">
                  {relativeTime(ev.at)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

// ---------- token activity ----------

function TokenActivityCard({ tokens }: { tokens: APIToken[] | null }) {
  const { t } = useTranslation("dashboard");
  if (tokens === null) {
    return <SkeletonCard rows={3} title={t("tokenActivity.title")} />;
  }
  const used = [...tokens]
    .filter((t) => t.last_used_at)
    .sort((a, b) => new Date(b.last_used_at!).getTime() - new Date(a.last_used_at!).getTime())
    .slice(0, 5);
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("tokenActivity.title")}</CardTitle>
      </CardHeader>
      <CardContent className="pb-4">
        {used.length === 0 ? (
          <p className="text-sm text-muted-foreground py-2">{t("tokenActivity.empty")}</p>
        ) : (
          <ul className="space-y-1.5 text-sm">
            {used.map((tok) => (
              <li key={tok.id} className="flex items-center justify-between gap-2">
                <span className="flex items-center gap-2 min-w-0">
                  <code className="font-mono text-xs rounded bg-muted px-1.5 py-0.5 shrink-0">
                    {tok.token_prefix}
                  </code>
                  <span className="truncate text-muted-foreground">{tok.name}</span>
                </span>
                <span className="text-xs text-muted-foreground shrink-0">
                  {relativeTime(tok.last_used_at)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

// ---------- organization ----------

function OrganizationCard({
  org,
  members,
}: {
  org: { slug: string; name: string; status: string } | null;
  members: OrgMember[] | null;
}) {
  const { t } = useTranslation("dashboard");
  if (!org || members === null) {
    return <SkeletonCard rows={2} title={t("organization.title")} />;
  }
  const statusVariant = org.status === "active" ? "secondary" : "destructive";
  const shown = members.slice(0, 6);
  const extra = Math.max(0, members.length - shown.length);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("organization.title")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 pb-4">
        <div className="flex items-center gap-2">
          <span className="font-medium text-sm">{org.name}</span>
          <Badge variant="outline" className="font-mono text-xs">{org.slug}</Badge>
          <Badge variant={statusVariant} className="capitalize">{org.status}</Badge>
        </div>
        <div>
          <div className="text-xs font-medium text-muted-foreground uppercase tracking-wide mb-2">
            {t("organization.members")}
          </div>
          {members.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("organization.empty")}</p>
          ) : (
            <div className="flex flex-wrap items-center gap-1.5">
              {shown.map((m) => (
                <InitialAvatar key={m.id} name={m.user?.name ?? m.user?.email ?? "?"} />
              ))}
              {extra > 0 && (
                <span className="flex h-7 items-center rounded-full bg-muted px-2 text-xs font-medium text-muted-foreground">
                  +{extra}
                </span>
              )}
              <Link to="/members" className="text-xs text-muted-foreground hover:underline ml-1">
                {t("organization.manage")}
              </Link>
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

function InitialAvatar({ name }: { name: string }) {
  const initials = name
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase() ?? "")
    .join("") || name[0]?.toUpperCase() || "?";
  return (
    <span
      className="flex h-7 w-7 items-center justify-center rounded-full bg-primary/10 text-xs font-medium text-primary"
      title={name}
    >
      {initials}
    </span>
  );
}

// ---------- generic skeleton card ----------

function SkeletonCard({ rows, title }: { rows: number; title: string }) {
  return (
    <Card aria-busy="true">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2 pb-4">
        {Array.from({ length: rows }).map((_, i) => (
          <Skeleton key={i} className="h-4 w-full" />
        ))}
      </CardContent>
    </Card>
  );
}
