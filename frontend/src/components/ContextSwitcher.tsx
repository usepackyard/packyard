import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Building2, Check, ChevronsUpDown, Shield } from "lucide-react";

import { useAuth } from "@/hooks/useAuth";
import { cn } from "@/lib/utils";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";

// ContextSwitcher sits at the top of the sidebar in multi mode. Handles
// org switching and entering/exiting the super-admin context.
export default function ContextSwitcher() {
  const { t } = useTranslation("layout");
  const { user, org, orgs, setOrg } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [exitOpen, setExitOpen] = useState(false);

  const isAdminView = location.pathname.startsWith("/admin");

  const enterAdmin = () => navigate("/admin/orgs");

  const pickOrg = (slug: string) => {
    const next = orgs.find((o) => o.slug === slug);
    if (!next) return;
    setOrg(next);
    navigate("/");
  };

  const handleExitPick = (slug: string) => {
    pickOrg(slug);
    setExitOpen(false);
  };

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          className={cn(
            "flex w-full items-center gap-2 rounded-md border bg-background px-3 py-2 text-left text-sm font-medium transition-colors hover:bg-muted",
            isAdminView && "border-amber-500/40 bg-amber-50/50 dark:bg-amber-500/5"
          )}
        >
          <div className="flex h-6 w-6 items-center justify-center rounded-md bg-muted shrink-0">
            {isAdminView ? (
              <Shield className="h-3.5 w-3.5 text-amber-600" />
            ) : (
              <Building2 className="h-3.5 w-3.5 text-muted-foreground" />
            )}
          </div>
          <div className="flex-1 min-w-0">
            <div className="truncate">
              {isAdminView ? t("contextSwitcher.superAdmin") : org?.name ?? t("contextSwitcher.select")}
            </div>
            <div className="truncate text-xs text-muted-foreground">
              {isAdminView ? t("contextSwitcher.allOrgs") : org?.slug ?? ""}
            </div>
          </div>
          <ChevronsUpDown className="h-4 w-4 text-muted-foreground shrink-0" />
        </DropdownMenuTrigger>

        <DropdownMenuContent align="start" className="w-[var(--anchor-width)]">
          {isAdminView ? (
            <DropdownMenuGroup>
              <DropdownMenuLabel>{t("contextSwitcher.superAdminMode")}</DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => setExitOpen(true)}>
                <span className="mr-2">←</span>
                {t("contextSwitcher.backToOrg")}
              </DropdownMenuItem>
            </DropdownMenuGroup>
          ) : (
            <>
              <DropdownMenuGroup>
                <DropdownMenuLabel>{t("contextSwitcher.organizations")}</DropdownMenuLabel>
                {orgs.length === 0 && (
                  <div className="px-2 py-1.5 text-xs text-muted-foreground">
                    {t("contextSwitcher.noOrgs")}
                  </div>
                )}
                {orgs.map((o) => {
                  const active = org?.slug === o.slug;
                  return (
                    <DropdownMenuItem key={o.slug} onClick={() => pickOrg(o.slug)}>
                      <div className="flex w-full items-center gap-2">
                        <div className="w-4 shrink-0">
                          {active && <Check className="h-3.5 w-3.5" />}
                        </div>
                        <span className="flex-1 truncate">{o.name}</span>
                        <span className="font-mono text-xs text-muted-foreground">
                          {o.slug}
                        </span>
                      </div>
                    </DropdownMenuItem>
                  );
                })}
              </DropdownMenuGroup>
              {user?.is_super_admin && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onClick={enterAdmin}>
                    <Shield className="h-3.5 w-3.5 text-amber-600 mr-2" />
                    {t("enterSuperAdmin")}
                  </DropdownMenuItem>
                </>
              )}
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      <ExitAdminDialog
        open={exitOpen}
        onOpenChange={setExitOpen}
        orgs={orgs}
        onPick={handleExitPick}
      />
    </>
  );
}

interface ExitAdminDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  orgs: { slug: string; name: string }[];
  onPick: (slug: string) => void;
}

function ExitAdminDialog({ open, onOpenChange, orgs, onPick }: ExitAdminDialogProps) {
  const { t } = useTranslation("layout");
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("contextSwitcher.switchTitle")}</DialogTitle>
        </DialogHeader>
        {orgs.length > 0 ? (
          <div className="space-y-1">
            {orgs.map((o) => (
              <button
                key={o.slug}
                type="button"
                onClick={() => onPick(o.slug)}
                className="flex w-full items-center gap-3 rounded-md border bg-background p-3 text-left hover:bg-muted transition-colors"
              >
                <div className="flex h-8 w-8 items-center justify-center rounded-md bg-muted shrink-0">
                  <Building2 className="h-4 w-4 text-muted-foreground" />
                </div>
                <div className="min-w-0 flex-1">
                  <div className="font-medium text-sm truncate">{o.name}</div>
                  <div className="font-mono text-xs text-muted-foreground truncate">{o.slug}</div>
                </div>
              </button>
            ))}
          </div>
        ) : (
          <div className="space-y-4">
            <p className="text-sm text-muted-foreground">
              {t("contextSwitcher.noOrgsHelp")}
            </p>
            <div className="flex gap-2 justify-end">
              <Button variant="ghost" onClick={() => onOpenChange(false)}>
                {t("contextSwitcher.stayInSuperAdmin")}
              </Button>
              <Button onClick={() => onOpenChange(false)}>
                {t("contextSwitcher.manageOrgs")}
              </Button>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
