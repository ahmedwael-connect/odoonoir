package selfupdate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	repoOwner = "ahmedwael-connect"
	repoName  = "odoonoir"
)

// Release holds info from the GitHub releases API.
type Release struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
}

// CheckResult is the outcome of a version check.
type CheckResult struct {
	Current  string
	Latest   string
	HasUpdate bool
}

// VersionCheck compares the running version against the latest GitHub release.
func VersionCheck(current string) (*CheckResult, error) {
	rel, err := fetchLatestRelease()
	if err != nil {
		return nil, fmt.Errorf("check for updates: %w", err)
	}
	latest := normalizeVersion(rel.TagName)
	cur := normalizeVersion(current)
	return &CheckResult{
		Current:   cur,
		Latest:    latest,
		HasUpdate: latest != "" && cur != "" && latest != cur,
	}, nil
}

// DownloadAndInstall downloads the latest .deb and installs it.
// progress is called with status messages.
func DownloadAndInstall(progress func(string)) error {
	progress("Checking latest release…")
	rel, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("fetch release: %w", err)
	}

	progress(fmt.Sprintf("Latest: %s", rel.TagName))

	// Find the .deb asset
	debURL, err := findDebAsset(rel.TagName)
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "odoonoir-update-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	debPath := filepath.Join(tmpDir, "odoonoir.deb")
	progress(fmt.Sprintf("Downloading %s…", filepath.Base(debURL)))

	if err := downloadFile(debURL, debPath); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	progress("Installing…")
	cmd := exec.Command("sudo", "dpkg", "-i", debPath)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dpkg install: %w", err)
	}

	progress("Done ✓")
	return nil
}

func fetchLatestRelease() (*Release, error) {
	// Try /releases/latest first (stable only)
	url := "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/releases/latest"
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var rel Release
		if err := json.NewDecoder(resp.Body).Decode(&rel); err == nil && rel.TagName != "" {
			return &rel, nil
		}
	}

	// Fallback: /releases (includes pre-releases)
	url = "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/releases"
	resp2, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != 200 {
		return nil, fmt.Errorf("no releases found at github.com/%s/%s", repoOwner, repoName)
	}

	var releases []Release
	if err := json.NewDecoder(resp2.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode releases: %w", err)
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found at github.com/%s/%s", repoOwner, repoName)
	}
	return &releases[0], nil
}

func findDebAsset(tag string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", repoOwner, repoName, tag)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var release struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("decode assets: %w", err)
	}

	for _, a := range release.Assets {
		if strings.HasSuffix(a.Name, ".deb") && strings.Contains(a.Name, "amd64") {
			return a.BrowserDownloadURL, nil
		}
	}
	// Fallback: any .deb
	for _, a := range release.Assets {
		if strings.HasSuffix(a.Name, ".deb") {
			return a.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("no .deb asset found in release %s", tag)
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// normalizeVersion strips leading "v" and trailing non-version chars.
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	// Extract semver-like prefix: X.Y.Z
	re := regexp.MustCompile(`^(\d+\.\d+\.\d+)`)
	if m := re.FindStringSubmatch(v); len(m) > 1 {
		return m[1]
	}
	return v
}
