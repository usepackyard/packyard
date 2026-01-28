import { useEffect, useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { adminApi } from "@/api/client";
import type { Package, Organization } from "@/types";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Trash2 } from "lucide-react";
import { useConfirm } from "@/hooks/useConfirm";
import { formatDate } from "@/lib/time";

export default function AdminPackages() {
  const { t } = useTranslation("admin");
  const { confirm, dialog: confirmDialog } = useConfirm();
  const [packages, setPackages] = useState<Package[]>([]);
  const [orgs, setOrgs] = useState<Map<number, Organization>>(new Map());

  const load = useCallback(async () => {
    const [pkgRes, orgRes] = await Promise.all([adminApi.listPackages(), adminApi.listOrgs()]);
    setPackages(pkgRes.packages || []);
    setOrgs(new Map((orgRes.organizations || []).map((o) => [o.id, o])));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const handleDelete = async (p: Package) => {
    const ok = await confirm({
      title: t("packages.confirmDelete.title", { name: p.name }),
      description: t("packages.confirmDelete.description"),
      confirmLabel: t("packages.confirmDelete.confirm"),
      variant: "destructive",
    });
    if (!ok) return;
    await adminApi.deletePackage(p.id);
    load();
  };

  return (
    <div>
      {confirmDialog}
      <h2 className="text-2xl font-bold tracking-tight mb-6">{t("packages.title")}</h2>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {t("packages.countTitle", { count: packages.length })}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {packages.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-4">{t("packages.empty")}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("packages.table.org")}</TableHead>
                  <TableHead>{t("packages.table.package")}</TableHead>
                  <TableHead>{t("packages.table.type")}</TableHead>
                  <TableHead>{t("packages.table.created")}</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {packages.map((p) => {
                  const o = orgs.get(p.org_id);
                  return (
                    <TableRow key={p.id}>
                      <TableCell className="font-mono text-xs">{o?.slug ?? `org#${p.org_id}`}</TableCell>
                      <TableCell className="font-medium">{p.name}</TableCell>
                      <TableCell className="text-muted-foreground">{p.type}</TableCell>
                      <TableCell className="text-muted-foreground">{formatDate(p.created_at)}</TableCell>
                      <TableCell>
                        <Button variant="ghost" size="sm" onClick={() => handleDelete(p)}>
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
