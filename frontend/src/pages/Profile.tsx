import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { ApiError, globalApi } from "@/api/client";
import { useAuth } from "@/hooks/useAuth";
import { applyUserLanguage, SUPPORTED_LANGUAGES } from "@/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export default function Profile() {
  const { t } = useTranslation("profile");
  const { user, setUser } = useAuth();

  const [name, setName] = useState(user?.name ?? "");
  const [language, setLanguage] = useState(user?.language ?? "en");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);

  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [savingPassword, setSavingPassword] = useState(false);
  const [passwordError, setPasswordError] = useState("");
  const [passwordSaved, setPasswordSaved] = useState(false);

  if (!user) return null;

  const dirty = name !== user.name || language !== user.language;

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    setSaved(false);
    setSaving(true);
    try {
      const { user: updated } = await globalApi.updateMe({ name, language });
      setUser(updated);
      applyUserLanguage(updated.language);
      setSaved(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("failed"));
    } finally {
      setSaving(false);
    }
  };

  const canSubmitPassword =
    currentPassword.length > 0 &&
    newPassword.length >= 8 &&
    newPassword === confirmPassword;

  const handleChangePassword = async (e: FormEvent) => {
    e.preventDefault();
    setPasswordError("");
    setPasswordSaved(false);

    if (newPassword !== confirmPassword) {
      setPasswordError(t("password.mismatch"));
      return;
    }
    if (newPassword.length < 8) {
      setPasswordError(t("password.tooShort"));
      return;
    }

    setSavingPassword(true);
    try {
      await globalApi.changePassword(currentPassword, newPassword);
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      setPasswordSaved(true);
    } catch (err) {
      if (err instanceof ApiError && err.code === "wrong_password") {
        setPasswordError(err.message);
      } else {
        setPasswordError(err instanceof Error ? err.message : t("failed"));
      }
    } finally {
      setSavingPassword(false);
    }
  };

  return (
    <div className="max-w-2xl">
      <div className="mb-6">
        <h2 className="text-2xl font-bold tracking-tight">{t("title")}</h2>
        <p className="text-muted-foreground mt-1">{t("subtitle")}</p>
      </div>

      <form onSubmit={handleSubmit} className="space-y-6">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("section.account")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="name">{t("field.name")}</Label>
              <Input
                id="name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="email">{t("field.email")}</Label>
              <Input id="email" value={user.email} readOnly disabled />
              <p className="text-xs text-muted-foreground">{t("field.emailHelp")}</p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("section.preferences")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="language">{t("field.language")}</Label>
              <select
                id="language"
                className="flex h-9 w-full rounded-md border bg-transparent px-3 py-1 text-sm"
                value={language}
                onChange={(e) => setLanguage(e.target.value)}
              >
                {SUPPORTED_LANGUAGES.map((code) => (
                  <option key={code} value={code}>
                    {t(`language.${code}` as "language.en" | "language.mk")}
                  </option>
                ))}
              </select>
              <p className="text-xs text-muted-foreground">{t("field.languageHelp")}</p>
            </div>
          </CardContent>
        </Card>

        {error && (
          <div className="text-sm text-destructive bg-destructive/10 px-3 py-2 rounded-md">
            {error}
          </div>
        )}
        {saved && !error && (
          <div className="text-sm text-muted-foreground">{t("saved")}</div>
        )}

        <div className="flex justify-end">
          <Button type="submit" disabled={saving || !dirty}>
            {saving ? t("saving") : t("save")}
          </Button>
        </div>
      </form>

      <form onSubmit={handleChangePassword} className="mt-6 space-y-6">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("section.password")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="currentPassword">{t("field.currentPassword")}</Label>
              <Input
                id="currentPassword"
                type="password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="newPassword">{t("field.newPassword")}</Label>
              <Input
                id="newPassword"
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                required
                minLength={8}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="confirmPassword">{t("field.confirmPassword")}</Label>
              <Input
                id="confirmPassword"
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                required
                minLength={8}
              />
            </div>
            {passwordError && (
              <div className="text-sm text-destructive bg-destructive/10 px-3 py-2 rounded-md">
                {passwordError}
              </div>
            )}
            {passwordSaved && !passwordError && (
              <div className="text-sm text-muted-foreground">{t("password.saved")}</div>
            )}
            <div className="flex justify-end">
              <Button type="submit" disabled={savingPassword || !canSubmitPassword}>
                {savingPassword ? t("password.saving") : t("password.change")}
              </Button>
            </div>
          </CardContent>
        </Card>
      </form>
    </div>
  );
}
