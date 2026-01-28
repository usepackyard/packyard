package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port      int
	BaseURL   string
	Log       LogConfig
	DB        DBConfig
	Storage   StorageConfig
	S3        S3Config
	Session   SessionConfig
	Admin     AdminConfig
	Providers ProvidersConfig

	Mode string // "single" or "multi"

	BcryptCost int

	// DownloadRetentionDays is how long download_events rows are kept.
	// A daily goroutine prunes rows older than this. 0 disables pruning
	// (keeps events forever — useful for audit, grows unbounded).
	DownloadRetentionDays int

	// DistRateLimit and DistRateLimitWindow cap the per-IP request rate
	// on the Composer dist endpoint. Every successful 200 triggers an
	// UPDATE of download_count and an INSERT into download_events, so an
	// uncapped endpoint can be abused to inflate counters / bloat the
	// table. Default is generous enough for `composer install` bursts.
	DistRateLimit       int
	DistRateLimitWindow time.Duration

	// StatsCacheTTL is how long /packages/stats responses are cached in
	// memory per-org. The aggregation does 6 DB queries including a scan
	// of download_events; caching collapses N concurrent dashboard loads
	// into one DB hit per TTL window per org. 0 disables caching.
	StatsCacheTTL time.Duration

	// SyncWorkers is the number of goroutines pulling sync jobs from the
	// queue concurrently. Two is plenty for a single-instance deploy: one
	// can absorb a long-running sync while another handles quick webhook
	// triggers. Set to 0 to disable sync workers entirely (read-only
	// mode).
	SyncWorkers int

	// JobRetentionDays — sync_jobs rows older than this are pruned by a
	// daily goroutine. Finished jobs only (queued/running are never
	// touched). 0 disables pruning (grows unbounded; fine for low-volume
	// but typically not desired).
	JobRetentionDays int

	// TrustedProxies is the set of CIDRs whose X-Forwarded-For header
	// is honored. Empty means we ignore the header entirely (correct
	// for direct exposure). Configure to your reverse proxy's CIDR
	// before exposing the server through one, otherwise per-IP rate
	// limiting can be bypassed by header injection.
	TrustedProxies []*net.IPNet
}

type LogConfig struct {
	Level string
}

type DBConfig struct {
	Driver   string
	Host     string
	Port     int
	Name     string
	User     string
	Password string
	SSLMode  string
	Path     string
}

type StorageConfig struct {
	Type      string
	LocalPath string
}

type S3Config struct {
	Bucket         string
	Region         string
	Endpoint       string
	AccessKey      string
	SecretKey      string
	ForcePathStyle bool
}

type SessionConfig struct {
	// Secret signs session cookies (HMAC-SHA256). Must be at least
	// minSessionSecretLen bytes — startup fails otherwise.
	Secret string
	MaxAge int
}

// minSessionSecretLen is the minimum acceptable length for the session HMAC
// key. 32 bytes (256 bits) is plenty against any practical attack.
const minSessionSecretLen = 32

type AdminConfig struct {
	Email    string
	Password string
}

type ProvidersConfig struct {
	GitHubToken string
	GitLabToken string
}

// Validate returns an error for any configuration that would make the
// server obviously unsafe to start. Called once from main after Load.
func (c *Config) Validate() error {
	if len(c.Session.Secret) < minSessionSecretLen {
		return fmt.Errorf(
			"PACKYARD_SESSION_SECRET must be at least %d characters (got %d). Generate one with: openssl rand -hex 32",
			minSessionSecretLen, len(c.Session.Secret),
		)
	}
	return nil
}

// TokenFor returns the global auth token for a given provider.
func (c *ProvidersConfig) TokenFor(provider string) string {
	switch provider {
	case "github":
		return c.GitHubToken
	case "gitlab":
		return c.GitLabToken
	default:
		return ""
	}
}

func (c *DBConfig) DSN() string {
	switch c.Driver {
	case "mysql":
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4",
			c.User, c.Password, c.Host, c.Port, c.Name)
	case "postgres":
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode)
	case "sqlite":
		return c.Path + "?_pragma=foreign_keys(1)&_time_format=sqlite"
	default:
		return ""
	}
}

// minBcryptCost is the floor we'll allow operators to configure. Below 10,
// password hashes verify in milliseconds on commodity hardware.
const minBcryptCost = 10

func Load() *Config {
	bcryptCost := envInt("PACKYARD_BCRYPT_COST", 12)
	if bcryptCost < minBcryptCost {
		bcryptCost = minBcryptCost
	}
	return &Config{
		Port:    envInt("PACKYARD_PORT", 8080),
		BaseURL: env("PACKYARD_BASE_URL", "http://localhost:8080"),
		Log: LogConfig{
			Level: env("PACKYARD_LOG_LEVEL", "info"),
		},
		DB: DBConfig{
			Driver:   env("PACKYARD_DB_DRIVER", "sqlite"),
			Host:     env("PACKYARD_DB_HOST", "localhost"),
			Port:     envInt("PACKYARD_DB_PORT", 3306),
			Name:     env("PACKYARD_DB_NAME", "packyard"),
			User:     env("PACKYARD_DB_USER", "packyard"),
			Password: env("PACKYARD_DB_PASSWORD", ""),
			SSLMode:  env("PACKYARD_DB_SSLMODE", "disable"),
			Path:     env("PACKYARD_DB_PATH", "./packyard.db"),
		},
		Storage: StorageConfig{
			Type:      env("PACKYARD_STORAGE_TYPE", "local"),
			LocalPath: env("PACKYARD_STORAGE_LOCAL_PATH", "./data/packages"),
		},
		S3: S3Config{
			Bucket:         env("PACKYARD_S3_BUCKET", "packyard-packages"),
			Region:         env("PACKYARD_S3_REGION", "us-east-1"),
			Endpoint:       env("PACKYARD_S3_ENDPOINT", ""),
			AccessKey:      env("PACKYARD_S3_ACCESS_KEY", ""),
			SecretKey:      env("PACKYARD_S3_SECRET_KEY", ""),
			ForcePathStyle: envBool("PACKYARD_S3_FORCE_PATH_STYLE", false),
		},
		Session: SessionConfig{
			Secret: env("PACKYARD_SESSION_SECRET", ""),
			MaxAge: envInt("PACKYARD_SESSION_MAX_AGE", 86400),
		},
		Admin: AdminConfig{
			Email:    env("PACKYARD_ADMIN_EMAIL", "admin@example.com"),
			Password: env("PACKYARD_ADMIN_PASSWORD", "changeme"),
		},
		Mode: env("PACKYARD_MODE", "single"),
		Providers: ProvidersConfig{
			GitHubToken: env("PACKYARD_GITHUB_TOKEN", ""),
			GitLabToken: env("PACKYARD_GITLAB_TOKEN", ""),
		},
		BcryptCost:            bcryptCost,
		TrustedProxies:        parseCIDRs(env("PACKYARD_TRUSTED_PROXIES", "")),
		DownloadRetentionDays: envInt("PACKYARD_DOWNLOAD_RETENTION_DAYS", 90),
		DistRateLimit:         envInt("PACKYARD_DIST_RATELIMIT", 120),
		DistRateLimitWindow:   time.Duration(envInt("PACKYARD_DIST_RATELIMIT_WINDOW_SECONDS", 60)) * time.Second,
		StatsCacheTTL:         time.Duration(envInt("PACKYARD_STATS_CACHE_TTL_SECONDS", 60)) * time.Second,
		SyncWorkers:           envInt("PACKYARD_SYNC_WORKERS", 2),
		JobRetentionDays:      envInt("PACKYARD_JOB_RETENTION_DAYS", 30),
	}
}

// parseCIDRs splits a comma-separated list of CIDRs (or bare IPs) and
// returns the parsed networks. Invalid entries are skipped silently —
// the alternative would be to fail startup, but a typo'd CIDR
// shouldn't take the server down.
func parseCIDRs(s string) []*net.IPNet {
	if s == "" {
		return nil
	}
	var nets []*net.IPNet
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Allow bare IPs as well as CIDRs.
		if !strings.Contains(part, "/") {
			if ip := net.ParseIP(part); ip != nil {
				if ip.To4() != nil {
					part += "/32"
				} else {
					part += "/128"
				}
			}
		}
		_, n, err := net.ParseCIDR(part)
		if err != nil {
			continue
		}
		nets = append(nets, n)
	}
	return nets
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
