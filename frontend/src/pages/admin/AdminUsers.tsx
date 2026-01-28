import { useEffect, useState, useCallback, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { adminApi } from "@/api/client";
import { useAuth } from "@/hooks/useAuth";
import type { User } from "@/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Plus, Trash2, Shield, ShieldOff, KeyRound } from "lucide-react";
import { useConfirm } from "@/hooks/useConfirm";
import { formatDate } from "@/lib/time";

export default function AdminUsers() {
  const { t } = useTranslation("admin");
  const { user: me } = useAuth();
  const { confirm, dialog: confirmDialog } = useConfirm();
  const [users, setUsers] = useState<User[]>([]);
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [isSuperAdmin, setIsSuperAdmin] = useState(false);
  const [error, setError] = useState("");

  const [resetTarget, setResetTarget] = useState<User | null>(null);
  const [resetPassword, setResetPassword] = useState("");
  const [resetOpen, setResetOpen] = useState(false);
  const [resetError, setResetError] = useState("");

  const load = useCallback(() => adminApi.listUsers().then((r) => setUsers(r.users || [])), []);
  useEffect(() => {
    load();
  }, [load]);

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    try {
      await adminApi.createUser({ email, password, name, is_super_admin: isSuperAdmin });
      setEmail("");
      setName("");
      setPassword("");
      setIsSuperAdmin(false);
      setOpen(false);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("users.errors.failedCreate"));
    }
  };

  const handleToggleSuperAdmin = async (u: User) => {
    if (u.id === me?.id && u.is_super_admin) {
      alert(t("users.errors.cannotRevokeSelf"));
      return;
    }
    const granting = !u.is_super_admin;
    const ok = await confirm({
      title: granting ? t("users.confirmGrant.title") : t("users.confirmRevoke.title"),
      description: granting
        ? t("users.confirmGrant.description", { email: u.email })
        : t("users.confirmRevoke.description", { email: u.email }),
      confirmLabel: granting ? t("users.confirmGrant.confirm") : t("users.confirmRevoke.confirm"),
      variant: granting ? "default" : "destructive",
    });
    if (!ok) return;
    await adminApi.setSuperAdmin(u.id, granting);
    load();
  };

  const handleResetPassword = async (e: FormEvent) => {
    e.preventDefault();
    if (!resetTarget) return;
    setResetError("");
    try {
      await adminApi.setUserPassword(resetTarget.id, resetPassword);
      setResetPassword("");
      setResetOpen(false);
      setResetTarget(null);
    } catch (err) {
      setResetError(err instanceof Error ? err.message : t("users.resetPassword.success"));
    }
  };

  const handleDelete = async (u: User) => {
    if (u.id === me?.id) {
      alert(t("users.errors.cannotDeleteSelf"));
      return;
    }
    const ok = await confirm({
      title: t("users.confirmDelete.title", { email: u.email }),
      description: t("users.confirmDelete.description"),
      confirmLabel: t("users.confirmDelete.confirm"),
      variant: "destructive",
    });
    if (!ok) return;
    await adminApi.deleteUser(u.id);
    load();
  };

  return (
    <div>
      {confirmDialog}
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold tracking-tight">{t("users.title")}</h2>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger render={<Button />}>
            <Plus className="h-4 w-4 mr-2" />{t("users.add")}
          </DialogTrigger>
          <DialogContent>
            <DialogHeader><DialogTitle>{t("users.createTitle")}</DialogTitle></DialogHeader>
            <form onSubmit={handleCreate} className="space-y-4">
              {error && (
                <div className="text-sm text-destructive bg-destructive/10 px-3 py-2 rounded-md">{error}</div>
              )}
              <div className="space-y-2">
                <Label htmlFor="email">{t("users.fields.email")}</Label>
                <Input id="email" type="email" value={email}
                  onChange={(e) => setEmail(e.target.value)} required />
              </div>
              <div className="space-y-2">
                <Label htmlFor="name">{t("users.fields.name")}</Label>
                <Input id="name" value={name} onChange={(e) => setName(e.target.value)} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="password">{t("users.fields.password")}</Label>
                <Input id="password" type="password" value={password}
                  onChange={(e) => setPassword(e.target.value)} required />
              </div>
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={isSuperAdmin}
                  onChange={(e) => setIsSuperAdmin(e.target.checked)} />
                {t("users.fields.grantSuper")}
              </label>
              <Button type="submit" className="w-full">{t("users.create")}</Button>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">{t("users.title")}</CardTitle></CardHeader>
        <CardContent>
          {users.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-4">{t("users.empty")}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("users.table.email")}</TableHead>
                  <TableHead>{t("users.table.name")}</TableHead>
                  <TableHead>{t("users.table.role")}</TableHead>
                  <TableHead>{t("users.table.created")}</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {users.map((u) => (
                  <TableRow key={u.id}>
                    <TableCell className="font-medium">{u.email}</TableCell>
                    <TableCell>{u.name}</TableCell>
                    <TableCell>
                      {u.is_super_admin ? (
                        <Badge variant="default">{t("users.role.super")}</Badge>
                      ) : (
                        <Badge variant="secondary">{t("users.role.user")}</Badge>
                      )}
                    </TableCell>
                    <TableCell className="text-muted-foreground">{formatDate(u.created_at)}</TableCell>
                    <TableCell className="flex gap-1 justify-end">
                      <Button variant="ghost" size="sm" onClick={() => handleToggleSuperAdmin(u)}
                        title={u.is_super_admin ? t("users.titles.revoke") : t("users.titles.grant")}>
                        {u.is_super_admin ? <ShieldOff className="h-4 w-4" /> : <Shield className="h-4 w-4" />}
                      </Button>
                      <Button variant="ghost" size="sm"
                        title={t("users.titles.resetPassword")}
                        onClick={() => { setResetTarget(u); setResetPassword(""); setResetError(""); setResetOpen(true); }}>
                        <KeyRound className="h-4 w-4" />
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => handleDelete(u)}>
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

      <Dialog open={resetOpen} onOpenChange={(v) => { setResetOpen(v); if (!v) setResetTarget(null); }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t("users.resetPassword.title")} — {resetTarget?.email}
            </DialogTitle>
          </DialogHeader>
          <form onSubmit={handleResetPassword} className="space-y-4">
            {resetError && (
              <div className="text-sm text-destructive bg-destructive/10 px-3 py-2 rounded-md">{resetError}</div>
            )}
            <div className="space-y-2">
              <Label htmlFor="resetPassword">{t("users.resetPassword.label")}</Label>
              <Input
                id="resetPassword"
                type="password"
                value={resetPassword}
                onChange={(e) => setResetPassword(e.target.value)}
                required
                minLength={8}
              />
            </div>
            <Button type="submit" className="w-full">{t("users.resetPassword.submit")}</Button>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
