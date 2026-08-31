package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ahmed/odoonoir/internal/odoconf"
)

// EnterpriseStatus reports detection result.
type EnterpriseStatus struct {
	IsEnterprise bool     `json:"isEnterprise"`
	Path         string   `json:"path"`
	Repo         string   `json:"repo"`
	Branch       string   `json:"branch"`
	AddonsPath   []string `json:"addonsPath"`
	Warning      string   `json:"warning,omitempty"`
}

// DetectEnterprise checks addons_path for enterprise substring or stored fields.
func (s *Service) DetectEnterprise(name string) (*EnterpriseStatus, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	conf, _ := odoconf.Load(p.Conf)
	var addons []string
	if conf != nil {
		if v, _ := conf.AddonsPath(); v != nil {
			addons = v
		}
	}
	if len(addons) == 0 {
		addons = inst.AddonsPaths
	}
	isEnt := false
	var entPath string
	for _, ap := range addons {
		if strings.Contains(strings.ToLower(ap), "enterprise") {
			isEnt = true
			entPath = ap
			break
		}
	}
	if inst.EnterprisePath != "" {
		isEnt = true
		entPath = inst.EnterprisePath
	}
	st := &EnterpriseStatus{
		IsEnterprise: isEnt,
		Path:         entPath,
		Repo:         inst.EnterpriseRepo,
		Branch:       inst.EnterpriseBranch,
		AddonsPath:   addons,
	}
	if isEnt && entPath != "" {
		if _, err := os.Stat(entPath); err != nil {
			st.Warning = fmt.Sprintf("enterprise path %q not found on disk", entPath)
		}
	}
	return st, nil
}

// LoadEnterpriseOptions drives LoadEnterprise.
type LoadEnterpriseOptions struct {
	Repo     string `json:"repo"`     // owner/repo e.g. odoo/enterprise
	Branch   string `json:"branch"`   // e.g. 16.0
	Position string `json:"position"` // default | last
}

// LoadEnterprise clones enterprise repo at branch and wires addons_path.
func (s *Service) LoadEnterprise(ctx context.Context, name string, opts LoadEnterpriseOptions, emit Sink) (*EnterpriseStatus, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	if opts.Repo == "" {
		opts.Repo = "odoo/enterprise"
	}
	parts := strings.Split(opts.Repo, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("repo must be owner/repo, got %q", opts.Repo)
	}
	branch := opts.Branch
	if branch == "" {
		branch = inst.Branch
		if branch == "" {
			branch = inst.Version + ".0"
			if branch == ".0" {
				branch = "16.0"
			}
		}
	}
	position := opts.Position
	if position == "" {
		position = "default"
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	// destination: <root>/enterprise or <root>/src/enterprise
	dest := filepath.Join(s.rootFor(inst), inst.Name, "enterprise")
	// prefer sibling to src/odoo -> src/enterprise
	alt := filepath.Join(filepath.Dir(p.Source), "enterprise")
	if _, err := os.Stat(filepath.Join(p.Source, ".git")); err == nil {
		// community source exists, use sibling
		dest = alt
	}
	emit(Event{Kind: StepStart, Instance: name, Step: "enterprise clone " + opts.Repo + "#" + branch})
	// if dest exists and has .git, fetch/checkout else clone
	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		// fetch + checkout
		if err := runGit(ctx, dest, "fetch", "origin", branch); err != nil {
			// try fetch all
			_ = runGit(ctx, dest, "fetch", "origin")
		}
		_ = runGit(ctx, dest, "checkout", branch)
		_ = runGit(ctx, dest, "pull", "origin", branch)
		emit(Event{Kind: LogLine, Instance: name, Message: "enterprise updated at " + dest})
	} else {
		_ = os.RemoveAll(dest)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, err
		}
		// build authenticated URL if token available and repo is private
		url := fmt.Sprintf("https://github.com/%s.git", opts.Repo)
		if s.github != nil {
			// use token from FileTokenStore directly (Client token is unexported)
			if store, err := newFileTokenStore(); err == nil {
				if tok, _ := store.Get(); tok != "" {
					url = fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", tok, opts.Repo)
				}
			}
		}
		args := []string{"clone", "--depth", "1", "-b", branch, url, dest}
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = filepath.Dir(dest)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("git clone %s#%s: %w\n%s", opts.Repo, branch, err, strings.TrimSpace(string(out)))
		}
		emit(Event{Kind: LogLine, Instance: name, Message: "enterprise cloned to " + dest})
	}
	// wire addons_path
	conf, err := odoconf.Load(p.Conf)
	if err != nil {
		return nil, fmt.Errorf("load odoo.conf: %w", err)
	}
	cur, _ := conf.AddonsPath()
	// remove previous enterprise path if any
	filtered := []string{}
	for _, ap := range cur {
		if ap == inst.EnterprisePath {
			continue
		}
		// also remove if contains old dest
		if ap == dest {
			continue
		}
		filtered = append(filtered, ap)
	}
	var newPaths []string
	if position == "last" {
		newPaths = append(filtered, dest)
	} else {
		// default: enterprise first (highest priority, before odoo/addons)
		newPaths = append([]string{dest}, filtered...)
	}
	if err := conf.SetAddonsPath(newPaths); err != nil {
		return nil, err
	}
	if err := conf.Save(); err != nil {
		// try Write fallback
		if err2 := conf.Write(p.Conf); err2 != nil {
			return nil, err2
		}
	}
	// persist instance
	inst.EnterprisePath = dest
	inst.EnterpriseRepo = opts.Repo
	inst.EnterpriseBranch = branch
	if err := s.reg.Put(inst); err != nil {
		return nil, err
	}
	emit(Event{Kind: StepDone, Instance: name, Step: "enterprise configured at " + dest + " position " + position})
	return s.DetectEnterprise(name)
}

func runGit(ctx context.Context, dir, sub string, args ...string) error {
	all := append([]string{sub}, args...)
	cmd := exec.CommandContext(ctx, "git", all...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w\n%s", strings.Join(all, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// newFileTokenStore returns a FileTokenStore for token retrieval.
func newFileTokenStore() (interface{ Get() (string, error) }, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".config", "odoonoir", "github_token.json")
	// minimal store reading directly
	return &fileTokenStore{path: path}, nil
}

type fileTokenStore struct{ path string }

func (s *fileTokenStore) Get() (string, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	// crude json parse for access_token
	str := string(data)
	idx := strings.Index(str, "access_token")
	if idx < 0 {
		return "", nil
	}
	// find first quote after colon
	rest := str[idx:]
	first := strings.Index(rest, "\"")
	if first < 0 {
		return "", nil
	}
	rest = rest[first+1:]
	second := strings.Index(rest, "\"")
	if second < 0 {
		return "", nil
	}
	// need to find value quotes: look for colon then quote
	// simpler: extract between quotes after access_token
	parts := strings.Split(str, "\"")
	for i, p := range parts {
		if p == "access_token" && i+2 < len(parts) {
			return parts[i+2], nil
		}
	}
	return "", nil
}

// UnloadEnterprise removes enterprise from addons_path (keeps clone on disk).
func (s *Service) UnloadEnterprise(name string) (*EnterpriseStatus, error) {
	inst, err := s.reg.Get(name)
	if err != nil {
		return nil, err
	}
	p := inst.ResolvePaths(s.rootFor(inst))
	conf, err := odoconf.Load(p.Conf)
	if err != nil {
		return nil, err
	}
	cur, _ := conf.AddonsPath()
	filtered := []string{}
	for _, ap := range cur {
		if ap == inst.EnterprisePath {
			continue
		}
		if strings.Contains(strings.ToLower(ap), "enterprise") && ap == inst.EnterprisePath {
			continue
		}
		filtered = append(filtered, ap)
	}
	if len(filtered) == len(cur) {
		// also try substring match if EnterprisePath empty
		tmp := []string{}
		for _, ap := range cur {
			if strings.Contains(strings.ToLower(ap), "enterprise") {
				continue
			}
			tmp = append(tmp, ap)
		}
		filtered = tmp
	}
	_ = conf.SetAddonsPath(filtered)
	_ = conf.Save()
	inst.EnterprisePath = ""
	// keep repo/branch for history
	_ = s.reg.Put(inst)
	return s.DetectEnterprise(name)
}
