// Package paths resolves every directory ccam touches.
//
// CCAM_HOME overrides everything so tests never touch the real home folder.
// Otherwise the XDG base directory variables are honoured.
package paths

import (
	"os"
	"os/user"
	"path/filepath"
)

type Paths struct {
	Config  string // ~/.config/ccam
	Data    string // ~/.local/share/ccam
	State   string // ~/.local/state/ccam
	Bin     string // ~/.local/bin
	Runtime string // ~/.local/state/ccam/locks: session markers and locks
	Home    string // user home, used by import
}

func FromEnv() Paths {
	if root := os.Getenv("CCAM_HOME"); root != "" {
		// CCAM_USER_HOME lets a separate data folder (the macOS app's) still
		// see the real ~/.claude, for import and its Keychain login.
		home := os.Getenv("CCAM_USER_HOME")
		if home == "" {
			home = filepath.Join(root, "home")
		}
		return Paths{
			Config:  filepath.Join(root, "config"),
			Data:    filepath.Join(root, "data"),
			State:   filepath.Join(root, "state"),
			Bin:     filepath.Join(root, "bin"),
			Runtime: filepath.Join(root, "run"),
			Home:    home,
		}
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		// HOME unset (cron, systemd units): without this every folder would
		// be relative to the working directory.
		if u, err := user.Current(); err == nil {
			home = u.HomeDir
		}
	}
	p := Paths{
		Config: filepath.Join(xdg("XDG_CONFIG_HOME", home, ".config"), "ccam"),
		Data:   filepath.Join(xdg("XDG_DATA_HOME", home, ".local/share"), "ccam"),
		State:  filepath.Join(xdg("XDG_STATE_HOME", home, ".local/state"), "ccam"),
		Bin:    filepath.Join(home, ".local", "bin"),
		Home:   home,
	}
	// Not $XDG_RUNTIME_DIR: shells outside the login session (cron, ssh,
	// su, a service's terminal, tmux started elsewhere) have another one or
	// none, and would not see each other's sessions. Markers check their
	// own process and start time, so they need no memory file system.
	p.Runtime = filepath.Join(p.State, "locks")
	return p
}

// xdg reads an XDG base directory; relative values are invalid by the spec
// and ignored.
func xdg(env, home, def string) string {
	if v := os.Getenv(env); filepath.IsAbs(v) {
		return v
	}
	return filepath.Join(home, def)
}

func (p Paths) ConfigFile() string            { return filepath.Join(p.Config, "config.toml") }
func (p Paths) ProfileDir(name string) string { return filepath.Join(p.Data, "profiles", name) }
func (p Paths) GroupsRoot() string            { return filepath.Join(p.Data, "groups") }
func (p Paths) GroupDir(name string) string   { return filepath.Join(p.GroupsRoot(), name) }
func (p Paths) TrashDir() string              { return filepath.Join(p.Data, "trash") }
