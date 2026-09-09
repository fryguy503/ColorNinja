package updates

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSemanticVersionOrder(t *testing.T) {
	ordered := []string{"v1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.2.0", "1.10.0", "2.0.0"}
	for i := range ordered {
		for j := range ordered {
			a, ok := parseVersion(ordered[i])
			if !ok {
				t.Fatal(ordered[i])
			}
			b, _ := parseVersion(ordered[j])
			got := a.compare(b)
			if (i < j && got >= 0) || (i > j && got <= 0) || (i == j && got != 0) {
				t.Fatalf("%s vs %s: %d", ordered[i], ordered[j], got)
			}
		}
	}
	a, _ := parseVersion("v1.2.3+build.17")
	b, _ := parseVersion("1.2.3+build.99")
	if a.compare(b) != 0 {
		t.Fatal("build metadata changed precedence")
	}
	for _, bad := range []string{"latest", "v1.0", "1.02.3", "1.2.3-beta.01", "1.2.3-", "1.2.3\n", "1.2.3/../../other", "https://evil.invalid"} {
		if _, ok := parseVersion(bad); ok {
			t.Fatal("accepted", bad)
		}
		if _, err := ReleasePage(bad); err == nil {
			t.Fatal("unsafe release URL", bad)
		}
	}
}

func TestChannelsPaginationAndCache(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("User-Agent") != "ColorNinja/1.0.0-alpha.1" || r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Error("missing GitHub headers")
		}
		if r.URL.Query().Get("page") == "1" {
			w.Header().Set("Link", `<https://untrusted.invalid/next>; rel="next"`)
			fmt.Fprint(w, `[{"tag_name":"v1.1.0-beta.11","prerelease":true},{"tag_name":"v1.1.0-beta.2","prerelease":true},{"tag_name":"v99.0.0","draft":true},{"tag_name":"v1.0.0-rc.1"},{"tag_name":"latest"}]`)
		} else {
			fmt.Fprint(w, `[{"tag_name":"v1.0.0"},{"tag_name":"v0.9.0"}]`)
		}
	}))
	defer srv.Close()
	c := New("1.0.0-alpha.1")
	c.endpoint = srv.URL
	stable, err := c.Check(context.Background(), false, true)
	if err != nil || stable.LatestVersion != "v1.0.0" || !stable.Available || stable.Prerelease {
		t.Fatalf("stable: %+v %v", stable, err)
	}
	preview, err := c.Check(context.Background(), true, true)
	if err != nil || preview.LatestVersion != "v1.1.0-beta.11" || !preview.Available || !preview.Prerelease {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if preview.ReleaseURL != RepositoryURL+"/releases/tag/v1.1.0-beta.11" {
		t.Fatal("wrong download destination")
	}
	_, _ = c.Check(context.Background(), false, true)
	_, _ = c.Check(context.Background(), true, false)
	if requests != 4 {
		t.Fatal("checks did not use separate channel caches", requests)
	}
}

func TestNoDowngradeAndNoStableRelease(t *testing.T) {
	for _, tt := range []struct{ current, body, latest string }{
		{"2.0.0", `[{"tag_name":"v1.0.0"}]`, "v1.0.0"},
		{"1.0.0", `[{"tag_name":"v1.0.0"}]`, "v1.0.0"},
		{"1.0.0-alpha.1", `[{"tag_name":"v1.0.0-beta.1","prerelease":true}]`, ""},
		{"1.0.0-alpha.1", `[{"tag_name":"v1.0.0-beta.1"}]`, ""},
		{"1.0.0", `[]`, ""},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tt.body) }))
		c := New(tt.current)
		c.endpoint = srv.URL
		r, err := c.Check(context.Background(), false, true)
		srv.Close()
		if err != nil || r.Available || r.LatestVersion != tt.latest {
			t.Fatalf("%+v %v", r, err)
		}
	}
}

func TestErrorsNeverReportUpToDate(t *testing.T) {
	for _, tt := range []struct {
		status int
		body   string
	}{{403, ""}, {429, ""}, {404, ""}, {500, ""}, {200, "null"}, {200, "{}"}, {200, "broken"}, {200, strings.Repeat(" ", (4<<20)+1)}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tt.status); fmt.Fprint(w, tt.body) }))
		c := New("1.0.0")
		c.endpoint = srv.URL
		_, err := c.Check(context.Background(), false, true)
		srv.Close()
		if err == nil || len(c.cache) != 0 {
			t.Fatal("failed check was treated as success", tt.status)
		}
	}
	c := New("1.0.0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Check(ctx, false, true); err == nil {
		t.Fatal("ignored cancellation")
	}
}
