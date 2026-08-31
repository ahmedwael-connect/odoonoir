package github

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-github/v60/github"
)

// PublishStandardOptions configures standard Odoo module publish.
type PublishStandardOptions struct {
	ModulePath  string   // local path to module (e.g. /home/.../custom_addons/my_module)
	Owner       string
	Repo        string
	Branch      string // target branch, default 18.0 / main
	Private     bool
	License     string // LGPL-3, MIT, AGPL-3, OPL-1
	Description string
	Topics      []string
	Message     string // commit message
	CreateTag   bool   // create git tag v{version}
}

// ValidateManifestStandard checks required Odoo manifest fields + version + license.
func ValidateManifestStandard(m *Manifest) []string {
	var errs []string
	if m.Name == "" {
		errs = append(errs, "manifest missing 'name'")
	}
	if m.Summary == "" {
		errs = append(errs, "manifest missing 'summary'")
	}
	if m.Author == "" {
		errs = append(errs, "manifest missing 'author'")
	}
	if m.License == "" {
		errs = append(errs, "manifest missing 'license' (LGPL-3/MIT/AGPL-3/OPL-1)")
	} else {
		allowed := map[string]bool{"LGPL-3": true, "MIT": true, "AGPL-3": true, "OPL-1": true, "LGPL": true, "AGPL": true}
		if !allowed[m.License] && !strings.HasPrefix(m.License, "LGPL") {
			errs = append(errs, fmt.Sprintf("license %q not standard (choose LGPL-3/MIT/AGPL-3/OPL-1)", m.License))
		}
	}
	// Odoo version: e.g. 18.0.1.0.0 or 1.0.0
	verRe := regexp.MustCompile(`^(\d+\.)*\d+$`)
	if m.Version == "" {
		errs = append(errs, "manifest missing 'version' (e.g. 18.0.1.0.0)")
	} else if !verRe.MatchString(m.Version) {
		errs = append(errs, fmt.Sprintf("version %q must be numeric dotted (e.g. 18.0.1.0.0)", m.Version))
	}
	if len(m.Depends) == 0 {
		// base is implicit, but warn if really empty
		// not error
	}
	return errs
}

// EnsureStandardStructure scaffolds missing static/description files.
func EnsureStandardStructure(modulePath string, m *Manifest) []string {
	var created []string
	descDir := filepath.Join(modulePath, "static", "description")
	if _, err := os.Stat(filepath.Join(descDir, "index.html")); os.IsNotExist(err) {
		_ = os.MkdirAll(descDir, 0o755)
		html := fmt.Sprintf("<section><h1>%s</h1><p>%s</p><p>Author: %s | License: %s</p></section>\n", m.Name, m.Summary, m.Author, m.License)
		if err := os.WriteFile(filepath.Join(descDir, "index.html"), []byte(html), 0o644); err == nil {
			created = append(created, "static/description/index.html")
		}
	}
	icon := filepath.Join(descDir, "icon.png")
	if _, err := os.Stat(icon); os.IsNotExist(err) {
		// placeholder: copy from parent or leave out
	}
	if _, err := os.Stat(filepath.Join(modulePath, "README.md")); os.IsNotExist(err) {
		readme := fmt.Sprintf("# %s\n\n%s\n\n## License\n%s\n", m.Name, m.Summary, m.License)
		if err := os.WriteFile(filepath.Join(modulePath, "README.md"), []byte(readme), 0o644); err == nil {
			created = append(created, "README.md")
		}
	}
	if _, err := os.Stat(filepath.Join(modulePath, "i18n")); os.IsNotExist(err) {
		_ = os.MkdirAll(filepath.Join(modulePath, "i18n"), 0o755)
	}
	return created
}

// PublishStandard validates, ensures structure, git pushes and creates release.
func (c *Client) PublishStandard(ctx context.Context, opts PublishStandardOptions) (*PublishStandardResult, error) {
	// 1. validate module path
	manifestPath := filepath.Join(opts.ModulePath, "__manifest__.py")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		manifestPath = filepath.Join(opts.ModulePath, "__openerp__.py")
		if _, err := os.Stat(manifestPath); err != nil {
			return nil, fmt.Errorf("manifest not found at %s/__manifest__.py", opts.ModulePath)
		}
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	manifest, err := parseManifest(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if errs := ValidateManifestStandard(manifest); len(errs) > 0 {
		return nil, fmt.Errorf("manifest validation failed: %s", strings.Join(errs, "; "))
	}
	created := EnsureStandardStructure(opts.ModulePath, manifest)

	// 2. ensure git repo
	branch := opts.Branch
	if branch == "" {
		branch = "18.0"
		if repoObj, _, _ := c.client.Repositories.Get(ctx, opts.Owner, opts.Repo); repoObj != nil && repoObj.GetDefaultBranch() != "" {
			branch = repoObj.GetDefaultBranch()
		}
	}
	// check if already git repo (look upwards)
	repoRoot := findGitRoot(opts.ModulePath)
	if repoRoot == "" {
		// init in module path parent? Use modulePath as repo root for single-module repos
		repoRoot = opts.ModulePath
		if err := runGit(ctx, repoRoot, "init"); err != nil {
			return nil, fmt.Errorf("git init: %w", err)
		}
	}
	// set remote
	token, _ := func() (string, error) {
		s, _ := NewFileTokenStore()
		if s != nil {
			return s.Get()
		}
		return "", nil
	}()
	remoteURL := fmt.Sprintf("https://github.com/%s/%s.git", opts.Owner, opts.Repo)
	if token != "" {
		remoteURL = fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", token, opts.Owner, opts.Repo)
	}
	_ = runGit(ctx, repoRoot, "remote", "remove", "origin")
	_ = runGit(ctx, repoRoot, "remote", "add", "origin", remoteURL)

	// ensure repo exists on GitHub (create if missing)
	repo := &github.Repository{
		Name:        github.String(opts.Repo),
		Description: github.String(opts.Description),
		Private:     github.Bool(opts.Private),
		AutoInit:    github.Bool(false),
	}
	if opts.License != "" {
		repo.LicenseTemplate = github.String(strings.ToLower(strings.ReplaceAll(opts.License, "-", "")))
	}
	_, _, err = c.client.Repositories.Create(ctx, opts.Owner, repo)
	if err != nil && !strings.Contains(err.Error(), "already exists") && !strings.Contains(strings.ToLower(err.Error()), "name already exists") {
		// not fatal if already exists — continue to push
	}

	// 3. git add/commit/push
	msg := opts.Message
	if msg == "" {
		msg = fmt.Sprintf("publish %s v%s", manifest.Name, manifest.Version)
	}
	_ = runGit(ctx, repoRoot, "add", ".")
	_ = runGit(ctx, repoRoot, "config", "user.email", "odoonoir@local")
	_ = runGit(ctx, repoRoot, "config", "user.name", "odoonoir")
	commitErr := runGit(ctx, repoRoot, "commit", "-m", msg)
	hasCommit := commitErr == nil
	// create branch if needed
	_ = runGit(ctx, repoRoot, "checkout", "-B", branch)
	pushErr := runGit(ctx, repoRoot, "push", "-u", "origin", branch)
	if pushErr != nil {
		return nil, fmt.Errorf("git push failed: %w — check token scopes (repo) and remote", pushErr)
	}
	// 4. tag/release if requested
	var tagName string
	if opts.CreateTag {
		tagName = "v" + manifest.Version
		_ = runGit(ctx, repoRoot, "tag", "-f", tagName)
		_ = runGit(ctx, repoRoot, "push", "origin", tagName, "--force")
		rel := &github.RepositoryRelease{
			TagName: github.String(tagName),
			Name:    github.String(fmt.Sprintf("%s %s", manifest.Name, tagName)),
			Body:    github.String(manifest.Summary),
		}
		_, _, _ = c.client.Repositories.CreateRelease(ctx, opts.Owner, opts.Repo, rel)
	}

	// 5. topics
	if len(opts.Topics) > 0 {
		_, _, _ = c.client.Repositories.ReplaceAllTopics(ctx, opts.Owner, opts.Repo, opts.Topics)
	}

	return &PublishStandardResult{
		Manifest:  manifest,
		Branch:    branch,
		HasCommit: hasCommit,
		Tag:       tagName,
		Created:   created,
		RepoURL:   fmt.Sprintf("https://github.com/%s/%s", opts.Owner, opts.Repo),
	}, nil
}

// PublishStandardResult reports what was done.
type PublishStandardResult struct {
	Manifest  *Manifest `json:"manifest"`
	Branch    string    `json:"branch"`
	HasCommit bool      `json:"hasCommit"`
	Tag       string    `json:"tag"`
	Created   []string  `json:"created"`
	RepoURL   string    `json:"repoUrl"`
}

func findGitRoot(start string) string {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir || parent == "." {
			return ""
		}
		dir = parent
	}
}

func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w\n%s", strings.Join(append([]string{"git"}, args...), " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ListBranches lists remote branches for a repo.
func (c *Client) ListBranches(ctx context.Context, owner, repo string) ([]string, error) {
	branches, _, err := c.client.Repositories.ListBranches(ctx, owner, repo, &github.BranchListOptions{ListOptions: github.ListOptions{PerPage: 100}})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(branches))
	for _, b := range branches {
		out = append(out, b.GetName())
	}
	return out, nil
}
