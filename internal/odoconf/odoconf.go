package odoconf

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Kind classifies a conf line.
type Kind int

const (
	KindSection      Kind = iota // [section]
	KindKey                      // key = value (active)
	KindCommentedKey             // # key = value (inactive, value kept)
	KindComment                  // # or ; free comment
	KindBlank                    // empty line
	KindContinuation             // indented line attached to the previous key
	KindUnknown                  // anything else — preserved verbatim
)

func (k Kind) String() string {
	switch k {
	case KindSection:
		return "section"
	case KindKey:
		return "key"
	case KindCommentedKey:
		return "commented-key"
	case KindComment:
		return "comment"
	case KindBlank:
		return "blank"
	case KindContinuation:
		return "continuation"
	default:
		return "unknown"
	}
}

// Line is one conf line with its parsed meaning. Raw is byte-exact apart
// from a trailing newline; unknown lines are preserved verbatim.
type Line struct {
	Kind    Kind
	Raw     string
	Key     string // KindKey / KindCommentedKey
	Value   string // raw value text
	Section string // section the key belongs to ("" = top of file)
}

// Entry is a conf option as listed to the user.
type Entry struct {
	Name   string
	Active bool
	Value  string // stored value of the last occurrence (even when commented)
}

// optionsSection is the section the engine reads and edits. Odoo confs are
// either bare top-of-file keys or keys under [options].
const optionsSection = "options"

// OdooConf is a lossless, line-preserving odoo.conf representation.
// Every edit keeps comments, blank lines, section headers, key ordering
// and unknown lines intact.
type OdooConf struct {
	path   string
	lines  []Line
	dirty  bool
	backed bool
}

// Load parses an odoo.conf file. The file is not rewritten on Save() unless
// an option actually changed.
func Load(path string) (*OdooConf, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &OdooConf{path: path}
	parts := strings.Split(string(data), "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		// the split of a file ending with "\n" produces a phantom empty
		// element; dropping exactly one keeps real trailing blank lines
		parts = parts[:len(parts)-1]
	}
	curSection := ""
	prev := -1
	for _, raw := range parts {
		raw = strings.TrimSuffix(raw, "\r")
		line := Line{Raw: raw, Section: curSection}
		trimmed := strings.TrimSpace(raw)
		switch {
		case trimmed == "":
			line.Kind = KindBlank
		case strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"):
			line.Kind = KindSection
			curSection = trimmed[1 : len(trimmed)-1]
			line.Section = curSection
		case strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";"):
			line.Kind = KindComment
			rest := strings.TrimSpace(trimmed[1:])
			if k, v, ok := splitKV(rest); ok {
				line.Kind = KindCommentedKey
				line.Key = k
				line.Value = v
			}
		case strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "\t"):
			// configparser continuation line: belongs to the previous key
			if prev >= 0 && c.lines[prev].Kind == KindKey {
				line.Kind = KindContinuation
				c.lines[prev].Value += "\n" + strings.TrimSpace(raw)
			} else {
				line.Kind = KindUnknown
			}
		default:
			if k, v, ok := splitKV(trimmed); ok {
				line.Kind = KindKey
				line.Key = k
				line.Value = v
			} else {
				line.Kind = KindUnknown
			}
		}
		prev = len(c.lines)
		c.lines = append(c.lines, line)
	}
	return c, nil
}

// splitKV splits "key = value" (configparser: value may contain =).
func splitKV(s string) (key, value string, ok bool) {
	idx := strings.Index(s, "=")
	if idx <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(s[:idx])
	if key == "" || strings.ContainsAny(key, " \t#;") {
		return "", "", false
	}
	return key, strings.TrimSpace(s[idx+1:]), true
}

// isOptions reports whether a key line lives in the options section.
func isOptions(l Line) bool {
	return l.Section == "" || l.Section == optionsSection
}

// findKey returns the index of the last line matching kind + key in the
// options section (-1 when absent).
func (c *OdooConf) findKey(key string, kinds ...Kind) int {
	idx := -1
	for i := range c.lines {
		l := c.lines[i]
		if !isOptions(l) || l.Key != key {
			continue
		}
		for _, k := range kinds {
			if l.Kind == k {
				idx = i
			}
		}
	}
	return idx
}

// Get reads a key from the options section. The last active occurrence
// wins (configparser semantics).
func (c *OdooConf) Get(key string) (string, bool) {
	i := c.findKey(key, KindKey)
	if i < 0 {
		return "", false
	}
	return c.lines[i].Value, true
}

// Set writes a key into the options section: an active occurrence is
// updated in place, a commented one is re-enabled, otherwise the key is
// appended after the last options line. Existing comments are kept.
func (c *OdooConf) Set(key, value string) {
	if i := c.findKey(key, KindKey); i >= 0 {
		c.lines[i].Value = value
		c.lines[i].Raw = key + " = " + value
		c.dirty = true
		return
	}
	if i := c.findKey(key, KindCommentedKey); i >= 0 {
		c.lines[i].Kind = KindKey
		c.lines[i].Value = value
		c.lines[i].Raw = key + " = " + value
		c.dirty = true
		return
	}
	c.lines = append(c.lines, Line{
		Kind: KindKey, Key: key, Value: value,
		Section: optionsSection, Raw: key + " = " + value,
	})
	c.dirty = true
}

// Unset deletes every occurrence of the key (active or commented) together
// with the comment/blank block attached directly above each occurrence.
func (c *OdooConf) Unset(key string) {
	removed := false
	var out []Line
	for i := 0; i < len(c.lines); i++ {
		l := c.lines[i]
		if isOptions(l) && l.Key == key && (l.Kind == KindKey || l.Kind == KindCommentedKey) {
			// Remove attached comment/blank block above
			j := len(out) - 1
			for j >= 0 && (out[j].Kind == KindComment || out[j].Kind == KindBlank) {
				j--
			}
			out = out[:j+1]
			removed = true
		} else {
			out = append(out, l)
		}
	}
	if removed {
		c.dirty = true
	}
	c.lines = out
}

// Comment disables a key in place ("# key = value"), keeping its value,
// position and comments. An absent key is appended as a commented line.
func (c *OdooConf) Comment(key string) error {
	if i := c.findKey(key, KindKey); i >= 0 {
		c.lines[i].Kind = KindCommentedKey
		c.lines[i].Raw = "# " + c.lines[i].Raw
		c.dirty = true
		return nil
	}
	if c.findKey(key, KindCommentedKey) >= 0 {
		return nil // already commented
	}
	c.lines = append(c.lines, Line{
		Kind: KindCommentedKey, Key: key, Section: optionsSection, Raw: "# " + key + " = ",
	})
	c.dirty = true
	return nil
}

// Uncomment re-enables a commented key in place. An absent key is set
// (with an empty value) like Set.
func (c *OdooConf) Uncomment(key string) error {
	if i := c.findKey(key, KindCommentedKey); i >= 0 {
		raw := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(c.lines[i].Raw), "#"), ";"))
		if k, v, ok := splitKV(raw); ok && k == key {
			c.lines[i].Kind = KindKey
			c.lines[i].Value = v
			c.lines[i].Raw = raw
			c.dirty = true
			return nil
		}
		return fmt.Errorf("cannot uncomment key %q: line is not a key=value pair", key)
	}
	if c.findKey(key, KindKey) >= 0 {
		return nil // already active
	}
	c.Set(key, "")
	return nil
}

// Keys lists the active options keys in file order (unique).
func (c *OdooConf) Keys() []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range c.lines {
		if isOptions(l) && l.Kind == KindKey && !seen[l.Key] {
			seen[l.Key] = true
			out = append(out, l.Key)
		}
	}
	return out
}

// AllKeys lists every option (active and commented) in file order.
// Active reports whether the last occurrence is enabled; Value is the
// stored value of that occurrence.
func (c *OdooConf) AllKeys() []Entry {
	var out []Entry
	seen := map[string]bool{}
	for _, l := range c.lines {
		if !isOptions(l) || (l.Kind != KindKey && l.Kind != KindCommentedKey) {
			continue
		}
		if seen[l.Key] {
			for i := range out {
				if out[i].Name == l.Key {
					out[i].Active = l.Kind == KindKey
					out[i].Value = l.Value
				}
			}
			continue
		}
		seen[l.Key] = true
		out = append(out, Entry{Name: l.Key, Active: l.Kind == KindKey, Value: l.Value})
	}
	return out
}

// Lines returns the raw line model (for viewers).
func (c *OdooConf) Lines() []Line {
	return append([]Line(nil), c.lines...)
}

// Dirty reports whether the document was mutated since Load/New.
func (c *OdooConf) Dirty() bool { return c.dirty }

// Path returns the bound path ("" for a New document).
func (c *OdooConf) Path() string { return c.path }

// Backup copies the current file to <conf>.bak (once per session).
func (c *OdooConf) Backup() (string, error) {
	if c.backed || c.path == "" {
		return c.path + ".bak", nil
	}
	data, err := os.ReadFile(c.path)
	if err != nil {
		return "", err
	}
	bak := c.path + ".bak"
	if err := os.WriteFile(bak, data, 0o644); err != nil {
		return "", err
	}
	c.backed = true
	return bak, nil
}

// Write persists the document to path with an atomic rename.
func (c *OdooConf) Write(path string) error {
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	for _, l := range c.lines {
		b.WriteString(l.Raw)
		b.WriteString("\n")
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Save persists to the bound path (after a .bak backup). It never rewrites
// the file unless an option actually changed.
func (c *OdooConf) Save() error {
	if c.path == "" {
		return errors.New("no path bound")
	}
	if !c.dirty {
		return nil
	}
	if _, err := c.Backup(); err != nil {
		return err
	}
	if err := c.Write(c.path); err != nil {
		return err
	}
	c.dirty = false
	return nil
}

// New creates an in-memory conf seeded with Odoo runtime basics, in a
// canonical, deterministic key order.
func New() *OdooConf {
	defaults := [][2]string{
		{"addons_path", ""},
		{"data_dir", ""},
		{"db_host", "localhost"},
		{"db_port", "5432"},
		{"db_user", "odoo"},
		{"db_password", "False"},
		{"http_port", "8069"},
		{"list_db", "True"},
		{"logfile", ""},
		{"log_level", "info"},
		{"xmlrpc_interface", "127.0.0.1"},
		{"web_interface", "127.0.0.1"},
		{"workers", "2"},
		{"max_cron_threads", "1"},
		{"longpolling_port", "8072"},
	}
	c := &OdooConf{lines: []Line{{Kind: KindSection, Raw: "[options]", Section: optionsSection}}}
	for _, kv := range defaults {
		c.lines = append(c.lines, Line{
			Kind: KindKey, Key: kv[0], Value: kv[1],
			Section: optionsSection, Raw: kv[0] + " = " + kv[1],
		})
	}
	c.dirty = false
	return c
}

func dirOf(path string) string {
	return filepath.Dir(path)
}

// AddonsPath returns the addons_path entries in order (order = module
// resolution priority: the first match wins). A missing or empty key
// yields an empty list.
func (c *OdooConf) AddonsPath() ([]string, error) {
	v, ok := c.Get("addons_path")
	if !ok {
		return nil, nil
	}
	return splitPaths(v)
}

// splitPaths parses a comma-separated addons_path value.
func splitPaths(v string) ([]string, error) {
	var out []string
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, ",") {
			return nil, fmt.Errorf("path %q contains a comma, which odoo.conf cannot express in addons_path", p)
		}
		out = append(out, p)
	}
	return out, nil
}

// SetAddonsPath replaces the whole addons_path list.
func (c *OdooConf) SetAddonsPath(paths []string) error {
	c.Set("addons_path", strings.Join(paths, ","))
	return nil
}

// AddonsAdd inserts a path into the list. at < 0 appends at the end.
func (c *OdooConf) AddonsAdd(path string, at int) error {
	cur, err := c.AddonsPath()
	if err != nil {
		return err
	}
	for _, p := range cur {
		if p == path {
			return fmt.Errorf("path %q is already in addons_path", path)
		}
	}
	if at < 0 {
		at = len(cur) // append
	}
	at = clampIndex(at, len(cur))
	cur = append(cur, "")
	copy(cur[at+1:], cur[at:])
	cur[at] = path
	return c.SetAddonsPath(cur)
}

// AddonsRemove deletes a path from the list.
func (c *OdooConf) AddonsRemove(path string) error {
	cur, err := c.AddonsPath()
	if err != nil {
		return err
	}
	for i, p := range cur {
		if p == path {
			cur = append(cur[:i], cur[i+1:]...)
			return c.SetAddonsPath(cur)
		}
	}
	return fmt.Errorf("path %q is not in addons_path", path)
}

// AddonsMove reorders the list (0-based indices; both clamped to bounds).
func (c *OdooConf) AddonsMove(from, to int) error {
	cur, err := c.AddonsPath()
	if err != nil {
		return err
	}
	n := len(cur)
	if n == 0 {
		return fmt.Errorf("addons_path is empty — nothing to move")
	}
	from = clampIndex(from, n-1)
	to = clampIndex(to, n-1)
	if from == to {
		return nil
	}
	p := cur[from]
	// Copy to avoid mutating original backing array via append
	rest := make([]string, 0, len(cur)-1)
	rest = append(rest, cur[:from]...)
	rest = append(rest, cur[from+1:]...)
	// Insert at position
	newPaths := make([]string, 0, len(rest)+1)
	newPaths = append(newPaths, rest[:to]...)
	newPaths = append(newPaths, p)
	newPaths = append(newPaths, rest[to:]...)
	return c.SetAddonsPath(newPaths)
}

func clampIndex(i, n int) int {
	if i < 0 {
		return 0
	}
	if i > n {
		return n
	}
	return i
}

// RawAddonsEntry is one path with its enabled flag (active vs commented).
type RawAddonsEntry struct {
	Path    string
	Enabled bool
}

// RawAddonsPathEntries returns all addons_path entries in file order,
// including disabled (commented) ones. Each comma-separated value is exploded.
func (c *OdooConf) RawAddonsPathEntries() []RawAddonsEntry {
	var out []RawAddonsEntry
	for _, l := range c.lines {
		if !isOptions(l) || l.Key != "addons_path" {
			continue
		}
		if l.Kind != KindKey && l.Kind != KindCommentedKey {
			continue
		}
		enabled := l.Kind == KindKey
		paths, _ := splitPaths(l.Value)
		for _, p := range paths {
			out = append(out, RawAddonsEntry{Path: p, Enabled: enabled})
		}
	}
	return out
}

// RemoveRawAddonsEntry removes a path whether enabled or disabled (commented).
func (c *OdooConf) RemoveRawAddonsEntry(path string) {
	// remove from active
	if cur, _ := c.AddonsPath(); cur != nil {
		for i, p := range cur {
			if p == path {
				cur = append(cur[:i], cur[i+1:]...)
				_ = c.SetAddonsPath(cur)
				break
			}
		}
	}
	// remove from commented line — iterate backwards to avoid index shift on mutation
	for i := len(c.lines) - 1; i >= 0; i-- {
		l := c.lines[i]
		if !isOptions(l) || l.Key != "addons_path" || l.Kind != KindCommentedKey {
			continue
		}
		paths, _ := splitPaths(l.Value)
		filtered := []string{}
		changed := false
		for _, p := range paths {
			if p == path {
				changed = true
				continue
			}
			filtered = append(filtered, p)
		}
		if changed {
			if len(filtered) == 0 {
				// remove the commented line entirely
				c.lines = append(c.lines[:i], c.lines[i+1:]...)
			} else {
				c.lines[i].Value = strings.Join(filtered, ",")
				c.lines[i].Raw = "; addons_path = " + c.lines[i].Value
			}
			c.dirty = true
			break
		}
	}
}

// EnableAddonsPath moves a disabled path back to active list (append by default).
func (c *OdooConf) EnableAddonsPath(path string) {
	// find commented entry containing path
	for i, l := range c.lines {
		if !isOptions(l) || l.Key != "addons_path" || l.Kind != KindCommentedKey {
			continue
		}
		paths, _ := splitPaths(l.Value)
		found := false
		remaining := []string{}
		for _, p := range paths {
			if p == path {
				found = true
			} else {
				remaining = append(remaining, p)
			}
		}
		if found {
			if len(remaining) == 0 {
				c.lines = append(c.lines[:i], c.lines[i+1:]...)
			} else {
				c.lines[i].Value = strings.Join(remaining, ",")
				c.lines[i].Raw = "; addons_path = " + c.lines[i].Value
			}
			c.dirty = true
			// add to active
			cur, _ := c.AddonsPath()
			cur = append(cur, path)
			_ = c.SetAddonsPath(cur)
			return
		}
	}
}

// DisableAddonsPath moves an active path to disabled (commented) storage.
func (c *OdooConf) DisableAddonsPath(path string) {
	cur, _ := c.AddonsPath()
	idx := -1
	for i, p := range cur {
		if p == path {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	cur = append(cur[:idx], cur[idx+1:]...)
	_ = c.SetAddonsPath(cur)
	// append to commented line or create new
	for i, l := range c.lines {
		if isOptions(l) && l.Key == "addons_path" && l.Kind == KindCommentedKey {
			paths, _ := splitPaths(l.Value)
			paths = append(paths, path)
			c.lines[i].Value = strings.Join(paths, ",")
			c.lines[i].Raw = "; addons_path = " + c.lines[i].Value
			c.dirty = true
			return
		}
	}
	c.lines = append(c.lines, Line{Kind: KindCommentedKey, Key: "addons_path", Value: path, Section: optionsSection, Raw: "; addons_path = " + path})
	c.dirty = true
}
