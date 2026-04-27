import { useEffect, useState, useCallback, type FormEvent } from "react";
import { useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/hooks/useAuth";
import type { APIToken } from "@/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Plus, Trash2, Copy, Check } from "lucide-react";
import { useConfirm } from "@/hooks/useConfirm";
import { formatDate } from "@/lib/time";

export default function Tokens() {
  const { t } = useTranslation("tokens");
  const { api, config, org } = useAuth();
  const { confirm, dialog: confirmDialog } = useConfirm();
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [newToken, setNewToken] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [copied, setCopied] = useState(false);

  const load = useCallback(() => api.listTokens().then((r) => setTokens(r.tokens)), [api]);
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
    const resp = await api.createToken(name);
    setNewToken(resp.token);
    setNewPassword(resp.password);
    setName("");
    load();
  };

  const handleCopy = () => {
    navigator.clipboard.writeText(newToken);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleDelete = async (tok: APIToken) => {
    const ok = await confirm({
      title: t("confirmRevoke.title", { name: tok.name }),
      description: t("confirmRevoke.description"),
      confirmLabel: t("confirmRevoke.confirm"),
      variant: "destructive",
    });
    if (!ok) return;
    await api.deleteToken(tok.id);
    load();
  };

  return (
    <div>
      {confirmDialog}
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold tracking-tight">{t("title")}</h2>
        <Dialog open={open} onOpenChange={(v) => { setOpen(v); if (!v) { setNewToken(""); setNewPassword(""); } }}>
          <DialogTrigger render={<Button />}>
            <Plus className="h-4 w-4 mr-2" />{t("add")}
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{newToken ? t("createdTitle") : t("createTitle")}</DialogTitle>
            </DialogHeader>
            {newToken ? (
              <div className="space-y-4">
                <p className="text-sm text-muted-foreground">{t("copyOnce")}</p>
                <div className="flex gap-2">
                  <Input value={newToken} readOnly className="font-mono text-xs" />
                  <Button variant="outline" size="icon" onClick={handleCopy}>
                    {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                  </Button>
                </div>
                {(() => {
                  const base = (config?.base_url || "").replace(/\/+$/, "");
                  const repoURL = org ? `${base}/${org.slug}` : base;
                  let hostKey = "your-repo.example.com";
                  try {
                    const u = new URL(config?.base_url || "");
                    hostKey = org ? `${u.host}/${org.slug}` : u.host;
                  } catch { /* keep fallback */ }
                  return (
                    <>
                      <div>
                        <p className="text-xs font-medium mb-2">{t("composerCommand")}</p>
                        <pre className="bg-muted rounded-md p-3 text-xs font-mono overflow-x-auto whitespace-pre-wrap break-all">{`composer config --auth http-basic.${hostKey} ${newToken} ${newPassword}`}</pre>
                      </div>
                      <div>
                        <p className="text-xs font-medium mb-2 text-muted-foreground">{t("composerAuthJson")}</p>
                        <pre className="bg-muted rounded-md p-3 text-xs font-mono overflow-x-auto whitespace-pre-wrap break-all text-muted-foreground">{`{
  "http-basic": {
    "${hostKey}": {
      "username": "${newToken}",
      "password": "${newPassword}"
    }
  }
}`}</pre>
                      </div>
                      <div className="rounded-md border border-dashed p-3">
                        <p className="text-xs text-muted-foreground mb-1">{t("composerRepoHint")}</p>
                        <code className="text-xs font-mono break-all">{`composer config repositories.packyard composer ${repoURL}`}</code>
                      </div>
                    </>
                  );
                })()}
                <Button className="w-full" onClick={() => { setOpen(false); setNewToken(""); setNewPassword(""); }}>
                  {t("done")}
                </Button>
              </div>
            ) : (
              <form onSubmit={handleCreate} className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="tokenName">{t("name")}</Label>
                  <Input id="tokenName" placeholder={t("namePlaceholder")} value={name}
                    onChange={(e) => setName(e.target.value)} required />
                </div>
                <Button type="submit" className="w-full">{t("create")}</Button>
              </form>
            )}
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">{t("title")}</CardTitle></CardHeader>
        <CardContent>
          {tokens.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-4">{t("noTokens")}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("table.name")}</TableHead>
                  <TableHead>{t("table.prefix")}</TableHead>
                  <TableHead>{t("table.status")}</TableHead>
                  <TableHead>{t("table.lastUsed")}</TableHead>
                  <TableHead>{t("table.created")}</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tokens.map((tok) => (
                  <TableRow key={tok.id}>
                    <TableCell className="font-medium">{tok.name}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{tok.token_prefix}...</TableCell>
                    <TableCell>
                      <Badge variant={tok.is_active ? "default" : "secondary"}>
                        {tok.is_active ? t("status.active") : t("status.inactive")}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {tok.last_used_at ? formatDate(tok.last_used_at) : t("never")}
                    </TableCell>
                    <TableCell className="text-muted-foreground">{formatDate(tok.created_at)}</TableCell>
                    <TableCell>
                      <Button variant="ghost" size="sm" onClick={() => handleDelete(tok)}>
                        <Trash2 className="h-4 w-4 text-destructive" />
                      </Button>
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
