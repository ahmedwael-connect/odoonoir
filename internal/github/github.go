package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ahmed/odoonoir/internal/instance"
	"github.com/google/go-github/v60/github"
	"golang.org/x/oauth2"
)

// Client wraps the GitHub API client with token management
type Client struct {
	client *github.Client
	token  string
}

// TokenStore manages GitHub OAuth tokens
type TokenStore interface {
	Get() (string, error)
	Set(token string) error
	Delete() error
}

// FileTokenStore stores tokens in a file
type FileTokenStore struct {
	path string
}

func NewFileTokenStore() (*FileTokenStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".config", "odoonoir", "github_token.json")
	return &FileTokenStore{path: path}, nil
}

func (s *FileTokenStore) Get() (string, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &token); err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

func (s *FileTokenStore) Set(token string) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(struct {
		AccessToken string `json:"access_token"`
	}{AccessToken: token}, "", "  ")
	return os.WriteFile(s.path, data, 0o600)
}

func (s *FileTokenStore) Delete() error {
	return os.Remove(s.path)
}

// NewClient creates a GitHub client with the given token
func NewClient(token string) *Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	return &Client{
		client: github.NewClient(tc),
		token:  token,
	}
}

// NewClientFromStore creates a client using token from store
func (c *Client) GoClient() *github.Client {
	return c.client
}

func NewClientFromStore(store TokenStore) (*Client, error) {
	token, err := store.Get()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("no GitHub token configured")
	}
	return NewClient(token), nil
}

// ============================================
// Module Discovery
// ============================================

type SearchQuery struct {
	Query       string
	OdooVersion string
	MinStars    int
	Category    string
	Language    string
	UpdatedAfter string
	Sort        string
	Order       string
	Page        int
	PerPage     int
}

type Module struct {
	Owner          string
	Repo           string
	Name           string
	DisplayName    string
	Description    string
	Stars          int
	Forks          int
	License        string
	Topics         []string
	UpdatedAt      string
	DefaultBranch  string
	OdooVersions   []string
	IsPrivate      bool
}

type ModuleSearchResult struct {
	Modules      []Module
	TotalCount   int
	Page         int
	PerPage      int
	HasMore      bool
}

func (c *Client) SearchModules(ctx context.Context, query SearchQuery) (*ModuleSearchResult, error) {
	q := query.Query
	if q == "" {
		q = "topic:odoo"
	}
	if query.OdooVersion != "" {
		q += fmt.Sprintf(" odoo-%s", query.OdooVersion)
	}
	if query.MinStars > 0 {
		q += fmt.Sprintf(" stars:>=%d", query.MinStars)
	}
	if query.Category != "" {
		q += fmt.Sprintf(" topic:%s", query.Category)
	}
	if query.Language != "" {
		q += fmt.Sprintf(" language:%s", query.Language)
	}
	if query.UpdatedAfter != "" {
		q += fmt.Sprintf(" pushed:>%s", query.UpdatedAfter)
	}

	sort := query.Sort
	if sort == "" {
		sort = "stars"
	}
	order := query.Order
	if order == "" {
		order = "desc"
	}

	page := query.Page
	if page <= 0 {
		page = 1
	}
	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 30
	}

	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{Page: page, PerPage: perPage},
	}
	result, _, err := c.client.Search.Repositories(ctx, q, opts)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	modules := make([]Module, 0, len(result.Repositories))
	for _, repo := range result.Repositories {
		mod := Module{
			Owner:         repo.GetOwner().GetLogin(),
			Repo:          repo.GetName(),
			DisplayName:   repo.GetFullName(),
			Description:   repo.GetDescription(),
			Stars:         repo.GetStargazersCount(),
			Forks:         repo.GetForksCount(),
			License:       "",
			Topics:        repo.Topics,
			UpdatedAt:     repo.GetUpdatedAt().Format("2006-01-02"),
			DefaultBranch: repo.GetDefaultBranch(),
			IsPrivate:     repo.GetPrivate(),
		}
		if repo.License != nil {
			mod.License = repo.License.GetSPDXID()
		}
		mod.OdooVersions = extractOdooVersions(repo.Topics)
		modules = append(modules, mod)
	}

	return &ModuleSearchResult{
		Modules:    modules,
		TotalCount: result.GetTotal(),
		Page:       page,
		PerPage:    perPage,
		HasMore:    len(modules) == perPage,
	}, nil
}

func extractOdooVersions(topics []string) []string {
	var versions []string
	for _, t := range topics {
		if strings.HasPrefix(t, "odoo-") {
			ver := strings.TrimPrefix(t, "odoo-")
			versions = append(versions, ver)
		}
	}
	return versions
}

// ============================================
// Module Detail & Manifest
// ============================================

type ModuleDetail struct {
	Module
	Manifest      *Manifest
	SubModules    []SubModule
	Readme        string
	Releases      []Release
	Dependencies  []Dependency
	Dependents    []Module
}

type SubModule struct {
	Name        string
	Path        string
	Description string
}

type Manifest struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	License     string   `json:"license"`
	Category    string   `json:"category"`
	Depends     []string `json:"depends"`
	Data        []string `json:"data"`
	Demo        []string `json:"demo"`
	Installable bool     `json:"installable"`
	AutoInstall bool     `json:"auto_install"`
	Images      []string `json:"images"`
}

type Dependency struct {
	Name        string
	Version     string
	Repo        string
	IsOptional  bool
	Satisfied   bool
}

type Release struct {
	TagName    string
	Name       string
	PublishedAt string
	Body       string
	Prerelease bool
	Draft      bool
}

func (c *Client) GetModule(ctx context.Context, owner, repo string) (*ModuleDetail, error) {
	repoObj, _, err := c.client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("get repo failed: %w", err)
	}

	mod := Module{
		Owner:         repoObj.GetOwner().GetLogin(),
		Repo:          repoObj.GetName(),
		DisplayName:   repoObj.GetFullName(),
		Description:   repoObj.GetDescription(),
		Stars:         repoObj.GetStargazersCount(),
		Forks:         repoObj.GetForksCount(),
		License:       "",
		Topics:        repoObj.Topics,
		UpdatedAt:     repoObj.GetUpdatedAt().Format("2006-01-02"),
		DefaultBranch: repoObj.GetDefaultBranch(),
		IsPrivate:     repoObj.GetPrivate(),
	}
	if repoObj.License != nil {
		mod.License = repoObj.License.GetSPDXID()
	}
	mod.OdooVersions = extractOdooVersions(repoObj.Topics)

	manifest, err := c.GetManifest(ctx, owner, repo, repoObj.GetDefaultBranch())
	if err != nil {
		// Not fatal - some repos might not have a manifest
	}

	subModules, _ := c.GetSubModules(ctx, owner, repo, repoObj.GetDefaultBranch())
	readme, _ := c.GetReadme(ctx, owner, repo)
	releases, _ := c.GetReleases(ctx, owner, repo)

	return &ModuleDetail{
		Module:     mod,
		Manifest:   manifest,
		SubModules: subModules,
		Readme:     readme,
		Releases:   releases,
	}, nil
}

func (c *Client) GetManifest(ctx context.Context, owner, repo, branch string) (*Manifest, error) {
	content, _, _, err := c.client.Repositories.GetContents(ctx, owner, repo, "__manifest__.py", &github.RepositoryContentGetOptions{Ref: branch})
	if err != nil {
		// Try __openerp__.py
		content, _, _, err = c.client.Repositories.GetContents(ctx, owner, repo, "__openerp__.py", &github.RepositoryContentGetOptions{Ref: branch})
		if err != nil {
			return nil, fmt.Errorf("manifest not found: %w", err)
		}
	}

	// Decode base64 content
	decoded, err := base64.StdEncoding.DecodeString(*content.Content)
	if err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}

	manifest, err := parseManifest(string(decoded))
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return manifest, nil
}

func parseManifest(content string) (*Manifest, error) {
	// Simple Python dict parser - in production use a proper Python AST parser
	// This is a simplified version
	manifest := &Manifest{
		Depends: []string{},
		Data:    []string{},
		Demo:    []string{},
		Images:  []string{},
	}

	// Use a simple regex-based extraction for common fields
	reName := regexp.MustCompile(`['"]name['"]\s*:\s*['"]([^'"]+)['"]`)
	if m := reName.FindStringSubmatch(content); len(m) > 1 {
		manifest.Name = m[1]
	}

	reVersion := regexp.MustCompile(`['"]version['"]\s*:\s*['"]([^'"]+)['"]`)
	if m := reVersion.FindStringSubmatch(content); len(m) > 1 {
		manifest.Version = m[1]
	}

	reSummary := regexp.MustCompile(`['"]summary['"]\s*:\s*['"]([^'"]+)['"]`)
	if m := reSummary.FindStringSubmatch(content); len(m) > 1 {
		manifest.Summary = m[1]
	}

	reDesc := regexp.MustCompile(`['"]description['"]\s*:\s*['"]([^'"]+)['"]`)
	if m := reDesc.FindStringSubmatch(content); len(m) > 1 {
		manifest.Description = m[1]
	}

	reAuthor := regexp.MustCompile(`['"]author['"]\s*:\s*['"]([^'"]+)['"]`)
	if m := reAuthor.FindStringSubmatch(content); len(m) > 1 {
		manifest.Author = m[1]
	}

	reLicense := regexp.MustCompile(`['"]license['"]\s*:\s*['"]([^'"]+)['"]`)
	if m := reLicense.FindStringSubmatch(content); len(m) > 1 {
		manifest.License = m[1]
	}

	reCategory := regexp.MustCompile(`['"]category['"]\s*:\s*['"]([^'"]+)['"]`)
	if m := reCategory.FindStringSubmatch(content); len(m) > 1 {
		manifest.Category = m[1]
	}

	// Extract depends list
	reDepends := regexp.MustCompile(`['"]depends['"]\s*:\s*\[([^\]]+)\]`)
	if m := reDepends.FindStringSubmatch(content); len(m) > 1 {
		depsStr := m[1]
		deps := strings.Split(depsStr, ",")
		for _, d := range deps {
			d = strings.TrimSpace(d)
			d = strings.Trim(d, "'\"")
			if d != "" {
				manifest.Depends = append(manifest.Depends, d)
			}
		}
	}

	// Extract data files
	reData := regexp.MustCompile(`['"]data['"]\s*:\s*\[([^\]]+)\]`)
	if m := reData.FindStringSubmatch(content); len(m) > 1 {
		dataStr := m[1]
		data := strings.Split(dataStr, ",")
		for _, d := range data {
			d = strings.TrimSpace(d)
			d = strings.Trim(d, "'\"")
			if d != "" {
				manifest.Data = append(manifest.Data, d)
			}
		}
	}

	// Extract demo files
	reDemo := regexp.MustCompile(`['"]demo['"]\s*:\s*\[([^\]]+)\]`)
	if m := reDemo.FindStringSubmatch(content); len(m) > 1 {
		demoStr := m[1]
		demo := strings.Split(demoStr, ",")
		for _, d := range demo {
			d = strings.TrimSpace(d)
			d = strings.Trim(d, "'\"")
			if d != "" {
				manifest.Demo = append(manifest.Demo, d)
			}
		}
	}

	// Extract images
	reImages := regexp.MustCompile(`['"]images['"]\s*:\s*\[([^\]]+)\]`)
	if m := reImages.FindStringSubmatch(content); len(m) > 1 {
		imgStr := m[1]
		imgs := strings.Split(imgStr, ",")
		for _, img := range imgs {
			img = strings.TrimSpace(img)
			img = strings.Trim(img, "'\"")
			if img != "" {
				manifest.Images = append(manifest.Images, img)
			}
		}
	}

	// Check installable/auto_install
	if strings.Contains(content, "'installable'") {
		if strings.Contains(content, "True") {
			manifest.Installable = true
		}
	}
	if strings.Contains(content, "'auto_install'") {
		if strings.Contains(content, "True") {
			manifest.AutoInstall = true
		}
	}

	return manifest, nil
}

func (c *Client) GetSubModules(ctx context.Context, owner, repo, branch string) ([]SubModule, error) {
	_, directoryContent, _, err := c.client.Repositories.GetContents(ctx, owner, repo, "", &github.RepositoryContentGetOptions{Ref: branch})
	if err != nil {
		return nil, err
	}

	var subModules []SubModule
	for _, content := range directoryContent {
		if content.GetType() == "dir" && !strings.HasPrefix(content.GetName(), ".") && content.GetName() != "tests" && content.GetName() != "docs" {
			// Check if it has __manifest__.py
			subContent, _, _, err := c.client.Repositories.GetContents(ctx, owner, repo, content.GetName()+"/__manifest__.py", &github.RepositoryContentGetOptions{Ref: branch})
			if err == nil {
				decoded, _ := base64.StdEncoding.DecodeString(*subContent.Content)
				manifest, _ := parseManifest(string(decoded))
				subModules = append(subModules, SubModule{
					Name:        content.GetName(),
					Path:        content.GetPath(),
					Description: manifest.Summary,
				})
			}
		}
	}
	return subModules, nil
}

func (c *Client) GetReadme(ctx context.Context, owner, repo string) (string, error) {
	content, _, _, err := c.client.Repositories.GetContents(ctx, owner, repo, "README.md", nil)
	if err != nil {
		// Try README.rst
		content, _, _, err = c.client.Repositories.GetContents(ctx, owner, repo, "README.rst", nil)
		if err != nil {
			return "", err
		}
	}
	decoded, _ := base64.StdEncoding.DecodeString(*content.Content)
	return string(decoded), nil
}

func (c *Client) GetReleases(ctx context.Context, owner, repo string) ([]Release, error) {
	releases, _, err := c.client.Repositories.ListReleases(ctx, owner, repo, &github.ListOptions{PerPage: 10})
	if err != nil {
		return nil, err
	}

	var releasesList []Release
	for _, r := range releases {
		releasesList = append(releasesList, Release{
			TagName:     r.GetTagName(),
			Name:        r.GetName(),
			PublishedAt: r.GetPublishedAt().Format("2006-01-02"),
			Body:        r.GetBody(),
			Prerelease:  r.GetPrerelease(),
			Draft:       r.GetDraft(),
		})
	}
	return releasesList, nil
}

// ============================================
// Dependency Resolution
// ============================================

func (c *Client) ResolveDependencies(ctx context.Context, manifest *Manifest) ([]Dependency, error) {
	var deps []Dependency
	for _, depName := range manifest.Depends {
		// Search for the dependency on GitHub
		query := fmt.Sprintf("%s topic:odoo", depName)
		result, _, err := c.client.Search.Repositories(ctx, query, &github.SearchOptions{
			ListOptions: github.ListOptions{PerPage: 5},
		})
		if err != nil || len(result.Repositories) == 0 {
			deps = append(deps, Dependency{
				Name:       depName,
				Satisfied:  false,
				IsOptional: false,
			})
			continue
		}

		// Find the best match (highest stars)
		var bestRepo *github.Repository
		maxStars := 0
		for _, repo := range result.Repositories {
			if repo.GetStargazersCount() > maxStars {
				maxStars = repo.GetStargazersCount()
				bestRepo = repo
			}
		}

		if bestRepo != nil {
			deps = append(deps, Dependency{
				Name:       depName,
				Repo:       bestRepo.GetFullName(),
				Version:    "latest",
				Satisfied:  false, // Will be checked against instance
				IsOptional: false,
			})
		} else {
			deps = append(deps, Dependency{
				Name:       depName,
				Satisfied:  false,
				IsOptional: false,
			})
		}
	}
	return deps, nil
}

// ============================================
// Module Operations
// ============================================

type ModuleSpec struct {
	Owner       string
	Repo        string
	Branch      string
	TargetDir   string
	Instance    *instance.Instance
	DBName      string
	AutoDeps    bool
	RunTests    bool
}

type InstallResult struct {
	ModuleName   string
	SubModules   []string
	Dependencies []string
	TestResults  []string
}

func (c *Client) InstallModule(ctx context.Context, spec ModuleSpec) (*InstallResult, error) {
	// Clone the repo
	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", spec.Owner, spec.Repo)
	cmd := exec.CommandContext(ctx, "git", "clone", "--branch", spec.Branch, "--depth", "1", repoURL, spec.TargetDir)
	cmd.Dir = filepath.Dir(spec.TargetDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git clone failed: %w\n%s", err, string(out))
	}

	// Parse manifest
	manifest, err := c.GetManifest(ctx, spec.Owner, spec.Repo, spec.Branch)
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}

	// Resolve dependencies
	var depList []Dependency
	var deps []string
	if spec.AutoDeps {
		depList, _ = c.ResolveDependencies(ctx, manifest)
		for _, dep := range depList {
			if !dep.Satisfied {
				// Recursively install dependency
				depOwner, depRepo := splitRepo(dep.Repo)
				depSpec := ModuleSpec{
					Owner:    depOwner,
					Repo:     depRepo,
					Branch:   "master",
					TargetDir: filepath.Join(filepath.Dir(spec.TargetDir), dep.Name),
					Instance: spec.Instance,
					DBName:   spec.DBName,
					AutoDeps: true,
					RunTests: false,
				}
				depResult, err := c.InstallModule(ctx, depSpec)
				if err != nil {
					return nil, fmt.Errorf("install dependency %s: %w", dep.Name, err)
				}
				deps = append(deps, depResult.SubModules...)
			}
		}
	}

	// Copy to instance addons
	srcAddons := filepath.Join(spec.TargetDir)
	dstAddons := filepath.Join(spec.Instance.SourcePath, "custom_addons", manifest.Name)
	if err := copyDir(srcAddons, dstAddons); err != nil {
		return nil, fmt.Errorf("copy addons: %w", err)
	}

	// Install in Odoo
	if spec.DBName != "" {
		// Use service to install module
		// This would be called via the service layer
	}

	return &InstallResult{
		ModuleName:   manifest.Name,
		SubModules:   []string{manifest.Name},
		Dependencies: deps,
	}, nil
}

func splitRepo(full string) (string, string) {
	parts := strings.Split(full, "/")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", full
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}

// ============================================
// Sync/Update
// ============================================

type SyncResult struct {
	ModuleName   string
	Updated      bool
	OldVersion   string
	NewVersion   string
	Changelog    string
	TestResults  []string
}

func (c *Client) SyncModule(ctx context.Context, spec ModuleSpec) (*SyncResult, error) {
	// Get current version from local manifest
	// Compare with remote
	// If different, update and run tests
	return &SyncResult{}, nil
}

// ============================================
// Publish
// ============================================

type PublishOptions struct {
	ModulePath   string
	Owner        string
	Repo         string
	Private      bool
	License      string
	Description  string
	Topics       []string
	CreateActions bool
}

func (c *Client) PublishModule(ctx context.Context, opts PublishOptions) error {
	// Create repository
	repo := &github.Repository{
		Name:        github.String(opts.Repo),
		Description: github.String(opts.Description),
		Private:     github.Bool(opts.Private),
		AutoInit:    github.Bool(true),
		LicenseTemplate: github.String(opts.License),
	}

	_, _, err := c.client.Repositories.Create(ctx, opts.Owner, repo)
	if err != nil {
		// Check if repo already exists
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("create repo: %w", err)
		}
	}

	// Push local code
	// This would use git commands
	return nil
}

// ============================================
// Token Management
// ============================================

func (c *Client) ValidateToken(ctx context.Context) (*github.User, error) {
	user, _, err := c.client.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	return user, nil
}

func (c *Client) GetTokenScopes(ctx context.Context) ([]string, error) {
	// This requires checking the token scopes via the API
	// The GitHub API doesn't directly expose scopes, but we can check via the OAuth app
	return nil, nil
}
