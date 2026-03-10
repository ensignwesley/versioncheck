// versioncheck — compare a locally installed version against the latest GitHub release.
//
// Usage:
//   go run versioncheck.go --repo owner/repo --local v1.0.0
//
// Exit codes:
//   0 — up to date
//   1 — usage or API error
//   2 — outdated (useful for scripting)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const apiURL = "https://api.github.com/repos/%s/releases/latest"

type release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Name    string `json:"name"`
}

func main() {
	repo        := flag.String("repo",        "", "GitHub owner/repo (e.g. nginx/nginx)")
	local       := flag.String("local",       "", "Installed version (e.g. v1.24.0)")
	stripPrefix := flag.String("strip-prefix", "", "Strip prefix from release tag before parsing (e.g. 'release-' for nginx)")
	flag.Parse()

	if *repo == "" || *local == "" {
		fmt.Fprintln(os.Stderr, "usage: versioncheck --repo owner/repo --local vX.Y.Z")
		os.Exit(1)
	}

	parts := strings.SplitN(*repo, "/", 2)
	if len(parts) != 2 {
		fmt.Fprintf(os.Stderr, "invalid repo format %q — want owner/repo\n", *repo)
		os.Exit(1)
	}
	name := parts[1]

	rel, err := latestRelease(*repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", *repo, err)
		os.Exit(1)
	}

	latestTag := rel.TagName
	if *stripPrefix != "" {
		latestTag = strings.TrimPrefix(latestTag, *stripPrefix)
	}

	cmp := compareSemver(*local, latestTag)
	switch {
	case cmp == 0:
		fmt.Printf("%s: local %s, latest %s — UP TO DATE\n", name, *local, latestTag)
	case cmp > 0:
		fmt.Printf("%s: local %s, latest %s — AHEAD (pre-release or manual build?)\n", name, *local, latestTag)
	default:
		fmt.Printf("%s: local %s, latest %s — OUTDATED  %s\n", name, *local, latestTag, rel.HTMLURL)
		os.Exit(2)
	}
}

// latestRelease fetches the latest release from the GitHub API.
func latestRelease(repo string) (*release, error) {
	url := fmt.Sprintf(apiURL, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "versioncheck/0.1 (github.com/ensignwesley/versioncheck)")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		// ok
	case 404:
		return nil, fmt.Errorf("no releases found (repo may not exist or have no releases)")
	case 403:
		return nil, fmt.Errorf("rate limited — set GITHUB_TOKEN env var for higher limits")
	default:
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	return &rel, nil
}

// compareSemver returns -1, 0, or 1 comparing a to b.
// Strips leading "v" and pre-release suffixes (e.g. v1.2.3-beta → 1.2.3).
// Falls back to string equality if parsing fails.
func compareSemver(a, b string) int {
	an := parseVer(a)
	bn := parseVer(b)
	for i := 0; i < 3; i++ {
		if an[i] < bn[i] {
			return -1
		}
		if an[i] > bn[i] {
			return 1
		}
	}
	return 0
}

func parseVer(s string) [3]int {
	s = strings.TrimPrefix(s, "v")
	// Drop pre-release / build metadata: v1.2.3-beta+001 → "1.2.3"
	if idx := strings.IndexAny(s, "-+"); idx != -1 {
		s = s[:idx]
	}
	parts := strings.SplitN(s, ".", 3)
	var out [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		n, _ := strconv.Atoi(p)
		out[i] = n
	}
	return out
}
