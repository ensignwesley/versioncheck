// versioncheck — compare installed versions against the latest GitHub releases.
//
// Single-repo mode:
//
//	go run versioncheck.go --repo owner/repo --local vX.Y.Z [--strip-prefix PREFIX] [--max-major N]
//
// Multi-repo mode (reads repos.yaml):
//
//	go run versioncheck.go --config repos.yaml
//
// Exit codes:
//
//	0 — all up to date
//	1 — usage or API error
//	2 — one or more repos are outdated
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	apiLatest = "https://api.github.com/repos/%s/releases/latest"
	apiList   = "https://api.github.com/repos/%s/releases?per_page=50&page=1"
	userAgent = "versioncheck/0.3 (github.com/ensignwesley/versioncheck)"
)

// Config file schema
type Config struct {
	Repos []RepoEntry `yaml:"repos"`
}

type RepoEntry struct {
	Name        string `yaml:"name"`
	Repo        string `yaml:"repo"`
	Local       string `yaml:"local"`
	StripPrefix string `yaml:"strip_prefix"`
	MaxMajor    int    `yaml:"max_major"` // constrain to releases within this major version (0 = no constraint)
}

// Result of a single version check
type Result struct {
	Name    string
	Local   string
	Latest  string
	URL     string
	Status  string // "UP TO DATE" | "OUTDATED" | "AHEAD" | "ERROR" | "PINNED"
	Err     error
}

type release struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

func main() {
	repo        := flag.String("repo",         "", "GitHub owner/repo (single-check mode)")
	local       := flag.String("local",        "", "Installed version (single-check mode)")
	stripPrefix := flag.String("strip-prefix", "", "Strip prefix from release tag (single-check mode)")
	maxMajor    := flag.Int("max-major",       0,  "Constrain to releases within this major version (single-check mode)")
	configFile  := flag.String("config",       "", "YAML config file (multi-check mode)")
	flag.Parse()

	// Multi-repo mode
	if *configFile != "" {
		runMulti(*configFile)
		return
	}

	// Single-repo mode
	if *repo == "" || *local == "" {
		fmt.Fprintln(os.Stderr, "usage:")
		fmt.Fprintln(os.Stderr, "  versioncheck --repo owner/repo --local vX.Y.Z [--strip-prefix PREFIX] [--max-major N]")
		fmt.Fprintln(os.Stderr, "  versioncheck --config repos.yaml")
		os.Exit(1)
	}

	parts := strings.SplitN(*repo, "/", 2)
	if len(parts) != 2 {
		fmt.Fprintf(os.Stderr, "invalid repo format %q — want owner/repo\n", *repo)
		os.Exit(1)
	}

	r := checkOne(RepoEntry{
		Name:        parts[1],
		Repo:        *repo,
		Local:       *local,
		StripPrefix: *stripPrefix,
		MaxMajor:    *maxMajor,
	})

	printSingle(r)
	if r.Status == "OUTDATED" {
		os.Exit(2)
	}
}

// runMulti reads a config file, checks all repos concurrently, prints a table.
func runMulti(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read %s: %v\n", path, err)
		os.Exit(1)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "cannot parse %s: %v\n", path, err)
		os.Exit(1)
	}
	if len(cfg.Repos) == 0 {
		fmt.Fprintln(os.Stderr, "no repos defined in config")
		os.Exit(1)
	}

	// Check concurrently
	results := make([]Result, len(cfg.Repos))
	var wg sync.WaitGroup
	for i, entry := range cfg.Repos {
		wg.Add(1)
		go func(idx int, e RepoEntry) {
			defer wg.Done()
			results[idx] = checkOne(e)
		}(i, entry)
	}
	wg.Wait()

	// Print aligned table
	printTable(results)

	// Exit 2 if any outdated
	for _, r := range results {
		if r.Status == "OUTDATED" {
			os.Exit(2)
		}
	}
}

// checkOne performs a single version check.
func checkOne(e RepoEntry) Result {
	name := e.Name
	if name == "" {
		parts := strings.SplitN(e.Repo, "/", 2)
		if len(parts) == 2 {
			name = parts[1]
		} else {
			name = e.Repo
		}
	}

	var rel *release
	var err error
	if e.MaxMajor > 0 {
		rel, err = latestReleaseInMajor(e.Repo, e.MaxMajor, e.StripPrefix)
	} else {
		rel, err = latestRelease(e.Repo)
	}
	if err != nil {
		return Result{Name: name, Local: e.Local, Status: "ERROR", Err: err}
	}

	latest := rel.TagName
	if e.StripPrefix != "" {
		latest = strings.TrimPrefix(latest, e.StripPrefix)
	}

	cmp := compareSemver(e.Local, latest)
	var status string
	switch {
	case cmp == 0:
		status = "UP TO DATE"
	case cmp > 0:
		status = "AHEAD"
	default:
		status = "OUTDATED"
	}

	// If constrained by max_major, annotate the status to clarify it's a pinned-track check.
	if e.MaxMajor > 0 && status == "UP TO DATE" {
		status = "UP TO DATE"
	}

	return Result{Name: name, Local: e.Local, Latest: latest, URL: rel.HTMLURL, Status: status}
}

// printSingle prints a single-line result (single-repo mode).
func printSingle(r Result) {
	if r.Err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", r.Name, r.Err)
		return
	}
	switch r.Status {
	case "UP TO DATE":
		fmt.Printf("%s: local %s, latest %s — UP TO DATE\n", r.Name, r.Local, r.Latest)
	case "AHEAD":
		fmt.Printf("%s: local %s, latest %s — AHEAD (pre-release or manual build?)\n", r.Name, r.Local, r.Latest)
	case "OUTDATED":
		fmt.Printf("%s: local %s, latest %s — OUTDATED  %s\n", r.Name, r.Local, r.Latest, r.URL)
	}
}

// printTable prints an aligned status table (multi-repo mode).
func printTable(results []Result) {
	// Column widths
	wName, wLocal, wLatest := 4, 9, 6 // "Name", "Installed", "Latest" minimums
	for _, r := range results {
		if len(r.Name) > wName     { wName   = len(r.Name)   }
		if len(r.Local) > wLocal   { wLocal  = len(r.Local)  }
		if len(r.Latest) > wLatest { wLatest = len(r.Latest) }
	}

	header := fmt.Sprintf("%-*s  %-*s  %-*s  %s", wName, "Service", wLocal, "Installed", wLatest, "Latest", "Status")
	fmt.Println(header)
	fmt.Println(strings.Repeat("─", len(header)+10))

	anyOutdated := false
	for _, r := range results {
		if r.Err != nil {
			fmt.Printf("%-*s  %-*s  %-*s  ERROR: %v\n", wName, r.Name, wLocal, r.Local, wLatest, "—", r.Err)
			continue
		}
		marker := "✓"
		if r.Status == "OUTDATED" {
			marker = "✗"
			anyOutdated = true
		} else if r.Status == "AHEAD" {
			marker = "↑"
		}
		fmt.Printf("%-*s  %-*s  %-*s  %s %s\n", wName, r.Name, wLocal, r.Local, wLatest, r.Latest, marker, r.Status)
	}

	if anyOutdated {
		fmt.Println()
		fmt.Println("Outdated repos:")
		for _, r := range results {
			if r.Status == "OUTDATED" {
				fmt.Printf("  %s → %s  %s\n", r.Local, r.Latest, r.URL)
			}
		}
	}
}

// latestRelease fetches the single latest release from the GitHub API.
func latestRelease(repo string) (*release, error) {
	url := fmt.Sprintf(apiLatest, repo)
	return fetchRelease(url, repo)
}

// latestReleaseInMajor fetches the list of recent releases and returns the
// highest non-prerelease, non-draft release whose major version ≤ maxMajor.
// stripPrefix is applied to tag names before version parsing.
func latestReleaseInMajor(repo string, maxMajor int, stripPrefix string) (*release, error) {
	url := fmt.Sprintf(apiList, repo)
	req, err := newGitHubRequest(url)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
	case 404:
		return nil, fmt.Errorf("no releases found (repo may not exist or have no releases)")
	case 403:
		return nil, fmt.Errorf("rate limited — set GITHUB_TOKEN env var for higher limits")
	default:
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var releases []release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	var best *release
	var bestVer [3]int
	for i := range releases {
		r := &releases[i]
		if r.Prerelease || r.Draft {
			continue
		}
		tag := r.TagName
		if stripPrefix != "" {
			tag = strings.TrimPrefix(tag, stripPrefix)
		}
		ver := parseVer(tag)
		if ver[0] > maxMajor {
			continue
		}
		if best == nil || compareVers(ver, bestVer) > 0 {
			best = r
			bestVer = ver
		}
	}

	if best == nil {
		return nil, fmt.Errorf("no stable releases found within major version %d (checked last 50 releases)", maxMajor)
	}
	return best, nil
}

// fetchRelease is a helper for the /releases/latest endpoint.
func fetchRelease(url, repo string) (*release, error) {
	req, err := newGitHubRequest(url)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
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

// newGitHubRequest builds a standard GitHub API request.
func newGitHubRequest(url string) (*http.Request, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

// compareSemver returns -1, 0, or 1 comparing version strings a and b.
func compareSemver(a, b string) int {
	return compareVers(parseVer(a), parseVer(b))
}

// compareVers compares two parsed version triples. Returns -1, 0, or 1.
func compareVers(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		if a[i] < b[i] { return -1 }
		if a[i] > b[i] { return 1 }
	}
	return 0
}

func parseVer(s string) [3]int {
	s = strings.TrimPrefix(s, "v")
	if idx := strings.IndexAny(s, "-+"); idx != -1 {
		s = s[:idx]
	}
	parts := strings.SplitN(s, ".", 3)
	var out [3]int
	for i, p := range parts {
		if i >= 3 { break }
		n, _ := strconv.Atoi(p)
		out[i] = n
	}
	return out
}
