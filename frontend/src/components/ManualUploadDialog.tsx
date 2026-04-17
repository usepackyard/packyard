import { useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";

// ManualUploadDialog collects the bits a manual-metadata upload needs
// that a plugin-style zip can't carry on its own: the version string
// and (optionally) overrides for the package's require block.
//
// Used when the package's source is `provider=upload + metadata=manual`.
// For `from_zip` uploads the drop zone posts directly without this
// modal — composer.json inside the zip has everything already.
export interface ManualUploadDialogProps {
  open: boolean;
  filename: string | null;
  baselineRequire: string; // the source's current manual_require (JSON, may be empty)
  submitting: boolean;
  onSubmit: (args: { version: string; requireOverride: string }) => void;
  onCancel: () => void;
}

export function ManualUploadDialog({
  open,
  filename,
  baselineRequire,
  submitting,
  onSubmit,
  onCancel,
}: ManualUploadDialogProps) {
  const { t } = useTranslation("packages");
  const [version, setVersion] = useState("");
  const [requireOverride, setRequireOverride] = useState("");
  const [localError, setLocalError] = useState("");

  // Reset fields every time the modal (re)opens with a new filename so
  // a failed upload doesn't leak stale values into the next attempt.
  useEffect(() => {
    if (open) {
      setVersion(filename ? guessVersionFromFilename(filename) : "");
      setRequireOverride("");
      setLocalError("");
    }
  }, [open, filename]);

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!version.trim()) {
      setLocalError(t("detail.upload.manualDialog.versionRequired"));
      return;
    }
    // Client-side JSON sanity check — the backend validates too, but
    // catching it here surfaces the error without a round-trip.
    const trimmed = requireOverride.trim();
    if (trimmed) {
      try {
        const parsed = JSON.parse(trimmed);
        if (typeof parsed !== "object" || Array.isArray(parsed) || parsed === null) {
          setLocalError(t("detail.upload.manualDialog.requireOverrideInvalid"));
          return;
        }
      } catch {
        setLocalError(t("detail.upload.manualDialog.requireOverrideInvalid"));
        return;
      }
    }
    setLocalError("");
    onSubmit({ version: version.trim(), requireOverride: trimmed });
  };

  return (
    <Dialog open={open} onOpenChange={(next) => { if (!next) onCancel(); }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("detail.upload.manualDialog.title")}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          {filename && (
            <p className="text-xs text-muted-foreground">
              <span className="font-medium text-foreground">{t("detail.upload.manualDialog.file")}:</span>{" "}
              <code className="bg-muted px-1.5 py-0.5 rounded font-mono">{filename}</code>
            </p>
          )}
          {localError && (
            <div className="text-sm text-destructive bg-destructive/10 px-3 py-2 rounded-md">{localError}</div>
          )}
          <div className="space-y-2">
            <Label htmlFor="manual-upload-version">{t("detail.upload.manualDialog.versionLabel")}</Label>
            <Input
              id="manual-upload-version"
              placeholder="1.0.0"
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              autoFocus
              required
            />
            <p className="text-xs text-muted-foreground">{t("detail.upload.manualDialog.versionHelp")}</p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="manual-upload-require">{t("detail.upload.manualDialog.requireOverrideLabel")}</Label>
            <textarea
              id="manual-upload-require"
              className="flex w-full min-h-[80px] rounded-md border bg-transparent px-3 py-2 text-xs font-mono resize-y"
              placeholder={`{"php": ">=8.0"}`}
              value={requireOverride}
              onChange={(e) => setRequireOverride(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              {t("detail.upload.manualDialog.requireOverrideHelp", {
                defaultValue:
                  baselineRequire
                    ? "Merged onto the source's baseline require. Keys here win."
                    : "Optional additional require entries for this version.",
              })}
            </p>
            {baselineRequire && (
              <p className="text-xs text-muted-foreground">
                <span className="font-medium text-foreground">{t("detail.upload.manualDialog.baselineLabel")}:</span>{" "}
                <code className="bg-muted px-1.5 py-0.5 rounded font-mono break-all">{baselineRequire}</code>
              </p>
            )}
          </div>
          <div className="flex gap-2 justify-end">
            <Button type="button" variant="outline" onClick={onCancel} disabled={submitting}>
              {t("detail.upload.manualDialog.cancel")}
            </Button>
            <Button type="submit" disabled={submitting}>
              {submitting
                ? t("detail.upload.manualDialog.submitting")
                : t("detail.upload.manualDialog.submit")}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}

// guessVersionFromFilename pulls a `vX.Y.Z`-shaped token out of an
// asset filename so we can pre-populate the version input. It's a
// best-effort convenience, not a contract — users can overwrite.
function guessVersionFromFilename(name: string): string {
  const withoutExt = name.replace(/\.(zip|tgz|tar\.gz)$/i, "");
  // Match a trailing version-like tail: digits, dots, dashes, and
  // alphanumeric suffix (-rc.1, -beta.3, -alpha, etc.).
  const m = withoutExt.match(/-?(v?\d[\w.+-]*)$/);
  if (!m) return "";
  return m[1].replace(/^v/, "");
}
