package provider

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	ProviderUpload = "upload"
	ProviderGitHub = "github"
	ProviderGitLab = "gitlab"

	StrategyReleaseAsset  = "release_asset"
	StrategySourceArchive = "source_archive"
)

type SourceConfig struct {
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	Strategy     string `json:"strategy"`
	AssetPattern string `json:"asset_pattern"`
}

type ConnectionConfig struct {
	Host string `json:"host,omitempty"`
}

func ParseSourceConfig(providerName, raw string) (SourceConfig, error) {
	if providerName == ProviderUpload {
		return SourceConfig{}, nil
	}
	var cfg SourceConfig
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return SourceConfig{}, fmt.Errorf("provider_config: %w", err)
		}
	}
	cfg.Owner = strings.TrimSpace(cfg.Owner)
	cfg.Repo = strings.TrimSpace(cfg.Repo)
	cfg.Strategy = strings.TrimSpace(cfg.Strategy)
	cfg.AssetPattern = strings.TrimSpace(cfg.AssetPattern)
	if cfg.Strategy == "" {
		cfg.Strategy = StrategyReleaseAsset
	}
	if cfg.AssetPattern == "" {
		cfg.AssetPattern = "*.zip"
	}
	if err := ValidateSourceConfig(providerName, cfg); err != nil {
		return SourceConfig{}, err
	}
	return cfg, nil
}

func MarshalSourceConfig(providerName string, cfg SourceConfig) (string, string, error) {
	if providerName == ProviderUpload {
		return "", "", nil
	}
	cfg.Owner = strings.TrimSpace(cfg.Owner)
	cfg.Repo = strings.TrimSpace(cfg.Repo)
	cfg.Strategy = strings.TrimSpace(cfg.Strategy)
	cfg.AssetPattern = strings.TrimSpace(cfg.AssetPattern)
	if cfg.Strategy == "" {
		cfg.Strategy = StrategyReleaseAsset
	}
	if cfg.AssetPattern == "" {
		cfg.AssetPattern = "*.zip"
	}
	if err := ValidateSourceConfig(providerName, cfg); err != nil {
		return "", "", err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "", "", err
	}
	return string(raw), RepoKey(providerName, "", cfg), nil
}

func ValidateSourceConfig(providerName string, cfg SourceConfig) error {
	switch providerName {
	case ProviderGitHub, ProviderGitLab:
		if cfg.Owner == "" || cfg.Repo == "" {
			return fmt.Errorf("repo owner and name are required")
		}
		switch cfg.Strategy {
		case StrategyReleaseAsset, StrategySourceArchive:
		default:
			return fmt.Errorf("strategy must be release_asset or source_archive")
		}
		if cfg.Strategy == StrategyReleaseAsset {
			if _, err := filepath.Match(cfg.AssetPattern, "test.zip"); err != nil {
				return fmt.Errorf("invalid asset_pattern glob")
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported provider: %s", providerName)
	}
}

func ParseConnectionConfig(providerName, raw string) (ConnectionConfig, error) {
	var cfg ConnectionConfig
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return ConnectionConfig{}, fmt.Errorf("connection config: %w", err)
		}
	}
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.Host = strings.TrimPrefix(strings.TrimPrefix(cfg.Host, "https://"), "http://")
	cfg.Host = strings.TrimRight(cfg.Host, "/")
	if providerName == ProviderGitLab && cfg.Host == "" {
		cfg.Host = "gitlab.com"
	}
	return cfg, nil
}

func MarshalConnectionConfig(providerName string, cfg ConnectionConfig) (string, error) {
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.Host = strings.TrimPrefix(strings.TrimPrefix(cfg.Host, "https://"), "http://")
	cfg.Host = strings.TrimRight(cfg.Host, "/")
	if providerName == ProviderGitLab && cfg.Host == "" {
		cfg.Host = "gitlab.com"
	}
	if providerName == ProviderGitHub {
		cfg.Host = ""
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func RepoKey(providerName, connectionConfig string, source SourceConfig) string {
	owner := strings.ToLower(strings.TrimSpace(source.Owner))
	repo := strings.ToLower(strings.TrimSpace(source.Repo))
	if owner == "" && repo == "" {
		return ""
	}
	if providerName == ProviderGitLab {
		conn, _ := ParseConnectionConfig(providerName, connectionConfig)
		host := conn.Host
		if host == "" {
			host = "gitlab.com"
		}
		return strings.ToLower(host) + "/" + owner + "/" + repo
	}
	return owner + "/" + repo
}

func ExternalRepoURL(providerName, connectionConfig string, source SourceConfig) string {
	switch providerName {
	case ProviderGitHub:
		return "https://github.com/" + source.Owner + "/" + source.Repo
	case ProviderGitLab:
		conn, _ := ParseConnectionConfig(providerName, connectionConfig)
		host := conn.Host
		if host == "" {
			host = "gitlab.com"
		}
		return "https://" + host + "/" + source.Owner + "/" + source.Repo
	default:
		return ""
	}
}

func DisplayName(providerName string) string {
	switch providerName {
	case ProviderGitHub:
		return "GitHub"
	case ProviderGitLab:
		return "GitLab"
	case ProviderUpload:
		return "Upload"
	default:
		return providerName
	}
}
