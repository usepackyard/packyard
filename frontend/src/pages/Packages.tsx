import { useEffect, useState, useCallback, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/hooks/useAuth";
import type { Package } from "@/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Download, Plus } from "lucide-react";
import { formatDateTime, formatNumber, relativeTime } from "@/lib/time";

export default function Packages() {
  const { t } = useTranslation("packages");
  const { api } = useAuth();
  const [packages, setPackages] = useState<Package[]>([]);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ name: "", type: "library", description: "" });
  const [error, setError] = useState("");

  const load = useCallback(() => api.listPackages().then((r) => setPackages(r.packages)), [api]);

  useEffect(() => { load(); }, [load]);

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
      await api.createPackage(form);
      setForm({ name: "", type: "library", description: "" });
      setOpen(false);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("failedCreate"));
    }
  };

  return (
    <div>
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
                </TableRow>
              </TableHeader>
              <TableBody>
                {packages.map((pkg) => {
                  const versions = pkg.versions ?? [];
                  const totalDl = versions.reduce((sum, v) => sum + (v.download_count ?? 0), 0);
                  const latest = versions[0];
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
