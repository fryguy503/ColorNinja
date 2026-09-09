// Package updates checks public GitHub releases. It never downloads or installs
// executables and never sends source images, library paths, or settings.
package updates

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const RepositoryURL = "https://github.com/fryguy503/ColorNinja"
const releasesAPI = "https://api.github.com/repos/fryguy503/ColorNinja/releases"

func ReleasePage(tag string) (string, error) {
	if tag == "" {
		return RepositoryURL + "/releases", nil
	}
	if _, ok := parseVersion(tag); !ok {
		return "", fmt.Errorf("invalid release tag")
	}
	return RepositoryURL + "/releases/tag/" + url.PathEscape(tag), nil
}

type Result struct {
	CurrentVersion     string    `json:"currentVersion"`
	LatestVersion      string    `json:"latestVersion"`
	Available          bool      `json:"available"`
	Prerelease         bool      `json:"prerelease"`
	IncludePrereleases bool      `json:"includePrereleases"`
	ReleaseURL         string    `json:"releaseURL"`
	CheckedAt          time.Time `json:"checkedAt"`
}
type release struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}
type Checker struct {
	mu       sync.Mutex
	client   *http.Client
	endpoint string
	current  string
	cache    map[bool]Result
}

func New(current string) *Checker {
	return &Checker{current: current, endpoint: releasesAPI, client: &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, cache: make(map[bool]Result)}
}
func (c *Checker) Check(ctx context.Context, includePrereleases, force bool) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	ttl := 15 * time.Minute
	if force {
		ttl = time.Minute
	}
	if cached, ok := c.cache[includePrereleases]; ok && time.Since(cached.CheckedAt) < ttl {
		return cached, nil
	}
	current, ok := parseVersion(c.current)
	if !ok {
		return Result{}, fmt.Errorf("this build has no valid release version")
	}
	result := Result{CurrentVersion: c.current, IncludePrereleases: includePrereleases}
	var latest version
	found := false
	for page := 1; page <= 10; page++ {
		req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s?per_page=100&page=%d", c.endpoint, page), nil)
		if err != nil {
			return Result{}, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
		req.Header.Set("User-Agent", "ColorNinja/"+c.current)
		response, err := c.client.Do(req)
		if err != nil {
			return Result{}, fmt.Errorf("could not reach GitHub. Check your internet connection and try again")
		}
		items, next, err := readReleases(response)
		if err != nil {
			return Result{}, err
		}
		for _, r := range items {
			v, valid := parseVersion(r.Tag)
			pre := r.Prerelease || len(v.pre) > 0
			if r.Draft || !valid || (!includePrereleases && pre) {
				continue
			}
			if !found || v.compare(latest) > 0 {
				latest, found = v, true
				result.LatestVersion, result.Prerelease = r.Tag, pre
				// Construct a repository-scoped URL instead of trusting remote HTML.
				result.ReleaseURL, _ = ReleasePage(r.Tag)
			}
		}
		if !next {
			break
		}
		if page == 10 {
			return Result{}, fmt.Errorf("GitHub returned too many releases to finish checking; open the releases page")
		}
	}
	result.Available = found && latest.compare(current) > 0
	result.CheckedAt = time.Now().UTC()
	c.cache[includePrereleases] = result
	return result, nil
}
func readReleases(response *http.Response) ([]release, bool, error) {
	defer response.Body.Close()
	if response.StatusCode == 403 || response.StatusCode == 429 {
		return nil, false, fmt.Errorf("GitHub rate-limited or denied the check; try again later or open the releases page")
	}
	if response.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("GitHub update check failed (HTTP %d); try again later", response.StatusCode)
	}
	const limit = 4 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, false, fmt.Errorf("could not finish reading GitHub releases. Please try again")
	}
	if len(body) > limit {
		return nil, false, fmt.Errorf("GitHub release response is too large")
	}
	var releases []release
	if err = json.Unmarshal(body, &releases); err != nil || releases == nil {
		return nil, false, fmt.Errorf("GitHub returned an invalid release list")
	}
	return releases, strings.Contains(response.Header.Get("Link"), `rel="next"`), nil
}
