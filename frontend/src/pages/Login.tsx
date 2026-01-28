import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Check, Globe } from "lucide-react";
import { useAuth } from "@/hooks/useAuth";
import { globalApi } from "@/api/client";
import {
  applyUserLanguage,
  setLocalLanguage,
  SUPPORTED_LANGUAGES,
  LANGUAGE_NAMES,
  isSupportedLanguage,
} from "@/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { LogoIcon } from "@/components/Logo";

export default function Login() {
  const { t, i18n } = useTranslation(["auth"]);
  const currentLang = isSupportedLanguage(i18n.language)
    ? i18n.language
    : "en";
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const { setUser } = useAuth();
  const navigate = useNavigate();

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const { user } = await globalApi.login(email, password);
      setUser(user);
      // Server-side preference wins immediately on successful login —
      // overrides whatever locale the pre-login picker may have set.
      applyUserLanguage(user.language);
      navigate("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : t("auth:login.failed"));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-muted/30">
      <Card className="w-full max-w-sm">
        <CardHeader className="text-center">
          <div className="flex justify-center mb-2"><LogoIcon size={36} /></div>
          <CardTitle className="text-2xl font-bold">{t("auth:login.title")}</CardTitle>
          <CardDescription>{t("auth:login.subtitle")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            {error && (
              <div className="text-sm text-destructive bg-destructive/10 px-3 py-2 rounded-md">{error}</div>
            )}
            <div className="space-y-2">
              <Label htmlFor="email">{t("auth:login.email")}</Label>
              <Input id="email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">{t("auth:login.password")}</Label>
              <Input id="password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} required />
            </div>
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? t("auth:login.submitting") : t("auth:login.submit")}
            </Button>
          </form>
          {/* Pre-login language picker. Writes to localStorage only —
              the server isn't reachable without a session. After the
              user logs in, their stored user.language takes over. */}
          <div className="mt-6 flex justify-center">
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-xs text-muted-foreground hover:text-foreground"
                  >
                    <Globe className="h-3.5 w-3.5" />
                    {LANGUAGE_NAMES[currentLang]}
                  </Button>
                }
              />
              <DropdownMenuContent align="center" sideOffset={6}>
                {SUPPORTED_LANGUAGES.map((code) => (
                  <DropdownMenuItem
                    key={code}
                    onClick={() => setLocalLanguage(code)}
                  >
                    <span className="flex-1">{LANGUAGE_NAMES[code]}</span>
                    {currentLang === code ? (
                      <Check className="ml-2 h-4 w-4" />
                    ) : null}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
