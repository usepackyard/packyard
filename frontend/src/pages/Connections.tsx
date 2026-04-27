import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Check, Pencil, Plus, Trash2 } from "lucide-react";

import { useAuth } from "@/hooks/useAuth";
import { useConfirm } from "@/hooks/useConfirm";
import type { ProviderAuthType, ProviderConnection } from "@/types";
import { formatDate } from "@/lib/time";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type ConnectionForm = {
  name: string;
  provider: "github" | "gitlab";
  auth_type: ProviderAuthType;
  token: string;
  host: string;
};

const emptyForm: ConnectionForm = {
  name: "",
  provider: "github",
  auth_type: "token",
  token: "",
  host: "",
};

export default function Connections() {
  const { t } = useTranslation("packages");
  const { api } = useAuth();
  const { confirm, dialog: confirmDialog } = useConfirm();
  const [connections, setConnections] = useState<ProviderConnection[]>([]);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<ProviderConnection | null>(null);
  const [form, setForm] = useState<ConnectionForm>(emptyForm);
  const [error, setError] = useState("");

  const load = useCallback(() => {
    api.listProviderConnections().then((r) => setConnections(r.connections));
  }, [api]);

  useEffect(() => { load(); }, [load]);

  const reset = () => {
    setEditing(null);
    setForm(emptyForm);
    setError("");
  };

  const startCreate = () => {
    reset();
    setOpen(true);
  };

  const startEdit = (conn: ProviderConnection) => {
    setEditing(conn);
    setForm({
      name: conn.name,
      provider: conn.provider,
      auth_type: conn.auth_type,
      token: "",
      host: conn.config?.host ?? "",
    });
    setError("");
    setOpen(true);
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    try {
      const payload = {
        name: form.name,
        provider: form.provider,
        auth_type: form.auth_type,
        token: form.token || undefined,
        config: form.provider === "gitlab" ? { host: form.host } : {},
      };
      if (editing) {
        await api.updateProviderConnection(editing.id, payload);
      } else {
        await api.createProviderConnection(payload);
      }
      setOpen(false);
      reset();
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("connections.failedSave"));
    }
  };

  const handleDelete = async (conn: ProviderConnection) => {
    const ok = await confirm({
      title: t("connections.confirmDelete.title", { name: conn.name }),
      description: t("connections.confirmDelete.description"),
      confirmLabel: t("connections.confirmDelete.confirm"),
      variant: "destructive",
    });
    if (!ok) return;
    try {
      await api.deleteProviderConnection(conn.id);
      load();
    } catch (err) {
      alert(err instanceof Error ? err.message : t("connections.failedDelete"));
    }
  };

  return (
    <div>
      {confirmDialog}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">{t("connections.title")}</h2>
          <p className="text-sm text-muted-foreground mt-1">{t("connections.subtitle")}</p>
        </div>
        <Dialog open={open} onOpenChange={(v) => { setOpen(v); if (!v) reset(); }}>
          <DialogTrigger render={<Button onClick={startCreate} />}>
            <Plus className="h-4 w-4 mr-2" />{t("connections.add")}
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{editing ? t("connections.editTitle") : t("connections.createTitle")}</DialogTitle>
            </DialogHeader>
            <form onSubmit={handleSubmit} className="space-y-4">
              {error && <div className="text-sm text-destructive bg-destructive/10 px-3 py-2 rounded-md">{error}</div>}
              <div className="space-y-2">
                <Label>{t("connections.fields.name")}</Label>
                <Input
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder={t("connections.fields.namePlaceholder")}
                  required
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>{t("connections.fields.provider")}</Label>
                  <select
                    className="flex h-9 w-full rounded-md border bg-transparent px-3 py-1 text-sm"
                    value={form.provider}
                    onChange={(e) => setForm({
                      ...form,
                      provider: e.target.value as "github" | "gitlab",
                      host: e.target.value === "gitlab" ? form.host : "",
                    })}
                  >
                    <option value="github">{t("detail.source.fields.providerGithub")}</option>
                    <option value="gitlab">{t("detail.source.fields.providerGitlab")}</option>
                  </select>
                </div>
                <div className="space-y-2">
                  <Label>{t("connections.fields.authType")}</Label>
                  <select
                    className="flex h-9 w-full rounded-md border bg-transparent px-3 py-1 text-sm"
                    value={form.auth_type}
                    onChange={(e) => setForm({ ...form, auth_type: e.target.value as ProviderAuthType, token: "" })}
                  >
                    <option value="token">{t("connections.fields.authToken")}</option>
                    <option value="none">{t("connections.fields.authNone")}</option>
                  </select>
                </div>
              </div>
              {form.provider === "gitlab" && (
                <div className="space-y-2">
                  <Label>{t("connections.fields.host")}</Label>
                  <Input
                    value={form.host}
                    onChange={(e) => setForm({ ...form, host: e.target.value })}
                    placeholder="gitlab.com"
                  />
                  <p className="text-xs text-muted-foreground">{t("connections.fields.hostHelp")}</p>
                </div>
              )}
              {form.auth_type === "token" && (
                <div className="space-y-2">
                  <Label>{t("connections.fields.token")}</Label>
                  <Input
                    type="password"
                    value={form.token}
                    onChange={(e) => setForm({ ...form, token: e.target.value })}
                    placeholder={editing ? t("connections.fields.tokenEditPlaceholder") : t("connections.fields.tokenPlaceholder")}
                    required={!editing}
                  />
                </div>
              )}
              <Button type="submit" className="w-full">
                {editing ? t("connections.save") : t("connections.create")}
              </Button>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">{t("connections.sectionTitle")}</CardTitle></CardHeader>
        <CardContent>
          {connections.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-6">{t("connections.empty")}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("connections.table.name")}</TableHead>
                  <TableHead>{t("connections.table.provider")}</TableHead>
                  <TableHead>{t("connections.table.auth")}</TableHead>
                  <TableHead>{t("connections.table.usedBy")}</TableHead>
                  <TableHead>{t("connections.table.created")}</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {connections.map((conn) => (
                  <TableRow key={conn.id}>
                    <TableCell className="font-medium">
                      <div>{conn.name}</div>
                      {conn.config?.host && (
                        <div className="text-xs text-muted-foreground">{conn.config.host}</div>
                      )}
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">{providerLabel(conn.provider)}</Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {conn.auth_type === "token" ? (
                        <span className="inline-flex items-center gap-1.5">
                          <Check className="h-3.5 w-3.5 text-green-600" />
                          {conn.token_prefix ? `${conn.token_prefix}…` : t("connections.tokenConfigured")}
                        </span>
                      ) : t("connections.noAuth")}
                    </TableCell>
                    <TableCell className="tabular-nums text-muted-foreground">{conn.source_count}</TableCell>
                    <TableCell className="text-muted-foreground">{formatDate(conn.created_at)}</TableCell>
                    <TableCell>
                      <div className="flex justify-end gap-1">
                        <Button variant="ghost" size="sm" onClick={() => startEdit(conn)}>
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={conn.source_count > 0}
                          title={conn.source_count > 0 ? t("connections.inUse") : undefined}
                          onClick={() => handleDelete(conn)}
                        >
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function providerLabel(provider: string): string {
  if (provider === "gitlab") return "GitLab";
  return "GitHub";
}
