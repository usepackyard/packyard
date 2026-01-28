import i18n from "i18next";
import { initReactI18next } from "react-i18next";

export {
  SUPPORTED_LANGUAGES,
  LANGUAGE_NAMES,
  isSupportedLanguage,
  type LanguageEntry,
  type SupportedLanguage,
} from "@/i18n/languages";
import {
  SUPPORTED_LANGUAGES,
  isSupportedLanguage,
  type SupportedLanguage,
} from "@/i18n/languages";

import enCommon from "@/locales/en/common.json";
import enAuth from "@/locales/en/auth.json";
import enProfile from "@/locales/en/profile.json";
import enErrors from "@/locales/en/errors.json";
import enDashboard from "@/locales/en/dashboard.json";
import enOrgSelector from "@/locales/en/orgSelector.json";
import enTokens from "@/locales/en/tokens.json";
import enUsers from "@/locales/en/users.json";
import enMembers from "@/locales/en/members.json";
import enPackages from "@/locales/en/packages.json";
import enAdmin from "@/locales/en/admin.json";
import enLayout from "@/locales/en/layout.json";
import mkCommon from "@/locales/mk/common.json";
import mkAuth from "@/locales/mk/auth.json";
import mkProfile from "@/locales/mk/profile.json";
import mkErrors from "@/locales/mk/errors.json";
import mkDashboard from "@/locales/mk/dashboard.json";
import mkOrgSelector from "@/locales/mk/orgSelector.json";
import mkTokens from "@/locales/mk/tokens.json";
import mkUsers from "@/locales/mk/users.json";
import mkMembers from "@/locales/mk/members.json";
import mkPackages from "@/locales/mk/packages.json";
import mkAdmin from "@/locales/mk/admin.json";
import mkLayout from "@/locales/mk/layout.json";
import deCommon from "@/locales/de/common.json";
import deAuth from "@/locales/de/auth.json";
import deProfile from "@/locales/de/profile.json";
import deErrors from "@/locales/de/errors.json";
import deDashboard from "@/locales/de/dashboard.json";
import deOrgSelector from "@/locales/de/orgSelector.json";
import deTokens from "@/locales/de/tokens.json";
import deUsers from "@/locales/de/users.json";
import deMembers from "@/locales/de/members.json";
import dePackages from "@/locales/de/packages.json";
import deAdmin from "@/locales/de/admin.json";
import deLayout from "@/locales/de/layout.json";
import frCommon from "@/locales/fr/common.json";
import frAuth from "@/locales/fr/auth.json";
import frProfile from "@/locales/fr/profile.json";
import frErrors from "@/locales/fr/errors.json";
import frDashboard from "@/locales/fr/dashboard.json";
import frOrgSelector from "@/locales/fr/orgSelector.json";
import frTokens from "@/locales/fr/tokens.json";
import frUsers from "@/locales/fr/users.json";
import frMembers from "@/locales/fr/members.json";
import frPackages from "@/locales/fr/packages.json";
import frAdmin from "@/locales/fr/admin.json";
import frLayout from "@/locales/fr/layout.json";
import esCommon from "@/locales/es/common.json";
import esAuth from "@/locales/es/auth.json";
import esProfile from "@/locales/es/profile.json";
import esErrors from "@/locales/es/errors.json";
import esDashboard from "@/locales/es/dashboard.json";
import esOrgSelector from "@/locales/es/orgSelector.json";
import esTokens from "@/locales/es/tokens.json";
import esUsers from "@/locales/es/users.json";
import esMembers from "@/locales/es/members.json";
import esPackages from "@/locales/es/packages.json";
import esAdmin from "@/locales/es/admin.json";
import esLayout from "@/locales/es/layout.json";
import ptBRCommon from "@/locales/pt-BR/common.json";
import ptBRAuth from "@/locales/pt-BR/auth.json";
import ptBRProfile from "@/locales/pt-BR/profile.json";
import ptBRErrors from "@/locales/pt-BR/errors.json";
import ptBRDashboard from "@/locales/pt-BR/dashboard.json";
import ptBROrgSelector from "@/locales/pt-BR/orgSelector.json";
import ptBRTokens from "@/locales/pt-BR/tokens.json";
import ptBRUsers from "@/locales/pt-BR/users.json";
import ptBRMembers from "@/locales/pt-BR/members.json";
import ptBRPackages from "@/locales/pt-BR/packages.json";
import ptBRAdmin from "@/locales/pt-BR/admin.json";
import ptBRLayout from "@/locales/pt-BR/layout.json";
import itCommon from "@/locales/it/common.json";
import itAuth from "@/locales/it/auth.json";
import itProfile from "@/locales/it/profile.json";
import itErrors from "@/locales/it/errors.json";
import itDashboard from "@/locales/it/dashboard.json";
import itOrgSelector from "@/locales/it/orgSelector.json";
import itTokens from "@/locales/it/tokens.json";
import itUsers from "@/locales/it/users.json";
import itMembers from "@/locales/it/members.json";
import itPackages from "@/locales/it/packages.json";
import itAdmin from "@/locales/it/admin.json";
import itLayout from "@/locales/it/layout.json";

// SUPPORTED_LANGUAGES / LANGUAGE_NAMES / SupportedLanguage come from the
// shared manifest at packyard/internal/i18n/languages.json, imported via
// the @i18n-manifest alias. The Go backend reads the same file, so the
// two lists cannot drift.

export const LANG_STORAGE_KEY = "packyard:lang";

function readStoredLanguage(): SupportedLanguage {
  try {
    const v = localStorage.getItem(LANG_STORAGE_KEY);
    if (v && isSupportedLanguage(v)) {
      return v;
    }
  } catch {
    // localStorage can be blocked — non-fatal, fall through to default.
  }
  return "en";
}

i18n
  .use(initReactI18next)
  .init({
    // Default is English for everyone. The real user preference arrives
    // from /api/auth/me after login and is applied via applyUserLanguage.
    // Until then, we render in whatever was last cached in localStorage
    // (so returning users don't see a flash of English) or fall back to
    // "en" if nothing is cached.
    lng: readStoredLanguage(),
    fallbackLng: "en",
    supportedLngs: [...SUPPORTED_LANGUAGES],
    ns: [
      "common", "auth", "profile", "errors",
      "dashboard", "orgSelector", "tokens", "users", "members",
      "packages", "admin", "layout",
    ],
    defaultNS: "common",
    resources: {
      en: {
        common: enCommon, auth: enAuth, profile: enProfile, errors: enErrors,
        dashboard: enDashboard, orgSelector: enOrgSelector, tokens: enTokens,
        users: enUsers, members: enMembers, packages: enPackages,
        admin: enAdmin, layout: enLayout,
      },
      mk: {
        common: mkCommon, auth: mkAuth, profile: mkProfile, errors: mkErrors,
        dashboard: mkDashboard, orgSelector: mkOrgSelector, tokens: mkTokens,
        users: mkUsers, members: mkMembers, packages: mkPackages,
        admin: mkAdmin, layout: mkLayout,
      },
      de: {
        common: deCommon, auth: deAuth, profile: deProfile, errors: deErrors,
        dashboard: deDashboard, orgSelector: deOrgSelector, tokens: deTokens,
        users: deUsers, members: deMembers, packages: dePackages,
        admin: deAdmin, layout: deLayout,
      },
      fr: {
        common: frCommon, auth: frAuth, profile: frProfile, errors: frErrors,
        dashboard: frDashboard, orgSelector: frOrgSelector, tokens: frTokens,
        users: frUsers, members: frMembers, packages: frPackages,
        admin: frAdmin, layout: frLayout,
      },
      es: {
        common: esCommon, auth: esAuth, profile: esProfile, errors: esErrors,
        dashboard: esDashboard, orgSelector: esOrgSelector, tokens: esTokens,
        users: esUsers, members: esMembers, packages: esPackages,
        admin: esAdmin, layout: esLayout,
      },
      "pt-BR": {
        common: ptBRCommon, auth: ptBRAuth, profile: ptBRProfile, errors: ptBRErrors,
        dashboard: ptBRDashboard, orgSelector: ptBROrgSelector, tokens: ptBRTokens,
        users: ptBRUsers, members: ptBRMembers, packages: ptBRPackages,
        admin: ptBRAdmin, layout: ptBRLayout,
      },
      it: {
        common: itCommon, auth: itAuth, profile: itProfile, errors: itErrors,
        dashboard: itDashboard, orgSelector: itOrgSelector, tokens: itTokens,
        users: itUsers, members: itMembers, packages: itPackages,
        admin: itAdmin, layout: itLayout,
      },
    },
    interpolation: { escapeValue: false },
    returnNull: false,
  });

// applyUserLanguage is called after /api/auth/me resolves (and after the
// user saves a new preference on the profile page). Writes to
// localStorage so the next cold-start shows the right language before
// the auth round-trip completes.
export function applyUserLanguage(lang: string | undefined | null) {
  if (!lang || !isSupportedLanguage(lang)) return;
  if (i18n.language !== lang) i18n.changeLanguage(lang);
  try {
    localStorage.setItem(LANG_STORAGE_KEY, lang);
  } catch {
    // Ignore storage failures.
  }
}

// setLocalLanguage is for the pre-login language picker — writes to
// localStorage only, does not hit the server. Used on the Login page so
// non-English speakers can read the form before they have a session.
export function setLocalLanguage(lang: string) {
  applyUserLanguage(lang);
}

export default i18n;
