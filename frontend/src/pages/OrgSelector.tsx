import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/hooks/useAuth";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { buttonVariants } from "@/components/ui/button";
import { Building2, Shield } from "lucide-react";

export default function OrgSelector() {
  const { t } = useTranslation("orgSelector");
  const { user, orgs, setOrg } = useAuth();

  return (
    <div className="min-h-screen flex items-center justify-center bg-muted/30">
      <Card className="w-full max-w-sm">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl font-bold">{t("title")}</CardTitle>
          <CardDescription>{t("subtitle")}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {orgs.map((org) => (
              <button
                key={org.id}
                onClick={() => setOrg(org)}
                className="flex items-center gap-3 w-full p-3 rounded-md border text-left hover:bg-muted transition-colors"
              >
                <Building2 className="h-5 w-5 text-muted-foreground" />
                <div>
                  <div className="font-medium text-sm">{org.name}</div>
                  <div className="text-xs text-muted-foreground">{org.slug}</div>
                </div>
              </button>
            ))}
            {orgs.length === 0 && (
              <div className="text-center py-4 space-y-3">
                <p className="text-sm text-muted-foreground">{t("empty")}</p>
                {user?.is_super_admin && (
                  <Link
                    to="/admin/orgs"
                    className={buttonVariants({ variant: "outline", className: "w-full" })}
                  >
                    <Shield className="h-4 w-4 mr-2" />
                    {t("manageOrgs")}
                  </Link>
                )}
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
