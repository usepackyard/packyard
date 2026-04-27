export interface User {
  id: string;
  email: string;
  name: string;
  language: string;
  is_active: boolean;
  is_super_admin: boolean;
  created_at: string;
  updated_at: string;
}

export type OrgStatus = "active" | "suspended" | "archived";

export interface Organization {
  id: number;
  slug: string;
  name: string;
  status: OrgStatus;
  created_at: string;
  updated_at: string;
}

export interface AdminToken {
  id: string;
  name: string;
  token_prefix: string;
  last_used_at?: string;
  expires_at?: string;
  is_active: boolean;
  created_at: string;
}

export interface OrgMember {
  id: string;
  role: string;
  permissions: string[];
  created_at: string;
  user?: User;
}

export interface Package {
  id: string;
  org_id: number;
  name: string;
  type: string;
  description: string;
  homepage?: string;
  versions?: Version[];
  created_at: string;
  updated_at: string;
}

export interface Version {
  id: string;
  version: string;
  version_normalized: string;
  dist_type: string;
  dist_sha1: string;
  file_size: number;
  download_count: number;
  last_downloaded_at?: string | null;
  created_at: string;
}

// Download-stats payload returned by GET /packages/stats. One request
// feeds the whole dashboard so stat cards, leaderboard, and activity
// feed stay consistent with each other. `package_id` is the package's
// prefixed-ULID public id (pkg_…), so links can use it directly.
export interface PackageDownloadCount {
  package_id: string;
  package_name: string;
  count: number;
}

export interface DownloadEventView {
  at: string;
  package_id: string;
  package_name: string;
  version: string;
}

export interface DailyCount {
  day: string; // YYYY-MM-DD (UTC)
  count: number;
}

export interface PackageStats {
  total_downloads: number;
  downloads_last_7d: number;
  downloads_last_30d: number;
  top_packages: PackageDownloadCount[];
  recent_downloads: DownloadEventView[];
  daily_series_30d: DailyCount[];
}

// Where a package gets its versions from.
//   github/gitlab — sync from release metadata (webhook/poll, repo coordinates required).
//   upload        — user drops zips directly; no repo, no sync.
export type SourceProvider = "github" | "gitlab" | "upload";

export interface SourceConfig {
  owner: string;
  repo: string;
  strategy: SourceStrategy;
  asset_pattern: string;
}

export interface ConnectionConfig {
  host?: string;
}

export interface PackageSource {
  id: string;
  provider: SourceProvider;
  connection_id?: string;
  config?: SourceConfig;
  repo_key?: string;
  repo_url?: string;
  metadata_source: MetadataSource;
  version_source: VersionSource;
  manual_require?: string;
  last_synced_at?: string;
  created_at: string;
  updated_at: string;
}

export interface SkippedEntry {
  tag: string;
  // "already-exists" | "no-matching-asset" | "composer-version-missing" | ...
  reason: string;
}

export interface SyncResult {
  imported: string[];
  // Tags whose release date was backfilled in-place (existing versions
  // already present in the DB; dist bytes untouched). Distinct from
  // Imported (new versions pulled in) and Skipped (no change at all).
  refreshed: string[];
  skipped: SkippedEntry[];
  errors: string[];
}

// SyncJob mirrors internal/model/sync_job.go — the persistent record of
// one sync pipeline run. The frontend polls GetSyncJob for live state and
// renders the final SyncResult (from ResultJSON) once terminal.
export interface SyncJob {
  id: string;
  trigger: "manual" | "webhook";
  status: "queued" | "running" | "succeeded" | "failed" | "stale";
  progress_done: number;
  progress_total: number;
  imported: number;
  refreshed: number;
  skipped: number;
  errored: number;
  result_json?: string;
  error_msg?: string;
  created_at: string;
  started_at?: string;
  finished_at?: string;
}

// SourceStrategy: only meaningful for provider=github.
export type SourceStrategy = "release_asset" | "source_archive" | "";

// Where composer.json content comes from on each sync.
//   from_zip — read composer.json from the dist zip (the default Composer flow).
//   manual   — synthesize composer.json from the Package row + ManualRequire.
//              Used for release zips that ship without composer.json (WordPress
//              plugin distributions).
export type MetadataSource = "from_zip" | "manual";

// Where the version string comes from. Valid values differ per provider:
//   github: auto | git_tag | composer_json
//   upload: composer_json | manual (user types the version per upload)
export type VersionSource = "auto" | "git_tag" | "composer_json" | "manual";

// One release surfaced by the /sources/preview endpoint — just enough
// for the UI to help the user build an asset_pattern.
export interface ReleasePreview {
  tag: string;
  assets: ReleasePreviewAsset[];
}

export interface ReleasePreviewAsset {
  name: string;
  size: number;
}

export type ProviderAuthType = "none" | "token";

export interface ProviderConnection {
  id: string;
  name: string;
  provider: "github" | "gitlab";
  auth_type: ProviderAuthType;
  token_prefix?: string;
  config?: ConnectionConfig;
  source_count: number;
  created_at: string;
  updated_at: string;
}

export interface APIToken {
  id: string;
  name: string;
  token_prefix: string;
  last_used_at?: string;
  expires_at?: string;
  is_active: boolean;
  created_at: string;
}

export interface AppConfig {
  base_url: string;
  // Optional absolute URL of an external homepage (operator's main
  // site, docs, intranet). When present, the dashboard renders a
  // subtle "back" link pointing at it. Emitted by the server only
  // when PACKYARD_PUBLIC_URL is set, so the property is absent on
  // deployments that don't opt in.
  public_url?: string;
}
