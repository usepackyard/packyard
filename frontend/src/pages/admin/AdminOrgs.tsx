import { useEffect, useState, useCallback, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { adminApi } from "@/api/client";
import type { Organization, OrgStatus } from "@/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Plus, MoreHorizontal } from "lucide-react";
import { useConfirm } from "@/hooks/useConfirm";
import { formatDate } from "@/lib/time";

const STATUS_VARIANT: Record<OrgStatus, "default" | "secondary" | "destructive"> = {
  active: "default",
  suspended: "secondary",
  archived: "destructive",
};

export default function AdminOrgs() {
  const { t } = useTranslation("admin");
  const { confirm, dialog: confirmDialog } = useConfirm();
  const [orgs, setOrgs] = useState<Organization[]>([]);
  const [open, setOpen] = useState(false);
  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [ownerEmail, setOwnerEmail] = useState("");
  const [ownerName, setOwnerName] = useState("");
  const [ownerPassword, setOwnerPassword] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(() => adminApi.listOrgs().then((r) => setOrgs(r.organizations || [])), []);
  useEffect(() => {
    load();
  }, [load]);

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    try {
      await adminApi.createOrg({ slug, name });
      if (ownerEmail.trim()) {
        await adminApi.addOrgMember(slug, {
          email: ownerEmail.trim(),
          name: ownerName.trim() || undefined,
          password: ownerPassword || undefined,
          role: "owner",
        });
      }
      setSlug("");
      setName("");
      setOwnerEmail("");
      setOwnerName("");
      setOwnerPassword("");
      setOpen(false);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("orgs.errors.failedCreate"));
    }
  };

  const handleStatus = async (org: Organization, status: OrgStatus) => {
    if (status === "archived") {
      const ok = await confirm({
        title: t("orgs.confirmArchive.title", { slug: org.slug }),
        description: t("orgs.confirmArchive.description"),
        confirmLabel: t("orgs.confirmArchive.confirm"),
        variant: "destructive",
      });
      if (!ok) return;
    }
    await adminApi.setOrgStatus(org.slug, status);
    load();
  };

  const handleDelete = async (org: Organization) => {
    const ok = await confirm({
      title: t("orgs.confirmHardDelete.title", { slug: org.slug }),
      description: t("orgs.confirmHardDelete.description"),
      confirmLabel: t("orgs.confirmHardDelete.confirm"),
      variant: "destructive",
    });
    if (!ok) return;
    try {
      await adminApi.deleteOrg(org.slug, false);
      load();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "delete failed";
      if (msg.includes("packages")) {
        const forceOk = await confirm({
          title: t("orgs.confirmForce.title", { slug: org.slug }),
          description: t("orgs.confirmForce.description"),
          confirmLabel: t("orgs.confirmForce.confirm"),
          variant: "destructive",
        });
        if (!forceOk) return;
        await adminApi.deleteOrg(org.slug, true);
        load();
      } else {
        alert(msg);
      }
    }
  };

  return (
    <div>
      {confirmDialog}
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold tracking-tight">{t("orgs.title")}</h2>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger render={<Button />}>
            <Plus className="h-4 w-4 mr-2" />{t("orgs.add")}
          </DialogTrigger>
          <DialogContent>
            <DialogHeader><DialogTitle>{t("orgs.createTitle")}</DialogTitle></DialogHeader>
            <form onSubmit={handleCreate} className="space-y-4">
              {error && (
                <div className="text-sm text-destructive bg-destructive/10 px-3 py-2 rounded-md">{error}</div>
              )}
              <div className="space-y-2">
                <Label htmlFor="slug">{t("orgs.fields.slug")}</Label>
                <Input id="slug" placeholder="acme" value={slug}
                  onChange={(e) => setSlug(e.target.value)} required pattern="[a-z][a-z0-9-]*[a-z0-9]" />
                <p className="text-xs text-muted-foreground">{t("orgs.fields.slugHelp")}</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="orgname">{t("orgs.fields.name")}</Label>
                <Input id="orgname" placeholder="Acme Inc." value={name}
                  onChange={(e) => setName(e.target.value)} required />
              </div>
              <div className="pt-2 border-t">
                <p className="text-xs font-medium text-muted-foreground mb-3">{t("orgs.fields.ownerSection")}</p>
                <div className="space-y-3">
                  <div className="space-y-2">
                    <Label htmlFor="ownerEmail">{t("orgs.fields.ownerEmail")}</Label>
                    <Input id="ownerEmail" type="email" placeholder="owner@acme.com" value={ownerEmail}
                      onChange={(e) => setOwnerEmail(e.target.value)} />
                  </div>
                  {ownerEmail.trim() && (
                    <>
                      <div className="space-y-2">
                        <Label htmlFor="ownerName">{t("orgs.fields.ownerName")}</Label>
                        <Input id="ownerName" placeholder="Jane Doe" value={ownerName}
                          onChange={(e) => setOwnerName(e.target.value)} />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="ownerPassword">{t("orgs.fields.ownerPassword")}</Label>
                        <Input id="ownerPassword" type="password" value={ownerPassword}
                          onChange={(e) => setOwnerPassword(e.target.value)} />
                        <p className="text-xs text-muted-foreground">{t("orgs.fields.ownerPasswordHelp")}</p>
                      </div>
                    </>
                  )}
                </div>
              </div>
              <Button type="submit" className="w-full">{t("orgs.create")}</Button>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">{t("orgs.all")}</CardTitle></CardHeader>
        <CardContent>
          {orgs.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-4">{t("orgs.empty")}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("orgs.table.slug")}</TableHead>
                  <TableHead>{t("orgs.table.name")}</TableHead>
                  <TableHead>{t("orgs.table.status")}</TableHead>
                  <TableHead>{t("orgs.table.created")}</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {orgs.map((o) => (
                  <TableRow key={o.id}>
                    <TableCell className="font-mono text-xs">{o.slug}</TableCell>
                    <TableCell className="font-medium">{o.name}</TableCell>
                    <TableCell>
                      <Badge variant={STATUS_VARIANT[o.status]}>{t(`orgs.status.${o.status}` as "orgs.status.active")}</Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{formatDate(o.created_at)}</TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger render={<Button variant="ghost" size="sm" />}>
                          <MoreHorizontal className="h-4 w-4" />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          {o.status !== "active" && (
                            <DropdownMenuItem onClick={() => handleStatus(o, "active")}>{t("orgs.actions.activate")}</DropdownMenuItem>
                          )}
                          {o.status !== "suspended" && (
                            <DropdownMenuItem onClick={() => handleStatus(o, "suspended")}>{t("orgs.actions.suspend")}</DropdownMenuItem>
                          )}
                          {o.status !== "archived" && (
                            <DropdownMenuItem onClick={() => handleStatus(o, "archived")}>{t("orgs.actions.archive")}</DropdownMenuItem>
                          )}
                          <DropdownMenuItem onClick={() => handleDelete(o)} className="text-destructive">
                            {t("orgs.actions.hardDelete")}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
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
