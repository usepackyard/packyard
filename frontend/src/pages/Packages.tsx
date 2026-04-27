import { useEffect, useState, useCallback, type FormEvent } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/hooks/useAuth";
import { useConfirm } from "@/hooks/useConfirm";
import type { Package, ProviderConnection, SourceProvider, SourceStrategy } from "@/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Download, Pencil, Plus, Trash2 } from "lucide-react";
import { formatDateTime, formatNumber, relativeTime } from "@/lib/time";

export default function Packages() {
  const { t } = useTranslation("packages");
  const { api } = useAuth();
  const { confirm, dialog: confirmDialog } = useConfirm();
  const navigate = useNavigate();
  const [packages, setPackages] = useState<Package[]>([]);
  const [connections, setConnections] = useState<ProviderConnection[]>([]);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({
    name: "",
    type: "library",
    description: "",
    source_provider: "upload" as SourceProvider,
    connection_id: "",
    repo_owner: "",
    repo_name: "",
    strategy: "release_asset" as SourceStrategy,
    asset_pattern: "*.zip",
  });
  const [error, setError] = useState("");

  const load = useCallback(() => api.listPackages().then((r) => setPackages(r.packages)), [api]);

  useEffect(() => { load(); }, [load]);
  useEffect(() => {
    api.listProviderConnections()
      .then((r) => setConnections(r.connections))
      .catch(() => setConnections([]));
  }, [api]);

  const [searchParams, setSearchParams] = useSearchParams();
  useEffect(() => {
    if (searchParams.get("new") === "1") {
      setOpen(true);
      searchParams.delete("new");
      setSearchParams(searchParams, { replace: true });
    }
  }, [searchParams, setSearchParams]);

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    try {
      const source = form.source_provider === "upload"
        ? { provider: "upload" }
        : {
            provider: form.source_provider,
            connection_id: form.connection_id,
            config: {
              owner: form.repo_owner,
              repo: form.repo_name,
              strategy: form.strategy,
              asset_pattern: form.asset_pattern,
            },
          };
      const resp = await api.createPackage({
        name: form.name,
        type: form.type,
        description: form.description,
        source,
      });
      if (resp.webhook_secret) {
        window.alert(`${t("detail.source.webhookSecret")}: ${resp.webhook_secret}`);
      }
      setForm({
        name: "",
        type: "library",
        description: "",
        source_provider: "upload",
        connection_id: "",
        repo_owner: "",
        repo_name: "",
        strategy: "release_asset",
        asset_pattern: "*.zip",
      });
      setOpen(false);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("failedCreate"));
    }
  };

  return (
    <div>
      {confirmDialog}
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold tracking-tight">{t("title")}</h2>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger render={<Button />}>
            <Plus className="h-4 w-4 mr-2" />{t("add")}
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("createTitle")}</DialogTitle>
            </DialogHeader>
            <form onSubmit={handleCreate} className="space-y-4">
              {error && <div className="text-sm text-destructive">{error}</div>}
              <div className="space-y-2">
                <Label htmlFor="name">{t("fields.name")}</Label>
                <Input id="name" placeholder={t("fields.namePlaceholder")} value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })} required />
              </div>
              <div className="space-y-2">
                <Label htmlFor="type">{t("fields.type")}</Label>
                <select id="type" className="flex h-9 w-full rounded-md border bg-transparent px-3 py-1 text-sm"
                  value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value })}>
                  <optgroup label={t("types.groupGeneric")}>
                    <option value="library">{t("types.library")}</option>
                    <option value="project">{t("types.project")}</option>
                    <option value="metapackage">{t("types.metapackage")}</option>
                    <option value="composer-plugin">{t("types.composerPlugin")}</option>
                  </optgroup>
                  <optgroup label={t("types.groupWordpress")}>
                    <option value="wordpress-plugin">{t("types.wordpressPlugin")}</option>
                    <option value="wordpress-theme">{t("types.wordpressTheme")}</option>
                    <option value="wordpress-muplugin">{t("types.wordpressMuplugin")}</option>
                  </optgroup>
                  <optgroup label={t("types.groupFramework")}>
                    <option value="symfony-bundle">{t("types.symfonyBundle")}</option>
                    <option value="laravel-package">{t("types.laravelPackage")}</option>
                  </optgroup>
                  <optgroup label={t("types.groupCms")}>
                    <option value="drupal-module">{t("types.drupalModule")}</option>
                    <option value="drupal-theme">{t("types.drupalTheme")}</option>
                    <option value="typo3-cms-extension">{t("types.typo3Extension")}</option>
                    <option value="magento-module">{t("types.magentoModule")}</option>
                  </optgroup>
                </select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="desc">{t("fields.description")}</Label>
                <Input id="desc" value={form.description}
                  onChange={(e) => setForm({ ...form, description: e.target.value })} />
              </div>
              <div className="rounded-md border bg-muted/20 p-3 space-y-3">
                <div className="space-y-2">
                  <Label>{t("detail.source.fields.provider")}</Label>
                  <select
                    className="flex h-9 w-full rounded-md border bg-background px-3 py-1 text-sm"
                    value={form.source_provider}
                    onChange={(e) => {
                      const provider = e.target.value as SourceProvider;
                      setForm({
                        ...form,
                        source_provider: provider,
                        connection_id: "",
                        repo_owner: provider === "upload" ? "" : form.repo_owner,
                        repo_name: provider === "upload" ? "" : form.repo_name,
                        strategy: provider === "upload" ? "" : (form.strategy || "release_asset"),
                        asset_pattern: provider === "upload" ? "" : (form.asset_pattern || "*.zip"),
                      });
                    }}
                  >
                    <option value="upload">{t("detail.source.fields.providerUpload")}</option>
                    <option value="github">{t("detail.source.fields.providerGithub")}</option>
                    <option value="gitlab">{t("detail.source.fields.providerGitlab")}</option>
                  </select>
                </div>
                {form.source_provider !== "upload" && (
                  <>
                    <div className="space-y-2">
                      <Label>{t("detail.source.fields.connection")}</Label>
                      <select
                        className="flex h-9 w-full rounded-md border bg-background px-3 py-1 text-sm"
                        value={form.connection_id}
                        onChange={(e) => setForm({ ...form, connection_id: e.target.value })}
                      >
                        <option value="">{t("detail.source.fields.connectionNone")}</option>
                        {connections
                          .filter((conn) => conn.provider === form.source_provider)
                          .map((conn) => (
                            <option key={conn.id} value={conn.id}>
                              {conn.name}{conn.config?.host ? ` (${conn.config.host})` : ""}
                            </option>
                          ))}
                      </select>
                    </div>
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <Label>{t("detail.source.fields.owner")}</Label>
                        <Input
                          placeholder={t("detail.source.fields.ownerPlaceholder")}
                          value={form.repo_owner}
                          onChange={(e) => setForm({ ...form, repo_owner: e.target.value })}
                          required
                        />
                      </div>
                      <div className="space-y-2">
                        <Label>{t("detail.source.fields.repo")}</Label>
                        <Input
                          placeholder={t("detail.source.fields.repoPlaceholder")}
                          value={form.repo_name}
                          onChange={(e) => setForm({ ...form, repo_name: e.target.value })}
                          required
                        />
                      </div>
                    </div>
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <Label>{t("detail.source.fields.strategy")}</Label>
                        <select
                          className="flex h-9 w-full rounded-md border bg-background px-3 py-1 text-sm"
                          value={form.strategy}
                          onChange={(e) => setForm({ ...form, strategy: e.target.value as SourceStrategy })}
                        >
                          <option value="release_asset">{t("detail.source.fields.strategyReleaseAsset")}</option>
                          <option value="source_archive">{t("detail.source.fields.strategySourceArchive")}</option>
                        </select>
                      </div>
                      {form.strategy === "release_asset" && (
                        <div className="space-y-2">
                          <Label>{t("detail.source.fields.assetPattern")}</Label>
                          <Input
                            value={form.asset_pattern}
                            onChange={(e) => setForm({ ...form, asset_pattern: e.target.value })}
                            placeholder="*.zip"
                          />
                        </div>
                      )}
                    </div>
                  </>
                )}
              </div>
              <Button type="submit" className="w-full">{t("create")}</Button>
            </form>
          </DialogContent>
        </Dialog>
      </div>
      {packages.length === 0 ? (
        <Card><CardContent className="py-8 text-center text-muted-foreground">{t("empty")}</CardContent></Card>
      ) : (
        <Card>
          <CardHeader><CardTitle className="text-base">{t("allPackages")}</CardTitle></CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("table.package")}</TableHead>
                  <TableHead>{t("table.type")}</TableHead>
                  <TableHead className="text-right">{t("table.versions")}</TableHead>
                  <TableHead>{t("table.latest")}</TableHead>
                  <TableHead className="text-right">{t("table.downloads")}</TableHead>
                  <TableHead>{t("table.lastReleased")}</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {packages.map((pkg) => {
                  const versions = pkg.versions ?? [];
                  const totalDl = versions.reduce((sum, v) => sum + (v.download_count ?? 0), 0);
                  const latest = versions[0];
  const handleDelete = async (pkg: Package) => {
    const ok = await confirm({
      title: t("detail.confirmDelete.title", { name: pkg.name }),
      description: t("detail.confirmDelete.description"),
      confirmLabel: t("detail.confirmDelete.confirm"),
      variant: "destructive",
    });
    if (!ok) return;
    await api.deletePackage(pkg.id);
    load();
  };

  return (
                    <TableRow key={pkg.id}>
                      <TableCell>
                        <Link
                          to={`/packages/${pkg.id}`}
                          className="font-medium text-primary hover:underline block"
                        >
                          {pkg.name}
                        </Link>
                        {pkg.description && (
                          <div className="text-xs text-muted-foreground mt-0.5 line-clamp-2 max-w-md">
                            {pkg.description}
                          </div>
                        )}
                      </TableCell>
                      <TableCell><Badge variant="secondary">{pkg.type}</Badge></TableCell>
                      <TableCell className="text-right tabular-nums text-muted-foreground">
                        {versions.length}
                      </TableCell>
                      <TableCell>
                        {latest ? (
                          <code className="text-xs bg-muted px-1.5 py-0.5 rounded font-mono">
                            {latest.version}
                          </code>
                        ) : (
                          <span className="text-muted-foreground">{t("dash")}</span>
                        )}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {totalDl > 0 ? (
                          <span className="inline-flex items-center gap-1 text-muted-foreground">
                            <Download className="h-3 w-3" />
                            {formatNumber(totalDl)}
                          </span>
                        ) : (
                          <span className="text-muted-foreground">{t("dash")}</span>
                        )}
                      </TableCell>
                      <TableCell
                        className="text-muted-foreground text-xs"
                        title={latest ? formatDateTime(latest.created_at) : ""}
                      >
                        {latest ? relativeTime(latest.created_at) : t("dash")}
                      </TableCell>
                      <TableCell>
                        <div className="flex justify-end gap-1">
                          <Button variant="ghost" size="sm" onClick={() => navigate(`/packages/${pkg.id}`)}>
                            <Pencil className="h-4 w-4" />
                          </Button>
                          <Button variant="ghost" size="sm" onClick={() => handleDelete(pkg)}>
                            <Trash2 className="h-4 w-4 text-destructive" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
