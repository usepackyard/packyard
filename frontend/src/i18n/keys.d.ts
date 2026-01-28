import "react-i18next";

import type common from "@/locales/en/common.json";
import type auth from "@/locales/en/auth.json";
import type profile from "@/locales/en/profile.json";
import type errors from "@/locales/en/errors.json";
import type dashboard from "@/locales/en/dashboard.json";
import type orgSelector from "@/locales/en/orgSelector.json";
import type tokens from "@/locales/en/tokens.json";
import type users from "@/locales/en/users.json";
import type members from "@/locales/en/members.json";
import type packages from "@/locales/en/packages.json";
import type admin from "@/locales/en/admin.json";
import type layout from "@/locales/en/layout.json";

// Teaches react-i18next's t() the shape of our English catalogs so
// missing-key typos fail the TypeScript build instead of shipping as
// broken UI. English is the authoritative source — other locales are
// allowed to lag (keys missing in mk/ fall back to en/ at runtime).
declare module "react-i18next" {
  interface CustomTypeOptions {
    defaultNS: "common";
    resources: {
      common: typeof common;
      auth: typeof auth;
      profile: typeof profile;
      errors: typeof errors;
      dashboard: typeof dashboard;
      orgSelector: typeof orgSelector;
      tokens: typeof tokens;
      users: typeof users;
      members: typeof members;
      packages: typeof packages;
      admin: typeof admin;
      layout: typeof layout;
    };
  }
}
