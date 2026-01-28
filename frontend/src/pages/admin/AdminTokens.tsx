import { useEffect, useState, useCallback, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { adminApi } from "@/api/client";
import type { AdminToken } from "@/types";
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

export default function AdminTokens() {
  const { t } = useTranslation("admin");
  const { confirm, dialog: confirmDialog } = useConfirm();
  const [tokens, setTokens] = useState<AdminToken[]>([]);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [newToken, setNewToken] = useState("");
  const [copied, setCopied] = useState(false);

  const load = useCallback(() => adminApi.listAdminTokens().then((r) => setTokens(r.tokens || [])), []);
  useEffect(() => {
    load();
  }, [load]);

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    const { token } = await adminApi.createAdminToken(name);
    setNewToken(token);
    setName("");
    load();
  };

  const handleCopy = () => {
    navigator.clipboard.writeText(newToken);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleDelete = async (tok: AdminToken) => {
    const ok = await confirm({
      title: t("tokens.confirmRevoke.title", { name: tok.name }),
      description: t("tokens.confirmRevoke.description"),
      confirmLabel: t("tokens.confirmRevoke.confirm"),
      variant: "destructive",
    });
    if (!ok) return;
    await adminApi.deleteAdminToken(tok.id);
    load();
  };

  return (
    <div>
      {confirmDialog}
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold tracking-tight">{t("tokens.title")}</h2>
        <Dialog open={open} onOpenChange={(v) => { setOpen(v); if (!v) setNewToken(""); }}>
          <DialogTrigger render={<Button />}>
            <Plus className="h-4 w-4 mr-2" />{t("tokens.add")}
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{newToken ? t("tokens.createdTitle") : t("tokens.createTitle")}</DialogTitle>
            </DialogHeader>
            {newToken ? (
              <div className="space-y-4">
                <p className="text-sm text-muted-foreground">{t("tokens.copyOnce")}</p>
                <div className="flex gap-2">
                  <Input value={newToken} readOnly className="font-mono text-xs" />
                  <Button variant="outline" size="icon" onClick={handleCopy}>
                    {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                  </Button>
                </div>
                <div className="bg-muted rounded-md p-3 overflow-hidden">
                  <p className="text-xs font-medium mb-1">{t("tokens.useAs")}</p>
                  <pre className="text-xs font-mono overflow-x-auto whitespace-pre-wrap break-all">{`Authorization: Bearer ${newToken}`}</pre>
                </div>
                <Button className="w-full" onClick={() => { setOpen(false); setNewToken(""); }}>
                  {t("tokens.done")}
                </Button>
              </div>
            ) : (
              <form onSubmit={handleCreate} className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="tokenName">{t("tokens.name")}</Label>
                  <Input id="tokenName" placeholder={t("tokens.namePlaceholder")} value={name}
                    onChange={(e) => setName(e.target.value)} required />
                  <p className="text-xs text-muted-foreground">{t("tokens.nameHelp")}</p>
                </div>
                <Button type="submit" className="w-full">{t("tokens.create")}</Button>
              </form>
            )}
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">{t("tokens.all")}</CardTitle></CardHeader>
        <CardContent>
          {tokens.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-4">{t("tokens.empty")}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("tokens.table.name")}</TableHead>
                  <TableHead>{t("tokens.table.prefix")}</TableHead>
                  <TableHead>{t("tokens.table.status")}</TableHead>
                  <TableHead>{t("tokens.table.lastUsed")}</TableHead>
                  <TableHead>{t("tokens.table.created")}</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tokens.map((tok) => (
                  <TableRow key={tok.id}>
                    <TableCell className="font-medium">{tok.name}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{tok.token_prefix}…</TableCell>
                    <TableCell>
                      <Badge variant={tok.is_active ? "default" : "secondary"}>
                        {tok.is_active ? t("tokens.status.active") : t("tokens.status.inactive")}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {tok.last_used_at ? formatDate(tok.last_used_at) : t("tokens.never")}
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
