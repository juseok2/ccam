package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCCAMHomeOverridesEverything(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CCAM_HOME", root)
	t.Setenv("CCAM_USER_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "/nope")
	p := FromEnv()
	for _, dir := range []string{p.Config, p.Data, p.State, p.Bin, p.Runtime, p.Home} {
		if !strings.HasPrefix(dir, root+string(filepath.Separator)) {
			t.Errorf("%s escapes CCAM_HOME %s", dir, root)
		}
	}
}

func TestCCAMUserHomeKeepsDataInCCAMHome(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("CCAM_HOME", root)
	t.Setenv("CCAM_USER_HOME", home)
	p := FromEnv()
	if p.Home != home {
		t.Errorf("Home = %s, want %s", p.Home, home)
	}
	for _, dir := range []string{p.Config, p.Data, p.State, p.Bin, p.Runtime} {
		if !strings.HasPrefix(dir, root+string(filepath.Separator)) {
			t.Errorf("%s escapes CCAM_HOME %s", dir, root)
		}
	}
}

func TestXDG(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix paths; see TestWindowsHome")
	}
	t.Setenv("CCAM_HOME", "")
	t.Setenv("HOME", "/h")
	t.Setenv("XDG_CONFIG_HOME", "/cfg")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1")
	p := FromEnv()
	want := Paths{Config: "/cfg/ccam", Data: "/h/.local/share/ccam", State: "/h/.local/state/ccam", Bin: "/h/.local/bin", Runtime: "/h/.local/state/ccam/locks", Home: "/h"}
	if p != want {
		t.Fatalf("got %+v want %+v", p, want)
	}
}

// Session markers are where every shell of the user finds them, whatever
// its XDG_RUNTIME_DIR (cron and ssh have none, su and WSLg another one).
func TestRuntimeIgnoresXDGRuntimeDir(t *testing.T) {
	t.Setenv("CCAM_HOME", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("XDG_STATE_HOME", "")
	var seen []string
	for _, rt := range []string{"", "/run/user/1", "/mnt/wslg/runtime-dir", "rel"} {
		t.Setenv("XDG_RUNTIME_DIR", rt)
		p := FromEnv()
		if p.Runtime != filepath.Join(p.State, "locks") {
			t.Fatalf("XDG_RUNTIME_DIR=%q: Runtime %s", rt, p.Runtime)
		}
		seen = append(seen, p.Runtime)
	}
	for _, r := range seen {
		if r != seen[0] {
			t.Fatalf("Runtime differs between shells: %v", seen)
		}
	}
}

func TestRelativeXDGIsIgnored(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix paths; see TestWindowsHome")
	}
	t.Setenv("CCAM_HOME", "")
	t.Setenv("HOME", "/h")
	t.Setenv("XDG_DATA_HOME", "rel/data")
	t.Setenv("XDG_RUNTIME_DIR", "rel/run")
	p := FromEnv()
	if p.Data != "/h/.local/share/ccam" || p.Runtime != p.State+"/locks" {
		t.Fatalf("got %+v", p)
	}
}

func TestUnsetHomeStaysAbsolute(t *testing.T) {
	t.Setenv("CCAM_HOME", "")
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if p := FromEnv(); !filepath.IsAbs(p.Config) || !filepath.IsAbs(p.Bin) {
		t.Fatalf("relative paths without HOME: %+v", p)
	}
}

// Windows: the same layout under %USERPROFILE%, where Claude Code keeps
// .claude and .local\bin too.
func TestWindowsHome(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	t.Setenv("CCAM_HOME", "")
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	p := FromEnv()
	want := Paths{
		Config: filepath.Join(home, ".config", "ccam"), Data: filepath.Join(home, ".local", "share", "ccam"),
		State: filepath.Join(home, ".local", "state", "ccam"), Bin: filepath.Join(home, ".local", "bin"),
		Runtime: filepath.Join(home, ".local", "state", "ccam", "locks"), Home: home,
	}
	if p != want {
		t.Fatalf("got %+v want %+v", p, want)
	}
}
