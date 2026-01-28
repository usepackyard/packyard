import { useEffect, useState, useCallback, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/hooks/useAuth";
import type { OrgMember } from "@/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Plus, Trash2, Pencil } from "lucide-react";
import { useConfirm } from "@/hooks/useConfirm";

// Permission keys map to stable `members:<key>` translation entries.
// Keeping them as an array (not object) preserves display order.
const ALL_PERMISSIONS = [
  { key: "packages:read", labelKey: "permission.packagesRead" },
  { key: "packages:write", labelKey: "permission.packagesWrite" },
  { key: "packages:delete", labelKey: "permission.packagesDelete" },
  { key: "tokens:manage", labelKey: "permission.tokensManage" },
  { key: "sources:manage", labelKey: "permission.sourcesManage" },
  { key: "members:manage", labelKey: "permission.membersManage" },
] as const;

export default function MembersPage() {
  const { t } = useTranslation("members");
  const { api, user: currentUser } = useAuth();
  const { confirm, dialog: confirmDialog } = useConfirm();
  const [members, setMembers] = useState<OrgMember[]>([]);
  const [addOpen, setAddOpen] = useState(false);
  const [editMember, setEditMember] = useState<OrgMember | null>(null);
  const [form, setForm] = useState({ email: "", password: "", name: "", role: "member", permissions: [] as string[] });
  const [editForm, setEditForm] = useState({ role: "member", permissions: [] as string[] });
  const [error, setError] = useState("");

  const load = useCallback(() => api.listMembers().then((r) => setMembers(r.members)), [api]);
  useEffect(() => { load(); }, [load]);

  const handleAdd = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    try {
      await api.addMember(form);
      setForm({ email: "", password: "", name: "", role: "member", permissions: [] });
      setAddOpen(false);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("failedAdd"));
    }
  };

  const handleUpdate = async (e: FormEvent) => {
    e.preventDefault();
    if (!editMember) return;
    try {
      await api.updateMember(editMember.id, editForm);
      setEditMember(null);
      load();
    } catch (err) {
      alert(err instanceof Error ? err.message : t("failedUpdate"));
    }
  };

  const handleRemove = async (m: OrgMember) => {
    const ok = await confirm({
      title: t("confirmRemove.title", { name: m.user?.name || m.user?.email || "member" }),
      description: t("confirmRemove.description"),
      confirmLabel: t("confirmRemove.confirm"),
      variant: "destructive",
    });
    if (!ok) return;
    try {
      await api.removeMember(m.id);
      load();
    } catch (err) {
      alert(err instanceof Error ? err.message : t("failedRemove"));
    }
  };

  const togglePermission = (perms: string[], key: string) =>
    perms.includes(key) ? perms.filter((p) => p !== key) : [...perms, key];

  return (
    <div>
      {confirmDialog}
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold tracking-tight">{t("title")}</h2>
        <Dialog open={addOpen} onOpenChange={setAddOpen}>
          <DialogTrigger render={<Button />}>
            <Plus className="h-4 w-4 mr-2" />{t("add")}
          </DialogTrigger>
          <DialogContent>
            <DialogHeader><DialogTitle>{t("addTitle")}</DialogTitle></DialogHeader>
            <form onSubmit={handleAdd} className="space-y-4">
              {error && <div className="text-sm text-destructive">{error}</div>}
              <div className="space-y-2">
                <Label>{t("fields.email")}</Label>
                <Input type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} required />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>{t("fields.name")}</Label>
                  <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
                </div>
                <div className="space-y-2">
                  <Label>{t("fields.password")}</Label>
                  <Input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })}
                    placeholder={t("fields.passwordHelp")} />
                </div>
              </div>
              <div className="space-y-2">
                <Label>{t("fields.role")}</Label>
                <select className="flex h-9 w-full rounded-md border bg-transparent px-3 py-1 text-sm"
                  value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}>
                  <option value="owner">{t("roles.ownerAll")}</option>
                  <option value="member">{t("roles.memberCustom")}</option>
                </select>
              </div>
              {form.role === "member" && (
                <div className="space-y-2">
                  <Label>{t("fields.permissions")}</Label>
                  <div className="space-y-1">
                    {ALL_PERMISSIONS.map(({ key, labelKey }) => (
                      <label key={key} className="flex items-center gap-2 text-sm cursor-pointer">
                        <input type="checkbox" checked={form.permissions.includes(key)}
                          onChange={() => setForm({ ...form, permissions: togglePermission(form.permissions, key) })} />
                        {t(labelKey as "permission.packagesRead")}
                      </label>
                    ))}
                  </div>
                </div>
              )}
              <Button type="submit" className="w-full">{t("addButton")}</Button>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      <Dialog open={!!editMember} onOpenChange={(v) => !v && setEditMember(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {editMember
                ? t("editTitle", { name: editMember.user?.name || t("editFallback") })
                : t("editFallback")}
            </DialogTitle>
          </DialogHeader>
          <form onSubmit={handleUpdate} className="space-y-4">
            <div className="space-y-2">
              <Label>{t("fields.role")}</Label>
              <select className="flex h-9 w-full rounded-md border bg-transparent px-3 py-1 text-sm"
                value={editForm.role} onChange={(e) => setEditForm({ ...editForm, role: e.target.value })}>
                <option value="owner">{t("roles.owner")}</option>
                <option value="member">{t("roles.member")}</option>
              </select>
            </div>
            {editForm.role === "member" && (
              <div className="space-y-2">
                <Label>{t("fields.permissions")}</Label>
                <div className="space-y-1">
                  {ALL_PERMISSIONS.map(({ key, labelKey }) => (
                    <label key={key} className="flex items-center gap-2 text-sm cursor-pointer">
                      <input type="checkbox" checked={editForm.permissions.includes(key)}
                        onChange={() => setEditForm({ ...editForm, permissions: togglePermission(editForm.permissions, key) })} />
                      {t(labelKey as "permission.packagesRead")}
                    </label>
                  ))}
                </div>
              </div>
            )}
            <Button type="submit" className="w-full">{t("save")}</Button>
          </form>
        </DialogContent>
      </Dialog>

      <Card>
        <CardHeader><CardTitle className="text-base">{t("sectionTitle")}</CardTitle></CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("table.name")}</TableHead>
                <TableHead>{t("table.email")}</TableHead>
                <TableHead>{t("table.role")}</TableHead>
                <TableHead>{t("table.permissions")}</TableHead>
                <TableHead></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {members.map((m) => (
                <TableRow key={m.id}>
                  <TableCell className="font-medium">
                    {m.user?.name || "—"}
                    {m.user_id === currentUser?.id && (
                      <Badge variant="outline" className="ml-2">{t("you")}</Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-muted-foreground">{m.user?.email}</TableCell>
                  <TableCell>
                    <Badge variant={m.role === "owner" ? "default" : "secondary"}>
                      {m.role === "owner" ? t("roles.owner") : t("roles.member")}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {m.role === "owner"
                      ? t("all")
                      : (m.permissions.length > 0 ? m.permissions.join(", ") : t("none"))}
                  </TableCell>
                  <TableCell>
                    <div className="flex gap-1">
                      {m.user_id !== currentUser?.id && (
                        <>
                          <Button variant="ghost" size="sm" onClick={() => {
                            setEditMember(m);
                            setEditForm({ role: m.role, permissions: [...m.permissions] });
                          }}>
                            <Pencil className="h-4 w-4" />
                          </Button>
                          <Button variant="ghost" size="sm" onClick={() => handleRemove(m)}>
                            <Trash2 className="h-4 w-4 text-destructive" />
                          </Button>
                        </>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
