package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/usepackyard/packyard/internal/provider"
)

func init() {
	provider.RegisterConfigured("gitlab", func(token, connectionConfig string) (provider.Provider, error) {
		cfg, err := provider.ParseConnectionConfig(provider.ProviderGitLab, connectionConfig)
		if err != nil {
			return nil, err
		}
		return New(token, cfg.Host), nil
	})
}

const maxReleasesPerSync = 1000

type GitLab struct {
	httpClient *http.Client
	token      string
	host       string
	apiBase    string
}

func New(token, host string) *GitLab {
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	host = strings.TrimRight(host, "/")
	if host == "" {
		host = "gitlab.com"
	}
	return &GitLab{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		token:      token,
		host:       host,
		apiBase:    "https://" + host + "/api/v4",
	}
}

type releaseJSON struct {
	TagName    string     `json:"tag_name"`
	ReleasedAt *time.Time `json:"released_at"`
	CreatedAt  *time.Time `json:"created_at"`
	Assets     struct {
		Links []assetLinkJSON `json:"links"`
	} `json:"assets"`
}

type assetLinkJSON struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	DirectAssetURL string `json:"direct_asset_url"`
	LinkType       string `json:"link_type"`
}

func (g *GitLab) ListReleases(ctx context.Context, owner, repo string) ([]provider.Release, error) {
	project := url.PathEscape(owner + "/" + repo)
	nextURL := fmt.Sprintf("%s/projects/%s/releases?per_page=100", g.apiBase, project)
	var releases []provider.Release

	for nextURL != "" {
		req, err := http.NewRequestWithContext(ctx, "GET", nextURL, nil)
		if err != nil {
			return nil, err
		}
		g.setHeaders(req)
		resp, err := g.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("list releases: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("list releases: status %d: %s", resp.StatusCode, string(body))
		}
		var raw []releaseJSON
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode releases: %w", err)
		}
		for _, r := range raw {
			var publishedAt time.Time
			if r.ReleasedAt != nil {
				publishedAt = *r.ReleasedAt
			} else if r.CreatedAt != nil {
				publishedAt = *r.CreatedAt
			}
			rel := provider.Release{
				TagName:     r.TagName,
				PublishedAt: publishedAt,
			}
			for _, a := range r.Assets.Links {
				assetURL := a.DirectAssetURL
				if assetURL == "" {
					assetURL = a.URL
				}
				assetURL = g.absoluteURL(assetURL)
				rel.Assets = append(rel.Assets, provider.Asset{
					Name: a.Name,
					URL:  assetURL,
				})
			}
			releases = append(releases, rel)
			if len(releases) >= maxReleasesPerSync {
				resp.Body.Close()
				return releases, nil
			}
		}
		nextURL = ""
		if next := resp.Header.Get("X-Next-Page"); next != "" {
			u, err := url.Parse(resp.Request.URL.String())
			if err == nil {
				q := u.Query()
				q.Set("page", next)
				u.RawQuery = q.Encode()
				nextURL = u.String()
			}
		}
		resp.Body.Close()
	}

	return releases, nil
}

func (g *GitLab) DownloadAsset(ctx context.Context, assetURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", assetURL, nil)
	if err != nil {
		return nil, err
	}
	g.setHeadersForURL(req)
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download asset: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download asset: status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func (g *GitLab) DownloadSourceArchive(ctx context.Context, owner, repo, tag string) (io.ReadCloser, error) {
	project := url.PathEscape(owner + "/" + repo)
	u := fmt.Sprintf("%s/projects/%s/repository/archive.zip?sha=%s", g.apiBase, project, url.QueryEscape(tag))
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	g.setHeaders(req)
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download source archive: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download source archive: status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func (g *GitLab) setHeaders(req *http.Request) {
	if g.token != "" {
		req.Header.Set("PRIVATE-TOKEN", g.token)
	}
	req.Header.Set("Accept", "application/json")
}

func (g *GitLab) setHeadersForURL(req *http.Request) {
	if g.token != "" && sameHost(req.URL.Host, g.host) {
		req.Header.Set("PRIVATE-TOKEN", g.token)
	}
}

func sameHost(got, want string) bool {
	return strings.EqualFold(got, want)
}

func (g *GitLab) absoluteURL(raw string) string {
	if strings.HasPrefix(raw, "/") {
		return "https://" + g.host + raw
	}
	return raw
}
