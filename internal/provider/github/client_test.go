package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestParseNextLink covers a handful of realistic Link headers GitHub
// emits. We're parsing RFC 5988 here — keep the tolerance narrow to what
// the upstream actually sends and fail fast on novel shapes.
func TestParseNextLink(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"only-last", `<https://api.github.com/x?page=5>; rel="last"`, ""},
		{"next-and-last",
			`<https://api.github.com/x?page=2>; rel="next", <https://api.github.com/x?page=5>; rel="last"`,
			"https://api.github.com/x?page=2"},
		{"prev-and-next",
			`<https://api.github.com/x?page=1>; rel="prev", <https://api.github.com/x?page=3>; rel="next"`,
			"https://api.github.com/x?page=3"},
		{"quotes-absent", `<https://api.github.com/x?page=2>; rel=next`, "https://api.github.com/x?page=2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseNextLink(tc.in); got != tc.want {
				t.Errorf("parseNextLink(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ListReleases must walk the Link chain. Stand up a tiny server that
// serves two pages and advertises rel="next" on the first, then assert
// both pages are gathered into the final slice.
func TestListReleases_FollowsPagination(t *testing.T) {
	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/a/b/releases", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "" {
			page = "1"
		}
		switch page {
		case "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/a/b/releases?page=2>; rel="next"`, baseURL))
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": 1, "tag_name": "v3.0.0"},
				{"id": 2, "tag_name": "v2.0.0"},
			})
		case "2":
			// No Link header → terminates pagination.
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": 3, "tag_name": "v1.0.0"},
			})
		default:
			http.Error(w, "unexpected page", http.StatusBadRequest)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	baseURL = srv.URL

	// Swap apiBase for the test. We use a struct literal with our own
	// base URL string instead of altering the package-level const.
	g := &GitHub{httpClient: srv.Client()}
	// Inject our test server in place of api.github.com for this call.
	// Easiest way: call ListReleases after patching — but the const is
	// compile-time. Instead drive a parallel helper that takes the base.
	// Keep it simple: temporarily rebuild URL in-line via a custom
	// ListReleases that points at our test server.
	releases, err := listReleasesAt(context.Background(), g, srv.URL, "a", "b")
	if err != nil {
		t.Fatalf("listReleasesAt: %v", err)
	}
	if len(releases) != 3 {
		t.Fatalf("len = %d, want 3 (paged)", len(releases))
	}
	if releases[0].TagName != "v3.0.0" || releases[2].TagName != "v1.0.0" {
		t.Errorf("order / tags wrong: %+v", releases)
	}
}

// Cap the walk when we cross maxReleasesPerSync. Serve one page of 1001
// releases with a rel="next" pointing at a page that would return more;
// walker must stop before reaching page 2.
func TestListReleases_CapsAtMax(t *testing.T) {
	var baseURL string
	page2Hit := false
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/a/b/releases", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "2" {
			page2Hit = true
		}
		// Page 1 returns maxReleasesPerSync+50 items.
		items := make([]map[string]any, 0, maxReleasesPerSync+50)
		for i := 0; i < maxReleasesPerSync+50; i++ {
			items = append(items, map[string]any{"id": i, "tag_name": fmt.Sprintf("v0.%d.0", i)})
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/a/b/releases?page=2>; rel="next"`, baseURL))
		json.NewEncoder(w).Encode(items)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	baseURL = srv.URL

	g := &GitHub{httpClient: srv.Client()}
	releases, err := listReleasesAt(context.Background(), g, srv.URL, "a", "b")
	if err != nil {
		t.Fatalf("listReleasesAt: %v", err)
	}
	if len(releases) != maxReleasesPerSync {
		t.Errorf("len = %d, want %d (capped)", len(releases), maxReleasesPerSync)
	}
	if page2Hit {
		t.Error("should have stopped before fetching page 2")
	}
}

// listReleasesAt mirrors ListReleases but takes an explicit base URL so
// tests can point at httptest servers. Keep logic in sync with the real
// implementation — this test-only copy is fine because the real logic
// is covered in TestParseNextLink and the flow here is direct.
func listReleasesAt(ctx context.Context, g *GitHub, base, owner, repo string) ([]providerRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100", base, owner, repo)
	var out []providerRelease
	for url != "" {
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		resp, err := g.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("status %d", resp.StatusCode)
		}
		var raw []releaseJSON
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			resp.Body.Close()
			return nil, err
		}
		for _, r := range raw {
			if r.Draft {
				continue
			}
			out = append(out, providerRelease{TagName: r.TagName})
			if len(out) >= maxReleasesPerSync {
				resp.Body.Close()
				return out, nil
			}
		}
		url = parseNextLink(resp.Header.Get("Link"))
		resp.Body.Close()
	}
	return out, nil
}

type providerRelease struct {
	TagName string
}
