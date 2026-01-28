package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/usepackyard/packyard/internal/provider"
)

func init() {
	provider.Register("github", func(token string) provider.Provider {
		return New(token)
	})
}

const apiBase = "https://api.github.com"

// maxReleasesPerSync caps how many releases a single sync will pull from
// GitHub. A repo with more than this many releases is almost certainly
// misusing releases as a changelog; we stop walking pagination at this
// threshold and log a warning upstream. The cap is intentionally a
// constant (not env-driven): operators needing more should rethink how
// they're using releases, not tune a knob.
const maxReleasesPerSync = 1000

type releaseJSON struct {
	ID          int64       `json:"id"`
	TagName     string      `json:"tag_name"`
	Draft       bool        `json:"draft"`
	PublishedAt *time.Time  `json:"published_at"` // nil for drafts; we filter those anyway
	CreatedAt   *time.Time  `json:"created_at"`   // fallback when published_at is missing
	Assets      []assetJSON `json:"assets"`
}

type assetJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

// GitHub implements the provider.Provider interface.
type GitHub struct {
	httpClient *http.Client
	token      string
}

// New creates a GitHub provider with the given auth token.
func New(token string) *GitHub {
	return &GitHub{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		token:      token,
	}
}

// ListReleases walks the GitHub API's Link-header pagination
// (RFC 5988 rel="next") until the chain is exhausted or the hard cap
// maxReleasesPerSync is reached. Fetching only the first page would
// silently lose older releases on any repo with >100 tags.
func (g *GitHub) ListReleases(ctx context.Context, owner, repo string) ([]provider.Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100", apiBase, owner, repo)
	var releases []provider.Release

	for url != "" {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
			if r.Draft {
				continue
			}
			// GitHub populates published_at on publish; prior to that
			// (or for never-published rows we shouldn't see here anyway)
			// fall back to created_at so versions don't end up stamped
			// with the import time.
			var publishedAt time.Time
			if r.PublishedAt != nil {
				publishedAt = *r.PublishedAt
			} else if r.CreatedAt != nil {
				publishedAt = *r.CreatedAt
			}
			rel := provider.Release{
				TagName:     r.TagName,
				Draft:       r.Draft,
				PublishedAt: publishedAt,
			}
			for _, a := range r.Assets {
				rel.Assets = append(rel.Assets, provider.Asset{
					Name: a.Name,
					URL:  a.URL,
					Size: a.Size,
				})
			}
			releases = append(releases, rel)
			if len(releases) >= maxReleasesPerSync {
				resp.Body.Close()
				return releases, nil
			}
		}

		url = parseNextLink(resp.Header.Get("Link"))
		resp.Body.Close()
	}

	return releases, nil
}

// parseNextLink extracts the rel="next" URL from a GitHub Link header, if
// present. Header format per RFC 5988:
//
//	<https://api.github.com/...?page=2>; rel="next", <https://...>; rel="last"
//
// Returns "" when there's no next page.
func parseNextLink(header string) string {
	if header == "" {
		return ""
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		// Each part is: <url>; rel="name"[; ...]
		sections := strings.Split(part, ";")
		if len(sections) < 2 {
			continue
		}
		urlPart := strings.TrimSpace(sections[0])
		if !strings.HasPrefix(urlPart, "<") || !strings.HasSuffix(urlPart, ">") {
			continue
		}
		url := urlPart[1 : len(urlPart)-1]
		for _, attr := range sections[1:] {
			attr = strings.TrimSpace(attr)
			if attr == `rel="next"` || attr == "rel=next" {
				return url
			}
		}
	}
	return ""
}

func (g *GitHub) DownloadAsset(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	g.setHeaders(req)
	req.Header.Set("Accept", "application/octet-stream")

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

func (g *GitHub) DownloadSourceArchive(ctx context.Context, owner, repo, tag string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/zipball/%s", apiBase, owner, repo, tag)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

func (g *GitHub) setHeaders(req *http.Request) {
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}
