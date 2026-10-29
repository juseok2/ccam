// Package config reads and writes ~/.config/ccam/config.toml.
package config

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/jhyunwoo/ccam/internal/fsutil"
)

const (
	MCPFlag = "flag"
	MCPSync = "sync"
)

// Local means "not shared" for a share item.
const Local = "local"

type Config struct {
	Default     string `toml:"default,omitempty"`
	ClaudeBin   string `toml:"claude_bin,omitempty"`
	MCPStrategy string `toml:"mcp_strategy,omitempty"`
	// AutoMaxUsage and AutoStrategy steer how next --auto, --fallback and
	// pick choose an account (usage.Policy); 0 and "" are the defaults.
	AutoMaxUsage float64 `toml:"auto_max_usage,omitempty"`
	AutoStrategy string  `toml:"auto_strategy,omitempty"`
	// CrossOrg lets next --auto, --fallback and pick choose an account of
	// another organization (or a personal plan for a team one).
	CrossOrg bool `toml:"cross_org,omitempty"`
	// UsageSource is where usage comes from: live (ccam asks the usage
	// endpoint with the account's token, the default) or claude (Claude
	// Code asks, and ccam never handles the token; slower).
	UsageSource string `toml:"usage_source,omitempty"`
	// Relay keeps ccam running beside an interactive Claude Code, so that
	// /ccam:switch can carry the conversation on as another account.
	Relay bool `toml:"relay,omitempty"`
	// AutoSwitch carries an interactive session that stopped at a usage
	// limit on as another account with room by itself (Linux, macOS: it
	// implies Relay there; Windows ignores it).
	AutoSwitch bool `toml:"auto_switch,omitempty"`
	// Theme picks the colors of ccam's screens: auto (from the terminal's
	// background, the default), dark, light, dark-daltonized,
	// light-daltonized, dark-ansi or light-ansi.
	Theme    string    `toml:"theme,omitempty"`
	Groups   []Group   `toml:"group,omitempty"`
	Profiles []Profile `toml:"profile"`
	Folders  []Folder  `toml:"folder,omitempty"`

	// legacy is set when Load converted the old per-account format; the
	// first Save keeps the old file as config.toml.bak.
	legacy bool
	// unknown describes the lines of the file Load did not read (Unknown).
	unknown []string
}

// Themes lists the values of the theme setting besides "auto".
var Themes = []string{"dark", "light", "dark-daltonized", "light-daltonized", "dark-ansi", "light-ansi"}

// Folder makes Account the one a launch without a name uses inside Path
// and every folder below it.
type Folder struct {
	Path    string `toml:"path"`
	Account string `toml:"account"`
}

type Profile struct {
	Name  string `toml:"name"`
	Slot  int    `toml:"slot"`
	Email string `toml:"email,omitempty"`
	Dir   string `toml:"dir,omitempty"` // set for imported folders registered in place
	// Folder is the name of its folder under profiles/ when that is not
	// the account's name: on macOS `ccam rename` leaves the folder where it
	// is, since the Keychain names the login after the folder's path.
	Folder string `toml:"folder,omitempty"`
	// Shortcut is the command in the bin folder that runs this account
	// ("" = none). It links to the ccam binary, which looks argv[0] up here.
	Shortcut string `toml:"shortcut,omitempty"`
	// Share is what the account's groups share, set by Load and Resolve.
	// Change the groups, never this.
	Share Share `toml:"-"`
	// OldShare is the per-account format before groups ([profile.share]);
	// Load converts it into groups.
	OldShare *Share `toml:"share,omitempty"`
	// API makes the account use an API endpoint and key instead of a Claude
	// subscription login (ccam add --api).
	API *API `toml:"api,omitempty"`
	// Login is who the account logged in as, recorded by ccam login.
	Login *Identity `toml:"login,omitempty"`
}

// API is an account that reaches an Anthropic-compatible endpoint with a key.
// The key itself is never stored: KeyFrom says where to read it at launch.
type API struct {
	BaseURL string `toml:"base_url,omitempty"` // ANTHROPIC_BASE_URL; "" is Anthropic's API
	KeyFrom string `toml:"key_from"`           // env:NAME or command:COMMAND
	Auth    string `toml:"auth,omitempty"`     // APIKey (default) or Bearer
	Model   string `toml:"model,omitempty"`    // ANTHROPIC_MODEL
}

// API auth styles: ANTHROPIC_API_KEY (x-api-key) or ANTHROPIC_AUTH_TOKEN
// (Authorization: Bearer, for gateways such as LiteLLM or OpenRouter).
const (
	APIKey = "api-key"
	Bearer = "bearer"
)

// Check reports an API setting that cannot work.
func (a *API) Check() error {
	if kind, value, _ := strings.Cut(a.KeyFrom, ":"); kind != "env" && kind != "command" || strings.TrimSpace(value) == "" {
		return fmt.Errorf("the key source is env:NAME or command:COMMAND, not %q", a.KeyFrom)
	}
	if a.Auth != "" && a.Auth != APIKey && a.Auth != Bearer {
		return fmt.Errorf("auth is %q or %q, not %q", APIKey, Bearer, a.Auth)
	}
	if a.BaseURL != "" && !strings.HasPrefix(a.BaseURL, "https://") && !strings.HasPrefix(a.BaseURL, "http://") {
		return fmt.Errorf("the base URL starts with https:// or http://: %q", a.BaseURL)
	}
	return nil
}

// Label names the endpoint for lists: "API" or "API: <host>".
func (a *API) Label() string {
	host := a.BaseURL
	host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	host, _, _ = strings.Cut(host, "/")
	if host == "" {
		return "API"
	}
	return "API: " + host
}

// Share maps each shareable item to Local or a group name.
type Share struct {
	MCP        string `toml:"mcp,omitempty"`
	Settings   string `toml:"settings,omitempty"`
	Memory     string `toml:"memory,omitempty"`
	Extensions string `toml:"extensions,omitempty"`
	History    string `toml:"history,omitempty"`
}

// Items lists share item keys in display order.
var Items = []string{"mcp", "settings", "memory", "extensions", "history"}

// field returns the setting of item, or nil for an unknown item.
func (s *Share) field(item string) *string {
	switch item {
	case "mcp":
		return &s.MCP
	case "settings":
		return &s.Settings
	case "memory":
		return &s.Memory
	case "extensions":
		return &s.Extensions
	case "history":
		return &s.History
	}
	return nil
}

func (s *Share) Get(item string) string {
	if f := s.field(item); f != nil && *f != "" {
		return *f
	}
	return Local
}

func (s *Share) Set(item, group string) error {
	if group == "" || group == "off" {
		group = Local
	}
	if group == "on" {
		group = "default"
	}
	if group != Local {
		if err := ValidateGroup(group); err != nil {
			return err
		}
	}
	f := s.field(item)
	if f == nil {
		return fmt.Errorf("unknown share item: %q (mcp, settings, memory, extensions, history)", item)
	}
	*f = group
	return nil
}

// Equal compares by meaning: "" and "local" are the same.
func (s Share) Equal(o Share) bool {
	for _, item := range Items {
		if s.Get(item) != o.Get(item) {
			return false
		}
	}
	return true
}

// Summary renders the shared items, e.g. "mcp settings md ext history@team";
// "" when nothing is shared.
func (s Share) Summary() string {
	short := map[string]string{"mcp": "mcp", "settings": "settings", "memory": "md", "extensions": "ext", "history": "history"}
	var parts []string
	for _, item := range Items {
		g := s.Get(item)
		if g == Local {
			continue
		}
		if g != "default" {
			parts = append(parts, short[item]+"@"+g)
		} else {
			parts = append(parts, short[item])
		}
	}
	return strings.Join(parts, " ")
}

// DefaultShare is what `ccam add` uses unless told otherwise.
func DefaultShare() Share {
	return Share{MCP: "default", Settings: "default", Memory: "default", Extensions: "default", History: Local}
}

// Reserved holds subcommand names that cannot be profile names,
// so `ccam <name>` is never ambiguous.
var Reserved = map[string]bool{
	"session": true, "add": true, "list": true, "ls": true, "run": true, "use": true, "next": true,
	"usage": true, "u": true, "share": true, "mcp": true, "login": true, "logout": true,
	"rename": true, "remove": true, "rm": true, "import": true, "migrate": true, "doctor": true,
	"shims": true, "shortcut": true, "completion": true, "help": true, "version": true, "statusline": true,
	"update": true, "uninstall": true, "folder": true, "pick": true, "ext": true, "group": true, "groups": true, "hook": true, "agents": true, "attach": true, "ccusage": true,
}

// isName reports whether s matches ^[a-z0-9][a-z0-9_-]{0,31}$, the rule for
// account and group names. Load checks every name on each launch, so this
// is a loop rather than a regexp.
func isName(s string) bool {
	if len(s) == 0 || len(s) > 32 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || i > 0 && (c == '_' || c == '-')) {
			return false
		}
	}
	return true
}

// IsShortcut reports whether s matches ^[a-z0-9][a-z0-9._-]{0,63}$, the
// rule for shortcut commands, which are file names in the bin folder. A
// loop, as isName.
func IsShortcut(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || i > 0 && (c == '_' || c == '-' || c == '.')) {
			return false
		}
	}
	return true
}

// ValidateGroup checks a share group name; groups are folder names.
func ValidateGroup(group string) error {
	if !isName(group) {
		return fmt.Errorf("invalid group name: %q", group)
	}
	return nil
}

func ValidateName(name string) error {
	if !isName(name) {
		return fmt.Errorf("profile names are 1-32 characters of a-z, 0-9, - and _: %q", name)
	}
	if _, err := strconv.Atoi(name); err == nil {
		return fmt.Errorf("names cannot be all digits (they look like account numbers): %q", name)
	}
	if Reserved[name] {
		return fmt.Errorf("%q is a ccam command, so it cannot be a profile name", name)
	}
	return nil
}

// Load reads the config. A missing file yields an empty config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	// Keys ccam does not read (a setting added below a table, which TOML
	// then counts as the table's) are reported, not dropped: the rest is
	// read, and Save refuses to write a file without them.
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		var missing *toml.StrictMissingError
		if !errors.As(err, &missing) {
			return nil, fmt.Errorf("could not read %s: %w", path, err)
		}
		for _, e := range missing.Errors {
			if k := e.Key(); len(k) == 1 && obsolete[k[0]] {
				continue // dropped by the next save
			}
			c.unknown = append(c.unknown, unknownKey(e))
		}
	}
	// Names and groups become folder names; a hand edit such as "../x"
	// must not point ccam outside its own folders.
	for _, f := range c.Folders {
		if !filepath.IsAbs(f.Path) || !isName(f.Account) {
			return nil, fmt.Errorf("%s: invalid folder entry (path %q, account %q)", path, f.Path, f.Account)
		}
	}
	for i := range c.Profiles {
		p := &c.Profiles[i]
		if !isName(p.Name) {
			return nil, fmt.Errorf("%s: invalid account name %q", path, p.Name)
		}
		if p.Folder != "" && !isName(p.Folder) {
			return nil, fmt.Errorf("%s: account %s has an invalid folder %q", path, p.Name, p.Folder)
		}
		for _, q := range c.Profiles[:i] {
			if p.Dir == "" && q.Dir == "" && cmp.Or(p.Folder, p.Name) == cmp.Or(q.Folder, q.Name) {
				return nil, fmt.Errorf("%s: accounts %s and %s use the same folder %q; give one of them a folder of its own", path, q.Name, p.Name, cmp.Or(p.Folder, p.Name))
			}
		}
		if s := p.Shortcut; s != "" {
			// A file in the bin folder: "claude" or "ccam" there would stand
			// for those.
			if !IsShortcut(s) || s == "claude" || s == "ccam" {
				return nil, fmt.Errorf("%s: account %s has an invalid shortcut %q", path, p.Name, s)
			}
			for _, q := range c.Profiles[:i] {
				if q.Shortcut == s {
					return nil, fmt.Errorf("%s: accounts %s and %s have the same shortcut %q", path, q.Name, p.Name, s)
				}
			}
		}
		if p.API != nil {
			if err := p.API.Check(); err != nil {
				return nil, fmt.Errorf("%s: account %s: %w", path, p.Name, err)
			}
		}
		if p.OldShare == nil {
			continue
		}
		if len(c.Groups) > 0 {
			return nil, fmt.Errorf("%s: account %s has a [profile.share] table, but sharing is set by [[group]] entries now; move it into a group and delete the table", path, p.Name)
		}
		for _, item := range Items {
			if g := p.OldShare.Get(item); g != Local && !isName(g) {
				return nil, fmt.Errorf("%s: account %s has an invalid %s group %q", path, p.Name, item, g)
			}
		}
		p.Share, p.OldShare = *p.OldShare, nil
		c.legacy = true
	}
	if c.legacy {
		c.ConvertShares()
	}
	if err := c.Resolve(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

// obsolete are the top-level keys earlier releases wrote (v0.1 and v0.2)
// and later ones dropped: Load ignores them, and a save leaves them out.
var obsolete = map[string]bool{"version": true, "name_shims": true}

// unknownKey says why the line of e is not read and what to do about it.
func unknownKey(e toml.DecodeError) string {
	line, _ := e.Position()
	key := e.Key()
	if len(key) == 0 {
		return fmt.Sprintf("line %d is not read", line)
	}
	name := key[len(key)-1]
	if len(key) == 1 {
		return fmt.Sprintf("line %d: %s is not a ccam setting (a typo, or one of a newer ccam)", line, name)
	}
	table := "[" + strings.Join(key[:len(key)-1], ".") + "]"
	top := topLevel()
	if list, ok := top[key[0]]; ok && list && len(key) == 2 {
		table = "[" + table + "]"
	}
	if _, ok := top[name]; ok {
		return fmt.Sprintf("line %d: %s is read as part of the %s table above it; settings for all accounts go above the first [[group]], [[profile]] or [[folder]]", line, name, table)
	}
	return fmt.Sprintf("line %d: %s is not a setting of %s", line, name, table)
}

// topLevel maps the keys a config file has outside any table to whether
// they are arrays of tables ([[profile]] …) rather than settings.
func topLevel() map[string]bool {
	out := map[string]bool{}
	t := reflect.TypeFor[Config]()
	for i := range t.NumField() {
		if name, _, _ := strings.Cut(t.Field(i).Tag.Get("toml"), ","); name != "" && name != "-" {
			out[name] = t.Field(i).Type.Kind() == reflect.Slice
		}
	}
	return out
}

// Unknown describes the lines of config.toml that Load did not read, for
// doctor; while there are some, Save refuses to write the file.
func (c *Config) Unknown() []string { return c.unknown }

// Writable reports why Save would not write c to path: lines of the file
// that ccam does not read, which writing the file would delete.
func (c *Config) Writable(path string) error {
	if len(c.unknown) == 0 {
		return nil
	}
	return fmt.Errorf("%s has lines ccam does not read, so it was not saved (that would delete them):\n  %s\nMove or fix them, then run the command again",
		path, strings.Join(c.unknown, "\n  "))
}

func (c *Config) Save(path string) error {
	if err := c.Writable(path); err != nil {
		return err
	}
	want, err := c.resolved()
	if err != nil {
		return err
	}
	for _, p := range c.Profiles {
		if !want[p.Name].Equal(p.Share) {
			return fmt.Errorf("internal error: the sharing of account %s does not match its groups", p.Name)
		}
	}
	data, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	if c.legacy {
		// The first save in the group format keeps the old file.
		if old, err := os.ReadFile(path); err == nil && !fsutil.Exists(path+".bak") {
			if err := fsutil.AtomicWrite(path+".bak", old, 0o600); err != nil {
				return err
			}
		}
	}
	if err := fsutil.AtomicWrite(path, data, 0o600); err != nil {
		return err
	}
	c.legacy = false
	return nil
}

// Find resolves a profile by name or slot number.
func (c *Config) Find(sel string) (*Profile, bool) {
	if n, err := strconv.Atoi(sel); err == nil {
		for i := range c.Profiles {
			if c.Profiles[i].Slot == n {
				return &c.Profiles[i], true
			}
		}
		return nil, false
	}
	for i := range c.Profiles {
		if c.Profiles[i].Name == sel {
			return &c.Profiles[i], true
		}
	}
	return nil, false
}

// MustFind is Find with a user-facing error.
func (c *Config) MustFind(sel string) (*Profile, error) {
	if p, ok := c.Find(sel); ok {
		return p, nil
	}
	if len(c.Profiles) == 0 {
		return nil, fmt.Errorf("no accounts yet. Add one first with `ccam add <name>`")
	}
	return nil, fmt.Errorf("no account %q (see `ccam list`)", sel)
}

// DefaultProfile returns the default profile, or the lowest slot if none is set.
func (c *Config) DefaultProfile() (*Profile, error) {
	if c.Default != "" {
		return c.MustFind(c.Default)
	}
	if len(c.Profiles) == 0 {
		return nil, fmt.Errorf("no accounts yet. Add one first with `ccam add <name>`")
	}
	best := &c.Profiles[0]
	for i := range c.Profiles {
		if c.Profiles[i].Slot < best.Slot {
			best = &c.Profiles[i]
		}
	}
	return best, nil
}

// FindShortcut returns the profile whose shortcut command is name.
func (c *Config) FindShortcut(name string) (*Profile, bool) {
	if name == "" {
		return nil, false
	}
	for i := range c.Profiles {
		if c.Profiles[i].Shortcut == name {
			return &c.Profiles[i], true
		}
	}
	return nil, false
}

// NextSlot returns the lowest unused slot number, starting at 1.
func (c *Config) NextSlot() int {
	used := map[int]bool{}
	for _, p := range c.Profiles {
		used[p.Slot] = true
	}
	n := 1
	for used[n] {
		n++
	}
	return n
}

// Remove drops a profile, its folders, its group memberships, and the
// default if it pointed at it.
func (c *Config) Remove(name string) {
	c.Profiles = slices.DeleteFunc(c.Profiles, func(p Profile) bool { return p.Name == name })
	c.Folders = slices.DeleteFunc(c.Folders, func(f Folder) bool { return f.Account == name })
	for i := range c.Groups {
		c.Groups[i].Members = slices.DeleteFunc(c.Groups[i].Members, func(m string) bool { return m == name })
	}
	if c.Default == name {
		c.Default = ""
	}
}

// FolderFor returns the folder entry that contains dir; when entries nest,
// the deepest one wins. It compares strings only (no file access), as it
// runs on every launch without an account name.
func (c *Config) FolderFor(dir string) (*Folder, bool) {
	var best *Folder
	for i := range c.Folders {
		f := &c.Folders[i]
		if within(f.Path, dir) && (best == nil || len(f.Path) > len(best.Path)) {
			best = f
		}
	}
	return best, best != nil
}

// SetFolder makes account the one used in path, replacing any account
// path had. path must be absolute.
func (c *Config) SetFolder(path, account string) {
	path = filepath.Clean(path)
	for i := range c.Folders {
		if SamePath(c.Folders[i].Path, path) {
			c.Folders[i].Account = account
			return
		}
	}
	c.Folders = append(c.Folders, Folder{Path: path, Account: account})
}

// UnsetFolder removes the entry for exactly path and reports whether there was one.
func (c *Config) UnsetFolder(path string) bool {
	path = filepath.Clean(path)
	n := len(c.Folders)
	c.Folders = slices.DeleteFunc(c.Folders, func(f Folder) bool { return SamePath(f.Path, path) })
	return len(c.Folders) != n
}

// within reports whether dir is base or inside it. Both are absolute.
func within(base, dir string) bool {
	base, dir = filepath.Clean(base), filepath.Clean(dir)
	if len(dir) < len(base) || !SamePath(dir[:len(base)], base) {
		return false
	}
	return len(dir) == len(base) || os.IsPathSeparator(dir[len(base)]) || os.IsPathSeparator(base[len(base)-1])
}

// foldCase is set where file systems ignore letter case by default:
// Windows, and macOS (APFS, HFS+), where `cd ~/code/app` reaches ~/Code/App
// and the working directory keeps the case typed.
var foldCase = runtime.GOOS == "windows" || runtime.GOOS == "darwin"

// SamePath compares two folder paths; Windows and macOS paths compare
// without case.
func SamePath(a, b string) bool {
	if foldCase {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func (c *Config) Strategy() string {
	if c.MCPStrategy == MCPSync {
		return MCPSync
	}
	return MCPFlag
}
