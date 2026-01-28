// Thin typed wrapper around the shared language manifest at
// packyard/internal/i18n/languages.json. The Go backend embeds the same
// file at build time, so the two lists cannot drift.
import manifest from "@i18n-manifest";

export interface LanguageEntry {
  code: string;
  native: string;
}

const entries = manifest as readonly LanguageEntry[];

export const SUPPORTED_LANGUAGES: readonly string[] = entries.map((e) => e.code);

// The user-facing type is just `string` — the backend is authoritative
// about which codes are valid; runtime checks (isSupportedLanguage) are
// how callers guard themselves.
export type SupportedLanguage = string;

export const LANGUAGE_NAMES: Record<string, string> = Object.fromEntries(
  entries.map((e) => [e.code, e.native]),
);

export function isSupportedLanguage(code: string): boolean {
  return SUPPORTED_LANGUAGES.includes(code);
}
