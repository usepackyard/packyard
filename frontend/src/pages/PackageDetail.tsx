import { useEffect, useState, useRef, useCallback, type FormEvent } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { useAuth } from "@/hooks/useAuth";
import type { Package, PackageSource, ProviderConnection, ReleasePreview, SourceProvider, SourceStrategy, SyncJob, SyncResult, Version } from "@/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Upload, Trash2, ArrowLeft, RefreshCw, GitBranch, Copy, Check, ExternalLink, ChevronRight, MoreHorizontal, CheckCircle2, MinusCircle, AlertTriangle, XCircle, Terminal } from "lucide-react";
import { cn } from "@/lib/utils";
import { formatDate, formatDateTime, formatNumber, relativeTime } from "@/lib/time";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useConfirm } from "@/hooks/useConfirm";
import { ManualUploadDialog } from "@/components/ManualUploadDialog";

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export default function PackageDetail() {
  const { t } = useTranslation("packages");
  const { api, org, config } = useAuth();
  const { confirm, dialog: confirmDialog } = useConfirm();
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [pkg, setPkg] = useState<Package | null>(null);
  const [source, setSource] = useState<PackageSource | null>(null);
  const [webhookUrl, setWebhookUrl] = useState("");
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  // Manual-mode upload intercept: when the source is upload+manual we
  // can't just POST the zip — the backend needs a version (and
  // optional require override) that the zip can't supply. Stash the
  // file here, open the modal, then submit once the user fills in.
  const [manualUploadFile, setManualUploadFile] = useState<File | null>(null);
  // Sync state. `activeJob` drives the progress UI while a job is
  // queued/running; `syncResult` renders the categorized Sync Result card
  // once the job reaches terminal status.
  const [activeJob, setActiveJob] = useState<SyncJob | null>(null);
  const [syncResult, setSyncResult] = useState<SyncResult | null>(null);
  const syncing = activeJob !== null &&
    (activeJob.status === "queued" || activeJob.status === "running");
  const fileRef = useRef<HTMLInputElement>(null);
  const [copiedCmd, setCopiedCmd] = useState("");

  const pkgId = id ?? "";

  const load = useCallback(() => {
    api.getPackage(pkgId).then((r) => setPkg(r.package));
    api.getSource(pkgId).then((r) => {
      if (r) {
        setSource(r.source);
        setWebhookUrl(r.webhook_url);
      } else {
        setSource(null);
        setWebhookUrl("");
      }
    });
  }, [api, pkgId]);

  useEffect(() => { load(); }, [load]);

  const handleUpload = async (file: File) => {
    // Manual metadata can't be inferred from the zip — route to the
    // dialog instead so the user supplies version + optional
    // require override. Any other combination goes straight to POST.
    if (source?.provider === "upload" && source.metadata_source === "manual") {
      setManualUploadFile(file);
      return;
    }
    setUploading(true);
    try {
      await api.uploadVersion(pkgId, file);
      load();
    } catch (err) {
      alert(err instanceof Error ? err.message : t("detail.upload.failed"));
    } finally {
      setUploading(false);
    }
  };

  const handleManualUpload = async (args: { version: string; requireOverride: string }) => {
    if (!manualUploadFile) return;
    setUploading(true);
    try {
      await api.uploadVersion(pkgId, manualUploadFile, args);
      setManualUploadFile(null);
      load();
    } catch (err) {
      alert(err instanceof Error ? err.message : t("detail.upload.failed"));
    } finally {
      setUploading(false);
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    const file = e.dataTransfer.files[0];
    if (file?.name.endsWith(".zip")) handleUpload(file);
  };

  const handleDeleteVersion = async (v: Version) => {
    const ok = await confirm({
      title: t("detail.version.confirmDelete.title", { version: v.version }),
      description: t("detail.version.confirmDelete.description"),
      confirmLabel: t("detail.version.confirmDelete.confirm"),
      variant: "destructive",
    });
    if (!ok) return;
    await api.deleteVersion(v.id);
    load();
  };

  const handleDeletePackage = async () => {
    if (!pkg) return;
    const ok = await confirm({
      title: t("detail.confirmDelete.title", { name: pkg.name }),
      description: t("detail.confirmDelete.description"),
      confirmLabel: t("detail.confirmDelete.confirm"),
      variant: "destructive",
    });
    if (!ok) return;
    await api.deletePackage(pkgId);
    navigate("/packages");
  };

  // pollJob walks the job through its state machine. Backs off from 2s
  // (active) to a one-shot final read, sets syncResult on completion.
  const pollJob = useCallback(async (jobId: string) => {
    const tick = async () => {
      let res;
      try {
        res = await api.getSyncJob(pkgId, jobId);
      } catch (err) {
        console.error("poll sync job:", err);
        return; // stop polling on error; user can click Sync again
      }
      setActiveJob(res.job);
      if (res.job.status === "queued" || res.job.status === "running") {
        setTimeout(tick, 2000);
        return;
      }
      // Terminal status — unpack the full result and reload the package
      // to pick up any freshly imported versions.
      if (res.job.result_json) {
        try {
          setSyncResult(JSON.parse(res.job.result_json));
        } catch {
          setSyncResult(null);
        }
      }
      load();
    };
    tick();
  }, [api, pkgId, load]);

  const handleSync = async () => {
    setSyncResult(null);
    try {
      const { job } = await api.syncSource(pkgId);
      setActiveJob(job);
      pollJob(job.id);
    } catch (err) {
      alert(err instanceof Error ? err.message : t("detail.source.failed"));
    }
  };

  const handleRemoveSource = async () => {
    const ok = await confirm({
      title: t("detail.source.confirmRemove.title"),
      description: t("detail.source.confirmRemove.description"),
      confirmLabel: t("detail.source.confirmRemove.confirm"),
      variant: "destructive",
    });
    if (!ok) return;
    await api.deleteSource(pkgId);
    setSource(null);
    setWebhookUrl("");
  };

  if (!pkg) return <div className="text-muted-foreground">{t("common:loading", { defaultValue: "Loading…" })}</div>;

  return (
    <div>
      {confirmDialog}
      <ManualUploadDialog
        open={manualUploadFile !== null}
        filename={manualUploadFile?.name ?? null}
        baselineRequire={source?.manual_require ?? ""}
        submitting={uploading}
        onSubmit={handleManualUpload}
        onCancel={() => setManualUploadFile(null)}
      />
      <Button variant="ghost" size="sm" className="mb-4" onClick={() => navigate("/packages")}>
        <ArrowLeft className="h-4 w-4 mr-2" />{t("detail.back")}
      </Button>

      <div className="flex items-start justify-between mb-6">
        <div className="min-w-0">
          <h2 className="text-2xl font-bold tracking-tight">{pkg.name}</h2>
          {pkg.description && (
            <p className="text-muted-foreground mt-1">{pkg.description}</p>
          )}
          <div className="flex items-center gap-2 mt-2">
            <Badge variant="secondary">{pkg.type}</Badge>
            {pkg.versions && pkg.versions.length > 0 && (
              <span className="text-xs text-muted-foreground">
                {t("detail.version.total", { count: pkg.versions.length })}
              </span>
            )}
          </div>
        </div>
        <Button variant="destructive" size="sm" onClick={handleDeletePackage}>
          <Trash2 className="h-4 w-4 mr-2" />{t("detail.delete")}
        </Button>
      </div>

      <InstallCard
        pkgName={pkg.name}
        orgSlug={org?.slug ?? ""}
        baseUrl={config?.base_url ?? ""}
        copiedCmd={copiedCmd}
        onCopy={(text) => {
          navigator.clipboard.writeText(text);
          setCopiedCmd(text);
          setTimeout(() => setCopiedCmd(""), 2000);
        }}
      />

      {/* Source configuration */}
      <SourceCard
        pkgId={pkgId}
        source={source}
        webhookUrl={webhookUrl}
        syncing={syncing}
        activeJob={activeJob}
        syncResult={syncResult}
        onSync={handleSync}
        onRemove={handleRemoveSource}
        onSaved={load}
      />

      {/* Upload zone — only relevant for the upload provider (or when no
          source is configured yet, since a fresh package defaults to
          upload). Git-backed providers (GitHub, GitLab) get their
          versions through sync, so showing a drop-zone there is
          confusing. */}
      {(!source || source.provider === "upload") && (
        <Card className="mb-6">
          <CardContent className="pt-6">
            <div
              className={`border-2 border-dashed rounded-lg p-8 text-center transition-colors cursor-pointer ${
                dragOver ? "border-primary bg-primary/5" : "border-muted-foreground/25 hover:border-primary/50"
              }`}
              onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
              onDragLeave={() => setDragOver(false)}
              onDrop={handleDrop}
              onClick={() => fileRef.current?.click()}
            >
              <Upload className="h-8 w-8 mx-auto mb-3 text-muted-foreground" />
              <p className="text-sm font-medium">{uploading ? t("detail.upload.uploading") : t("detail.upload.dragOrClick")}</p>
              <p className="text-xs text-muted-foreground mt-1">{t("detail.upload.title")}</p>
              <input ref={fileRef} type="file" accept=".zip" className="hidden"
                onChange={(e) => e.target.files?.[0] && handleUpload(e.target.files[0])} />
            </div>
          </CardContent>
        </Card>
      )}

      {/* Versions */}
      <VersionsTable versions={pkg.versions ?? []} onDelete={handleDeleteVersion} />
    </div>
  );
}

// ── Versions Table ───────────────────────────────────────────────────────────

const VERSIONS_PAGE_SIZE = 20;

// VersionsTable renders the package's versions with client-side pagination.
// Versions arrive pre-sorted (newest first) so page 1 is always the most
// recent releases; no filters or re-sort controls by design — keep the
// surface small until a concrete use case pushes for more.
function VersionsTable({ versions, onDelete }: { versions: Version[]; onDelete: (v: Version) => void }) {
  const { t } = useTranslation("packages");
  const [page, setPage] = useState(0);

  const pageCount = Math.max(1, Math.ceil(versions.length / VERSIONS_PAGE_SIZE));
  const safePage = Math.min(page, pageCount - 1);
  const start = safePage * VERSIONS_PAGE_SIZE;
  const pageVersions = versions.slice(start, start + VERSIONS_PAGE_SIZE);

  if (versions.length === 0) {
    return (
      <Card>
        <CardHeader><CardTitle className="text-base">{t("detail.version.title")}</CardTitle></CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground text-center py-4">{t("detail.version.empty")}</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <CardTitle className="text-base">{t("detail.version.title")}</CardTitle>
          <span className="text-xs text-muted-foreground tabular-nums">
            {t("detail.version.total", { count: versions.length })}
          </span>
        </div>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("detail.version.table.version")}</TableHead>
              <TableHead>{t("detail.version.table.normalized")}</TableHead>
              <TableHead>{t("detail.version.table.size")}</TableHead>
              <TableHead className="text-right">{t("detail.version.table.downloads")}</TableHead>
              <TableHead>{t("detail.version.table.sha1")}</TableHead>
              <TableHead>{t("detail.version.table.released")}</TableHead>
              <TableHead></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {pageVersions.map((v) => (
              <TableRow key={v.id}>
                <TableCell className="font-medium">{v.version}</TableCell>
                <TableCell className="text-muted-foreground font-mono text-xs">{v.version_normalized}</TableCell>
                <TableCell className="text-muted-foreground">{formatSize(v.file_size)}</TableCell>
                <TableCell
                  className="text-right tabular-nums text-muted-foreground"
                  title={v.last_downloaded_at
                    ? t("detail.version.lastDownloaded", { when: formatDateTime(v.last_downloaded_at) })
                    : t("detail.version.neverDownloaded")}
                >
                  {formatNumber(v.download_count ?? 0)}
                </TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">{v.dist_sha1.slice(0, 12)}...</TableCell>
                <TableCell className="text-muted-foreground">{formatDate(v.created_at)}</TableCell>
                <TableCell>
                  <Button variant="ghost" size="sm" onClick={() => onDelete(v)}>
                    <Trash2 className="h-4 w-4 text-destructive" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>

        {pageCount > 1 && (
          <div className="flex items-center justify-between pt-3 mt-3 border-t">
            <span className="text-xs text-muted-foreground tabular-nums">
              {start + 1}–{Math.min(start + VERSIONS_PAGE_SIZE, versions.length)} / {versions.length}
            </span>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={safePage === 0}
                onClick={() => setPage(safePage - 1)}
              >
                {t("detail.version.prev")}
              </Button>
              <span className="text-xs text-muted-foreground tabular-nums px-1">
                {t("detail.version.page", { current: safePage + 1, total: pageCount })}
              </span>
              <Button
                variant="outline"
                size="sm"
                disabled={safePage >= pageCount - 1}
                onClick={() => setPage(safePage + 1)}
              >
                {t("detail.version.next")}
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

// ── Install Card ──────────────────────────────────────────────────────────────

function InstallCard({ pkgName, orgSlug, baseUrl, copiedCmd, onCopy }: {
  pkgName: string;
  orgSlug: string;
  baseUrl: string;
  copiedCmd: string;
  onCopy: (text: string) => void;
}) {
  const { t } = useTranslation("packages");

  const composerRequire = `composer require ${pkgName}`;
  const repoUrl = baseUrl && orgSlug ? `${baseUrl}/${orgSlug}` : "";

  const composerRepo = repoUrl
    ? `composer config repo.packyard composer ${repoUrl}`
    : "";

  return (
    <Card className="mb-6">
      <CardContent className="pt-5 space-y-3">
        <h3 className="text-sm font-semibold flex items-center gap-2">
          <Terminal className="h-4 w-4 text-muted-foreground" />
          {t("detail.install.title")}
        </h3>
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <code className="flex-1 rounded-md bg-muted px-3 py-2 text-sm font-mono">
              {composerRequire}
            </code>
            <Button
              variant="outline"
              size="icon"
              className="shrink-0"
              onClick={() => onCopy(composerRequire)}
              title={t("detail.install.copy")}
            >
              {copiedCmd === composerRequire
                ? <Check className="h-4 w-4 text-green-600" />
                : <Copy className="h-4 w-4" />}
            </Button>
          </div>
          {composerRepo && (
            <>
              <p className="text-xs text-muted-foreground">{t("detail.install.repoHint")}</p>
              <div className="flex items-center gap-2">
                <code className="flex-1 rounded-md bg-muted px-3 py-2 text-xs font-mono text-muted-foreground">
                  {composerRepo}
                </code>
                <Button
                  variant="outline"
                  size="icon"
                  className="shrink-0"
                  onClick={() => onCopy(composerRepo)}
                  title={t("detail.install.copy")}
                >
                  {copiedCmd === composerRepo
                    ? <Check className="h-4 w-4 text-green-600" />
                    : <Copy className="h-4 w-4" />}
                </Button>
              </div>
            </>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

// ── Source Card ──────────────────────────────────────────────────────────────

interface SourceCardProps {
  pkgId: string;
  source: PackageSource | null;
  webhookUrl: string;
  syncing: boolean;
  activeJob: SyncJob | null;
  syncResult: SyncResult | null;
  onSync: () => void;
  onRemove: () => void;
  onSaved: () => void;
}

function SourceCard({ pkgId, source, webhookUrl, syncing, activeJob, syncResult, onSync, onRemove, onSaved }: SourceCardProps) {
  const { t } = useTranslation("packages");
  const { api } = useAuth();
  const [editing, setEditing] = useState(false);
  const [connections, setConnections] = useState<ProviderConnection[]>([]);
  const [form, setForm] = useState({
    provider: "github" as SourceProvider,
    connection_id: "",
    repo_owner: "",
    repo_name: "",
    strategy: "release_asset" as SourceStrategy,
    asset_pattern: "*.zip",
    metadata_source: "from_zip",
    version_source: "auto",
    manual_require: "",
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [newSecret, setNewSecret] = useState("");
  const [copied, setCopied] = useState("");

  // Asset preview: fetch recent releases so the user can see real asset
  // names and pick a sensible pattern instead of guessing.
  const [previewing, setPreviewing] = useState(false);
  const [preview, setPreview] = useState<ReleasePreview[] | null>(null);
  const [previewError, setPreviewError] = useState("");

  useEffect(() => {
    api.listProviderConnections()
      .then((r) => setConnections(r.connections))
      .catch(() => setConnections([]));
  }, [api]);

  const isGitProvider = form.provider === "github" || form.provider === "gitlab";
  const providerConnections = connections.filter((conn) => conn.provider === form.provider);
  const needsAssetPattern = isGitProvider && form.strategy === "release_asset";
  const isManual = form.metadata_source === "manual";

  const handlePreview = async () => {
    setPreviewError("");
    setPreviewing(true);
    try {
      const res = await api.previewSource(form.provider, form.connection_id, {
        owner: form.repo_owner,
        repo: form.repo_name,
        strategy: form.strategy,
        asset_pattern: form.asset_pattern,
      });
      setPreview(res.releases);
    } catch (err) {
      setPreviewError(err instanceof Error ? err.message : t("detail.source.failed"));
    } finally {
      setPreviewing(false);
    }
  };

  const handleCopy = (text: string, key: string) => {
    navigator.clipboard.writeText(text);
    setCopied(key);
    setTimeout(() => setCopied(""), 2000);
  };

  const handleSave = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    setSaving(true);
    try {
      const res = await api.setSource(pkgId, {
        provider: form.provider,
        connection_id: form.connection_id,
        config: isGitProvider ? {
          owner: form.repo_owner,
          repo: form.repo_name,
          strategy: form.strategy,
          asset_pattern: form.asset_pattern,
        } : undefined,
        metadata_source: form.metadata_source,
        version_source: form.version_source,
        manual_require: form.manual_require,
      });
      if (res.webhook_secret) {
        setNewSecret(res.webhook_secret);
      }
      setEditing(false);
      onSaved();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("detail.source.failed"));
    } finally {
      setSaving(false);
    }
  };

  // Every package has a source (auto-provisioned on create). The
  // "add a source" empty state is obsolete — if `source` is null here
  // we're waiting on the initial fetch; render nothing rather than
  // flashing stale affordances.
  if (!source && !editing) {
    return null;
  }

  if (editing) {
    return (
      <Card className="mb-6">
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <GitBranch className="h-4 w-4" />{t("detail.source.title")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSave} className="space-y-4">
            {error && <div className="text-sm text-destructive bg-destructive/10 px-3 py-2 rounded-md">{error}</div>}
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>{t("detail.source.fields.provider")}</Label>
                <select className="flex h-9 w-full rounded-md border bg-transparent px-3 py-1 text-sm"
                  value={form.provider}
                  onChange={(e) => {
                    // Switching providers resets provider-specific
                    // fields so the saved row doesn't carry stale
                    // repo/strategy leftovers from the previous mode.
                    const nextProvider = e.target.value as SourceProvider;
                    if (nextProvider === "upload") {
                      setForm({
                        ...form,
                        provider: nextProvider,
                        connection_id: "",
                        repo_owner: "",
                        repo_name: "",
                        strategy: "",
                        asset_pattern: "",
                        version_source: form.metadata_source === "manual" ? "manual" : "composer_json",
                      });
                    } else {
                      setForm({
                        ...form,
                        provider: nextProvider,
                        connection_id: "",
                        strategy: form.strategy || "release_asset",
                        asset_pattern: form.asset_pattern || "*.zip",
                        version_source: form.metadata_source === "manual" ? "git_tag" : "auto",
                      });
                    }
                  }}>
                  <option value="upload">{t("detail.source.fields.providerUpload")}</option>
                  <option value="github">{t("detail.source.fields.providerGithub")}</option>
                  <option value="gitlab">{t("detail.source.fields.providerGitlab")}</option>
                </select>
                <p className="text-xs text-muted-foreground">{t("detail.source.fields.providerHelp")}</p>
              </div>
              {isGitProvider && (
                <div className="space-y-2">
                  <Label>{t("detail.source.fields.strategy")}</Label>
                  <select className="flex h-9 w-full rounded-md border bg-transparent px-3 py-1 text-sm"
                    value={form.strategy}
                    onChange={(e) => setForm({
                      ...form,
                      strategy: e.target.value as SourceStrategy,
                      metadata_source: e.target.value === "source_archive" ? "from_zip" : form.metadata_source,
                    })}>
                    <option value="release_asset">{strategyLabel("release_asset", t)}</option>
                    <option value="source_archive">{strategyLabel("source_archive", t)}</option>
                  </select>
                  <p className="text-xs text-muted-foreground">{t("detail.source.fields.strategyHelp")}</p>
                </div>
              )}
            </div>
            {isGitProvider && (
              <div className="space-y-2">
                <Label>{t("detail.source.fields.connection")}</Label>
                <select
                  className="flex h-9 w-full rounded-md border bg-transparent px-3 py-1 text-sm"
                  value={form.connection_id}
                  onChange={(e) => setForm({ ...form, connection_id: e.target.value })}
                >
                  <option value="">{t("detail.source.fields.connectionNone")}</option>
                  {providerConnections.map((conn) => (
                    <option key={conn.id} value={conn.id}>
                      {conn.name}{conn.config?.host ? ` (${conn.config.host})` : ""}
                    </option>
                  ))}
                </select>
                <p className="text-xs text-muted-foreground">{t("detail.source.fields.connectionHelp")}</p>
              </div>
            )}
            {/* Metadata source — visible for upload (both modes make
                sense) and for github+release_asset (manual doesn't
                apply to source_archive). */}
            {(form.provider === "upload" || form.strategy === "release_asset") && (
              <div className="space-y-2">
                <Label>{t("detail.source.fields.metadataSource")}</Label>
                <select className="flex h-9 w-full rounded-md border bg-transparent px-3 py-1 text-sm"
                  value={form.metadata_source}
                  onChange={(e) => {
                    const nextMetadata = e.target.value;
                    // Per-provider version-source coercion on metadata
                    // change keeps invalid pairs out of the form state.
                    let nextVersion = form.version_source;
                    if (nextMetadata === "manual") {
                      nextVersion = isGitProvider ? "git_tag" : "manual";
                    } else if (form.provider === "upload") {
                      nextVersion = "composer_json";
                    }
                    setForm({
                      ...form,
                      metadata_source: nextMetadata,
                      version_source: nextVersion,
                    });
                  }}>
                  <option value="from_zip">{t("detail.source.fields.metadataFromZip")}</option>
                  <option value="manual">{t("detail.source.fields.metadataManual")}</option>
                </select>
                <p className="text-xs text-muted-foreground">{t("detail.source.fields.metadataSourceHelp")}</p>
              </div>
            )}
            {/* Version source — choices depend on provider. For
                github+manual and upload+manual we don't render a
                dropdown since the value is forced by the metadata
                choice. */}
            {isGitProvider && !isManual && (
              <div className="space-y-2">
                <Label>{t("detail.source.fields.versionSource")}</Label>
                <select
                  className="flex h-9 w-full rounded-md border bg-transparent px-3 py-1 text-sm"
                  value={form.version_source}
                  onChange={(e) => setForm({ ...form, version_source: e.target.value })}
                >
                  <option value="auto">{t("detail.source.fields.versionAuto")}</option>
                  <option value="git_tag">{t("detail.source.fields.versionGitTag")}</option>
                  <option value="composer_json">{t("detail.source.fields.versionComposerJson")}</option>
                </select>
                <p className="text-xs text-muted-foreground">{t("detail.source.fields.versionSourceHelp")}</p>
              </div>
            )}
            {isGitProvider && (
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>{t("detail.source.fields.owner")}</Label>
                  <Input placeholder={t("detail.source.fields.ownerPlaceholder")} value={form.repo_owner}
                    onChange={(e) => setForm({ ...form, repo_owner: e.target.value })} required />
                </div>
                <div className="space-y-2">
                  <Label>{t("detail.source.fields.repo")}</Label>
                  <Input placeholder={t("detail.source.fields.repoPlaceholder")} value={form.repo_name}
                    onChange={(e) => setForm({ ...form, repo_name: e.target.value })} required />
                </div>
              </div>
            )}
            {needsAssetPattern && (
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label>{t("detail.source.fields.assetPattern")}</Label>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={previewing || !form.repo_owner || !form.repo_name}
                    onClick={handlePreview}
                  >
                    {previewing ? t("common:loading", { defaultValue: "Loading…" }) : t("detail.source.fields.preview")}
                  </Button>
                </div>
                <Input placeholder="*.zip" value={form.asset_pattern}
                  onChange={(e) => setForm({ ...form, asset_pattern: e.target.value })} />
                <p className="text-xs text-muted-foreground">
                  {t("detail.source.fields.assetPatternHelp")}
                </p>
                {previewError && (
                  <div className="text-xs text-destructive bg-destructive/10 px-2 py-1.5 rounded">{previewError}</div>
                )}
                {preview && preview.length === 0 && (
                  <div className="text-xs text-muted-foreground">{t("detail.source.preview.empty")}</div>
                )}
                {preview && preview.length > 0 && (() => {
                  const pattern = form.asset_pattern.trim();
                  const allAssets = preview.flatMap((r) => r.assets);
                  const matchCount = pattern
                    ? allAssets.filter((a) => matchesGlob(pattern, a.name)).length
                    : 0;
                  return (
                    <div className="space-y-2 rounded-md border bg-muted/30 p-3">
                      <div className="flex items-center justify-between gap-3">
                        <p className="text-xs font-medium text-muted-foreground">
                          {t("detail.source.preview.title")}
                        </p>
                        {pattern && allAssets.length > 0 && (
                          <p className="text-xs">
                            <span
                              className={cn(
                                "font-medium tabular-nums",
                                matchCount === 0
                                  ? "text-destructive"
                                  : "text-green-600 dark:text-green-400",
                              )}
                            >
                              {matchCount} / {allAssets.length}
                            </span>
                            <span className="text-muted-foreground">
                              {" "}{t("detail.source.preview.matchCount")}
                            </span>
                          </p>
                        )}
                      </div>
                      {preview.map((rel) => (
                        <div key={rel.tag} className="space-y-1">
                          <p className="text-xs font-mono text-muted-foreground">{rel.tag}</p>
                          {rel.assets.length === 0 ? (
                            <p className="text-xs text-muted-foreground italic pl-2">{t("detail.source.preview.noAssets")}</p>
                          ) : (
                            <ul className="space-y-1 pl-2">
                              {rel.assets.map((a) => {
                                const globs = suggestPatterns(a.name);
                                const matches = pattern ? matchesGlob(pattern, a.name) : null;
                                return (
                                  <li key={a.name} className="flex items-center gap-2 text-xs">
                                    {matches === true && (
                                      <CheckCircle2
                                        className="h-3.5 w-3.5 shrink-0 text-green-600 dark:text-green-400"
                                        aria-label={t("detail.source.preview.matches")}
                                      />
                                    )}
                                    {matches === false && (
                                      <XCircle
                                        className="h-3.5 w-3.5 shrink-0 text-muted-foreground"
                                        aria-label={t("detail.source.preview.doesNotMatch")}
                                      />
                                    )}
                                    <code
                                      className={cn(
                                        "flex-1 truncate bg-background rounded px-1.5 py-0.5",
                                        matches === false && "text-muted-foreground",
                                      )}
                                    >
                                      {a.name}
                                    </code>
                                    {globs.map((g) => (
                                      <button
                                        key={g}
                                        type="button"
                                        className="rounded-md border bg-background px-2 py-0.5 font-mono text-xs hover:bg-muted"
                                        onClick={() => setForm({ ...form, asset_pattern: g })}
                                        title={`Use as pattern: ${g}`}
                                      >
                                        {g}
                                      </button>
                                    ))}
                                  </li>
                                );
                              })}
                            </ul>
                          )}
                        </div>
                      ))}
                    </div>
                  );
                })()}
              </div>
            )}
            {isManual && (
              <div className="space-y-2">
                <Label>{t("detail.source.fields.manualRequire")}</Label>
                <textarea
                  className="flex w-full min-h-[100px] rounded-md border bg-transparent px-3 py-2 text-xs font-mono resize-y"
                  placeholder={`{"composer/installers": "^2.0"}`}
                  value={form.manual_require}
                  onChange={(e) => setForm({ ...form, manual_require: e.target.value })}
                />
                <p className="text-xs text-muted-foreground">
                  {t("detail.source.fields.manualRequireHelp")}
                </p>
              </div>
            )}
            <div className="flex gap-2">
              <Button type="submit" disabled={saving}>{saving ? t("common:loading", { defaultValue: "Saving…" }) : t("detail.source.save")}</Button>
              <Button type="button" variant="outline" onClick={() => setEditing(false)}>{t("detail.source.cancel")}</Button>
            </div>
          </form>
        </CardContent>
      </Card>
    );
  }

  // Show configured source (source is guaranteed non-null here)
  if (!source) return null;

  const sourceConfig = source.config ?? { owner: "", repo: "", strategy: "", asset_pattern: "" };
  const sourceIsGit = source.provider === "github" || source.provider === "gitlab";
  const repoURL = source.repo_url || null;
  const repoLabel = sourceIsGit
    ? `${sourceConfig.owner}/${sourceConfig.repo}`
    : t("detail.source.fields.providerUpload");
  const selectedConnection = source.connection_id
    ? connections.find((conn) => conn.id === source.connection_id)
    : null;

  const startEdit = () => {
    setForm({
      provider: source.provider,
      connection_id: source.connection_id ?? "",
      repo_owner: sourceConfig.owner,
      repo_name: sourceConfig.repo,
      strategy: sourceConfig.strategy || "release_asset",
      asset_pattern: sourceConfig.asset_pattern || "*.zip",
      metadata_source: source.metadata_source ?? "from_zip",
      version_source: source.version_source ?? "auto",
      manual_require: source.manual_require ?? "",
    });
    setEditing(true);
  };

  return (
    <Card className="mb-6">
      <CardHeader>
        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-2 min-w-0">
            <ProviderIcon provider={source.provider} />
            <div className="min-w-0">
              <CardTitle className="text-base truncate">
                {repoURL ? (
                  <a
                    href={repoURL}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="hover:underline inline-flex items-center gap-1"
                  >
                    {repoLabel}
                    <ExternalLink className="h-3 w-3 text-muted-foreground" />
                  </a>
                ) : (
                  <span>{repoLabel}</span>
                )}
              </CardTitle>
              <p className="text-xs text-muted-foreground">
                {source.last_synced_at
                  ? t("detail.source.lastSynced", { when: relativeTime(source.last_synced_at) })
                  : t("detail.source.neverSynced")}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-1 shrink-0">
            {/* Sync-now only applies to repository providers — upload-provider
                packages have no upstream to poll. */}
            {sourceIsGit && (
              <Button variant="outline" size="sm" onClick={onSync} disabled={syncing}>
                <RefreshCw className={`h-4 w-4 mr-2 ${syncing ? "animate-spin" : ""}`} />
                {syncButtonLabel(syncing, activeJob, t)}
              </Button>
            )}
            <Button variant="ghost" size="sm" onClick={startEdit}>{t("detail.source.editSource")}</Button>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <Button variant="ghost" size="icon" aria-label={t("detail.source.moreActions")}>
                    <MoreHorizontal className="h-4 w-4" />
                  </Button>
                }
              />
              <DropdownMenuContent align="end">
                <DropdownMenuItem variant="destructive" onClick={onRemove}>
                  <Trash2 className="h-3.5 w-3.5 mr-2" />
                  {t("detail.source.remove")}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
          <div className="flex items-center gap-2 min-w-0">
            <dt className="text-xs text-muted-foreground shrink-0">{t("detail.source.provider")}</dt>
            <dd className="min-w-0"><Badge variant="outline" className="text-xs">{providerLabel(source.provider, t)}</Badge></dd>
          </div>
          {selectedConnection && (
            <div className="flex items-center gap-2 min-w-0">
              <dt className="text-xs text-muted-foreground shrink-0">{t("detail.source.connection")}</dt>
              <dd className="min-w-0"><Badge variant="outline" className="text-xs">{selectedConnection.name}</Badge></dd>
            </div>
          )}
          {sourceIsGit && (
            <div className="flex items-center gap-2 min-w-0">
              <dt className="text-xs text-muted-foreground shrink-0">{t("detail.source.strategy")}</dt>
              <dd className="min-w-0"><Badge variant="outline" className="text-xs">{strategyLabel(sourceConfig.strategy, t)}</Badge></dd>
            </div>
          )}
          {(source.provider === "upload" || sourceConfig.strategy === "release_asset") && (
            <div className="flex items-center gap-2 min-w-0">
              <dt className="text-xs text-muted-foreground shrink-0">{t("detail.source.metadataSource")}</dt>
              <dd className="min-w-0"><Badge variant="outline" className="text-xs">
                {(source.metadata_source ?? "from_zip") === "manual"
                  ? t("detail.source.fields.metadataManual")
                  : t("detail.source.fields.metadataFromZip")}
              </Badge></dd>
            </div>
          )}
          {source.metadata_source !== "manual" && (
            <div className="flex items-center gap-2 min-w-0">
              <dt className="text-xs text-muted-foreground shrink-0">{t("detail.source.versionSource")}</dt>
              <dd className="min-w-0"><Badge variant="outline" className="text-xs">{versionSourceLabel(source.version_source ?? "auto", t)}</Badge></dd>
            </div>
          )}
          {sourceIsGit && sourceConfig.strategy === "release_asset" && (
            <div className="flex items-center gap-2 min-w-0">
              <dt className="text-xs text-muted-foreground shrink-0">{t("detail.source.assetPattern")}</dt>
              <dd className="min-w-0"><code className="text-xs bg-muted px-1.5 py-0.5 rounded">{sourceConfig.asset_pattern}</code></dd>
            </div>
          )}
        </dl>

        {source.metadata_source === "manual" && source.manual_require && (
          <div className="text-xs text-muted-foreground">
            <span className="font-medium text-foreground">{t("detail.source.manualRequire")}:</span>{" "}
            <code className="bg-muted px-1.5 py-0.5 rounded font-mono">
              {truncate(source.manual_require.replace(/\s+/g, " "), 80)}
            </code>
          </div>
        )}

        {webhookUrl && (
          <div className="rounded-md border bg-muted/30 p-2.5">
            <div className="flex items-center justify-between gap-2 mb-1">
              <span className="text-xs font-medium text-muted-foreground">{t("detail.source.webhookUrl")}</span>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2"
                onClick={() => handleCopy(webhookUrl, "url")}
              >
                {copied === "url" ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
              </Button>
            </div>
            <code className="text-xs font-mono text-muted-foreground break-all">{webhookUrl}</code>
          </div>
        )}

        {newSecret && (
          <div className="rounded-md border border-amber-500/40 bg-amber-50/50 dark:bg-amber-500/5 p-2.5">
            <div className="flex items-center justify-between gap-2 mb-1">
              <span className="text-xs font-medium">{t("detail.source.webhookSecret")}</span>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2"
                onClick={() => handleCopy(newSecret, "secret")}
              >
                {copied === "secret" ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
              </Button>
            </div>
            <code className="text-xs font-mono break-all">{newSecret}</code>
          </div>
        )}

        {syncResult && <SyncResultView result={syncResult} />}
      </CardContent>
    </Card>
  );
}

function providerLabel(provider: string, t: TFunction<"packages">): string {
  switch (provider) {
    case "gitlab":
      return t("detail.source.fields.providerGitlab");
    case "upload":
      return t("detail.source.fields.providerUpload");
    default:
      return t("detail.source.fields.providerGithub");
  }
}

// ProviderIcon renders a small icon for the configured provider. Lucide
// dropped its GitHub glyph upstream — using a small inline SVG keeps brand
// recognition without chasing version churn.
function ProviderIcon({ provider }: { provider: string }) {
  if (provider === "github") {
    return (
      <svg
        xmlns="http://www.w3.org/2000/svg"
        viewBox="0 0 24 24"
        fill="currentColor"
        className="h-5 w-5 text-muted-foreground"
        aria-label="GitHub"
      >
        <path d="M12 .5C5.73.5.5 5.73.5 12c0 5.08 3.29 9.39 7.86 10.92.58.1.79-.25.79-.56v-2.16c-3.2.7-3.87-1.37-3.87-1.37-.52-1.32-1.27-1.67-1.27-1.67-1.04-.71.08-.7.08-.7 1.15.08 1.76 1.18 1.76 1.18 1.02 1.75 2.68 1.25 3.33.96.1-.75.4-1.25.72-1.54-2.56-.29-5.25-1.28-5.25-5.69 0-1.26.45-2.28 1.18-3.08-.12-.29-.51-1.47.11-3.06 0 0 .97-.31 3.18 1.17a10.96 10.96 0 0 1 5.78 0c2.21-1.48 3.18-1.17 3.18-1.17.63 1.59.23 2.77.11 3.06.73.8 1.17 1.82 1.17 3.08 0 4.42-2.7 5.39-5.27 5.68.41.35.78 1.05.78 2.11v3.13c0 .31.21.67.8.56 4.56-1.53 7.85-5.84 7.85-10.92C23.5 5.73 18.27.5 12 .5Z" />
      </svg>
    );
  }
  return <GitBranch className="h-5 w-5 text-muted-foreground" />;
}

function versionSourceLabel(v: string, t: TFunction<"packages">): string {
  switch (v) {
    case "git_tag":
      return t("detail.source.fields.versionGitTag");
    case "composer_json":
      return t("detail.source.fields.versionComposerJson");
    default:
      return t("detail.source.fields.versionAuto");
  }
}

// ── Sync Result ──────────────────────────────────────────────────────────────

// SyncResultView renders sync outcomes as a status banner + stat tiles
// with expandable breakdowns. A real sync against a mismatched source
// config can produce 50+ errors; tiles surface counts at-a-glance and
// <details> sections let the user drill into root causes on demand.
function SyncResultView({ result }: { result: SyncResult }) {
  const { t } = useTranslation("packages");
  const imported = result.imported ?? [];
  const refreshed = result.refreshed ?? [];
  const skipped = result.skipped ?? [];
  const errors = result.errors ?? [];
  const [showFullErrors, setShowFullErrors] = useState(false);

  const hasAnything = imported.length + refreshed.length + skipped.length + errors.length > 0;
  if (!hasAnything) {
    return (
      <div className="mt-4 rounded-lg border bg-muted/40 px-4 py-3 text-sm">
        <p className="font-medium">{t("detail.source.syncResult.title")}</p>
        <p className="mt-1 text-xs text-muted-foreground">
          {t("detail.source.syncResult.statusEmpty")}
        </p>
      </div>
    );
  }

  // Status tone: failed if only errors, partial if errors alongside
  // successful work, success otherwise.
  const didWork = imported.length > 0 || refreshed.length > 0;
  const tone: "success" | "partial" | "failed" =
    errors.length > 0 ? (didWork ? "partial" : "failed") : "success";

  const skippedByReason = groupBy(skipped, (s) => s.reason);
  const errorsByMessage = groupBy(errors, errorMessagePart);
  const uniqueErrors = Object.entries(errorsByMessage);

  return (
    <div className="mt-4 overflow-hidden rounded-lg border bg-card">
      <div
        className={cn(
          "flex items-center gap-2 border-b px-4 py-2.5 text-sm font-medium",
          tone === "success" && "bg-green-50 text-green-900 dark:bg-green-950/30 dark:text-green-100",
          tone === "partial" && "bg-amber-50 text-amber-900 dark:bg-amber-950/30 dark:text-amber-100",
          tone === "failed" && "bg-destructive/10 text-destructive",
        )}
      >
        {tone === "success" && <CheckCircle2 className="h-4 w-4" />}
        {tone === "partial" && <AlertTriangle className="h-4 w-4" />}
        {tone === "failed" && <XCircle className="h-4 w-4" />}
        <span>
          {tone === "success" && t("detail.source.syncResult.statusSuccess")}
          {tone === "partial" && t("detail.source.syncResult.statusPartial")}
          {tone === "failed" && t("detail.source.syncResult.statusFailed")}
        </span>
      </div>

      <div className="grid grid-cols-2 gap-2 p-3 sm:grid-cols-4">
        {imported.length > 0 && (
          <StatTile
            tone="success"
            icon={<CheckCircle2 className="h-5 w-5" />}
            count={imported.length}
            label={t("detail.source.syncResult.tileImported")}
          />
        )}
        {skipped.length > 0 && (
          <StatTile
            tone="muted"
            icon={<MinusCircle className="h-5 w-5" />}
            count={skipped.length}
            label={t("detail.source.syncResult.tileSkipped")}
          />
        )}
        {errors.length > 0 && (
          <StatTile
            tone="destructive"
            icon={<AlertTriangle className="h-5 w-5" />}
            count={errors.length}
            label={t("detail.source.syncResult.tileErrors")}
          />
        )}
        {refreshed.length > 0 && (
          <StatTile
            tone="info"
            icon={<RefreshCw className="h-5 w-5" />}
            count={refreshed.length}
            label={t("detail.source.syncResult.tileRefreshed")}
            tooltip={t("detail.source.syncResult.refreshedTooltip")}
          />
        )}
      </div>

      {(skipped.length > 0 || errors.length > 0 || imported.length > 10) && (
        <div className="space-y-1 border-t px-3 py-2">
          {errors.length > 0 && (
            <DisclosureSection label={t("detail.source.syncResult.detailsErrors")} defaultOpen>
              <ul className="space-y-1 text-xs">
                {uniqueErrors.map(([msg, items]) => (
                  <li key={msg} className="flex items-baseline justify-between gap-3">
                    <span className="break-all font-mono text-destructive">{truncate(msg, 120)}</span>
                    <span className="shrink-0 tabular-nums text-muted-foreground">× {items.length}</span>
                  </li>
                ))}
              </ul>
              {errors.length > uniqueErrors.length && (
                <button
                  type="button"
                  onClick={() => setShowFullErrors((v) => !v)}
                  className="mt-2 text-xs text-muted-foreground underline hover:text-foreground"
                >
                  {showFullErrors
                    ? t("detail.source.syncResult.hideFullList")
                    : t("detail.source.syncResult.showFullList")}
                </button>
              )}
              {showFullErrors && (
                <ul className="mt-2 max-h-64 space-y-0.5 overflow-y-auto font-mono text-xs text-muted-foreground">
                  {errors.slice(0, 100).map((e, i) => (
                    <li key={i}>{e}</li>
                  ))}
                  {errors.length > 100 && (
                    <li className="italic">… {errors.length - 100}</li>
                  )}
                </ul>
              )}
            </DisclosureSection>
          )}

          {skipped.length > 0 && (
            <DisclosureSection label={t("detail.source.syncResult.detailsSkipped")}>
              <ul className="space-y-1 text-xs">
                {Object.entries(skippedByReason).map(([reason, items]) => (
                  <li key={reason} className="flex items-baseline justify-between gap-3">
                    <span className="text-foreground">{formatReason(reason)}</span>
                    <span className="shrink-0 tabular-nums text-muted-foreground">{items.length}</span>
                  </li>
                ))}
              </ul>
            </DisclosureSection>
          )}

          {imported.length > 10 && (
            <DisclosureSection label={t("detail.source.syncResult.detailsImported")}>
              <p className="break-words font-mono text-xs text-muted-foreground">
                {imported.join(", ")}
              </p>
            </DisclosureSection>
          )}
        </div>
      )}
    </div>
  );
}

function StatTile({ tone, icon, count, label, tooltip }: {
  tone: "success" | "muted" | "destructive" | "info";
  icon: React.ReactNode;
  count: number;
  label: string;
  tooltip?: string;
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-3 rounded-md border px-3 py-2.5",
        tone === "success" && "border-green-500/20 bg-green-50 dark:bg-green-950/20",
        tone === "muted" && "border-transparent bg-muted/60",
        tone === "destructive" && "border-destructive/20 bg-destructive/10",
        tone === "info" && "border-transparent bg-muted/40",
      )}
      title={tooltip}
    >
      <div
        className={cn(
          "shrink-0",
          tone === "success" && "text-green-600 dark:text-green-400",
          tone === "muted" && "text-muted-foreground",
          tone === "destructive" && "text-destructive",
          tone === "info" && "text-muted-foreground",
        )}
      >
        {icon}
      </div>
      <div className="min-w-0">
        <div className="text-lg font-semibold leading-tight tabular-nums">{count}</div>
        <div className="text-xs text-muted-foreground">{label}</div>
      </div>
    </div>
  );
}

function DisclosureSection({ label, defaultOpen = false, children }: {
  label: string;
  defaultOpen?: boolean;
  children: React.ReactNode;
}) {
  return (
    <details className="group rounded-md" open={defaultOpen}>
      <summary className="flex cursor-pointer select-none items-center gap-2 rounded-md px-2 py-1.5 text-sm font-medium hover:bg-muted/70">
        <ChevronRight className="h-4 w-4 text-muted-foreground transition-transform group-open:rotate-90" />
        {label}
      </summary>
      <div className="px-2 pb-2 pl-8 pt-1">{children}</div>
    </details>
  );
}

// errorMessagePart strips the "tag: " prefix so we can bucket errors by
// message. `v1.0.0: parse zip: composer.json not found...` → `parse zip:
// composer.json not found...`. Returns the whole string if no colon.
function errorMessagePart(err: string): string {
  const idx = err.indexOf(": ");
  return idx >= 0 ? err.slice(idx + 2) : err;
}

function groupBy<T>(items: T[], key: (item: T) => string): Record<string, T[]> {
  const out: Record<string, T[]> = {};
  for (const item of items) {
    const k = key(item);
    (out[k] ||= []).push(item);
  }
  return out;
}

function formatReason(reason: string): string {
  switch (reason) {
    case "already-exists":
      return "already exists";
    case "no-matching-asset":
      return "no matching asset";
    default:
      return reason;
  }
}

function truncate(s: string, max: number): string {
  return s.length <= max ? s : s.slice(0, max - 1) + "…";
}

// syncButtonLabel shows the live state of the sync — queued, running
// with N/M progress, or idle. Keeps the button text honest so the user
// sees why the page isn't updating.
function syncButtonLabel(syncing: boolean, job: SyncJob | null, t: TFunction<"packages">): string {
  if (!syncing || !job) return t("detail.source.sync");
  if (job.status === "queued") return t("detail.source.queued");
  if (job.progress_total > 0) {
    return t("detail.source.running", { done: job.progress_done, total: job.progress_total });
  }
  return t("detail.source.syncing");
}

function strategyLabel(strategy: string, t: TFunction<"packages">): string {
  switch (strategy) {
    case "release_asset":
      return t("detail.source.fields.strategyReleaseAsset");
    case "source_archive":
      return t("detail.source.fields.strategySourceArchive");
    default:
      return strategy;
  }
}

// suggestPatterns turns a concrete asset filename into one or two glob
// patterns the user can click-apply. Always includes the most general
// extension match (e.g. "*.zip") plus, when the name has an obvious
// version-like suffix, a stem-based pattern that matches all releases
// of the same artifact (e.g. "dlm-pro-*.zip").
// matchesGlob mirrors Go's `path/filepath.Match` semantics for the
// subset the backend actually uses against release asset names: `*`
// (any run of chars), `?` (single char), and `[...]` / `[!...]`
// character classes. Asset names don't contain path separators so the
// Go behaviour of `*` not crossing `/` is irrelevant here.
//
// Keep this in sync with `filepath.Match` in `internal/provider/sync.go`.
// If the glob is syntactically invalid (e.g. unclosed `[`), we return
// false rather than throw — the backend treats malformed patterns the
// same way, so the preview stays consistent with the real sync.
function matchesGlob(pattern: string, name: string): boolean {
  try {
    const re = globToRegex(pattern);
    return re.test(name);
  } catch {
    return false;
  }
}

function globToRegex(glob: string): RegExp {
  let out = "^";
  for (let i = 0; i < glob.length; i++) {
    const c = glob[i];
    if (c === "*") {
      out += ".*";
    } else if (c === "?") {
      out += ".";
    } else if (c === "[") {
      const end = glob.indexOf("]", i + 1);
      if (end < 0) {
        throw new Error("unclosed character class");
      }
      let cls = glob.slice(i + 1, end);
      if (cls.startsWith("!")) cls = "^" + cls.slice(1);
      out += "[" + cls + "]";
      i = end;
    } else {
      out += c.replace(/[.+^${}()|\\]/g, "\\$&");
    }
  }
  out += "$";
  return new RegExp(out);
}

function suggestPatterns(name: string): string[] {
  const patterns: string[] = [];
  const ext = extOf(name);
  if (ext) {
    patterns.push(`*${ext}`);
  }
  // Strip trailing version suffix: "-v1.2.3", "-1.2.3", "-v2.0.0-beta.47",
  // "-2026.04.15", etc. Heuristic: find last "-" followed by optional "v"
  // and a digit.
  const stem = name.replace(ext, "");
  const m = stem.match(/^(.*?)-v?\d[\w.+-]*$/);
  if (m && m[1] && m[1].length >= 3) {
    const stemPattern = `${m[1]}-*${ext}`;
    if (!patterns.includes(stemPattern)) {
      patterns.push(stemPattern);
    }
  }
  return patterns;
}

function extOf(name: string): string {
  const i = name.lastIndexOf(".");
  return i > 0 ? name.slice(i) : "";
}
