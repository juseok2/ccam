// Package cli holds the cobra commands. It is only reached for real
// subcommands; launching claude goes through app/launcher directly.
package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jhyunwoo/ccam/internal/config"
	"github.com/jhyunwoo/ccam/internal/fsutil"
	"github.com/jhyunwoo/ccam/internal/launcher"
	"github.com/jhyunwoo/ccam/internal/lock"
	"github.com/jhyunwoo/ccam/internal/paths"
	"github.com/jhyunwoo/ccam/internal/profile"
	"github.com/jhyunwoo/ccam/internal/usage"
)

// Version is set at build time with -ldflags "-X .../cli.Version=...".
var Version = "dev"

// version is Version or, for a binary built without the ldflags (`go
// install github.com/jhyunwoo/ccam/cmd/ccam@v0.10.3`), the release its build
// info names. It reads the build info, so it never runs from init().
func version() string {
	if Version != "dev" {
		return Version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		return releaseOf(bi.Main.Version)
	}
	return Version
}

// releaseOf is a module version without its v when it names a release, and
// "dev" for (devel), a pseudo-version (go install ...@main, or a build a
// commit past a tag) or a +dirty build: a pseudo-version reads as a
// prerelease of the next release, which update would then offer to replace.
func releaseOf(v string) string {
	if !strings.HasPrefix(v, "v") || strings.Contains(v, "+") || pseudo(v) {
		return "dev"
	}
	return strings.TrimPrefix(v, "v")
}

// pseudo reports whether v ends like a Go pseudo-version: 14 digits of
// time, "-", 12 hex digits of the commit.
func pseudo(v string) bool {
	if len(v) < 27 || v[len(v)-13] != '-' {
		return false
	}
	for i, c := range []byte(v[len(v)-27:]) {
		switch {
		case i == 14:
		case i < 14 && c >= '0' && c <= '9':
		case i > 14 && (c >= '0' && c <= '9' || c >= 'a' && c <= 'f'):
		default:
			return false
		}
	}
	return true
}

// env is what every command works on.
type env struct {
	paths  paths.Paths
	stdin  io.Reader
	in     *bufio.Reader // the one buffered reader over stdin, shared by all prompts
	src    *lineSource   // the lines of in; read them through lines()
	stdout io.Writer
	stderr io.Writer
	dryRun bool
}

// lines is where prompts and a login's relay read stdin's lines, so that a
// read the relay leaves under way reaches the next of them.
func (e *env) lines() *lineSource {
	if e.src == nil {
		e.src = &lineSource{in: e.in}
	}
	return e.src
}

func (e *env) load() (*config.Config, error) { return config.Load(e.paths.ConfigFile()) }

func (e *env) save(cfg *config.Config) error { return cfg.Save(e.paths.ConfigFile()) }

// loadProfile loads the config and finds the account sel names.
func (e *env) loadProfile(sel string) (*config.Config, *config.Profile, error) {
	cfg, err := e.load()
	if err != nil {
		return nil, nil, err
	}
	prof, err := cfg.MustFind(sel)
	if err != nil {
		return nil, nil, err
	}
	return cfg, prof, nil
}

// update reloads the config, applies change and saves it. Commands that
// wait (a browser login, a prompt, the picker) use it so they do not save
// a copy loaded minutes ago over what another ccam wrote meanwhile.
func (e *env) update(change func(*config.Config) error) (*config.Config, error) {
	cfg, err := e.load()
	if err != nil {
		return nil, err
	}
	if err := change(cfg); err != nil {
		return nil, err
	}
	return cfg, e.save(cfg)
}

// refuseRunning stops commands that move or remove an account's folder
// while Claude Code still writes to it, however it was started. Ask it
// right before the change, after any question.
func refuseRunning(e *env, prof *config.Profile) error {
	if launcher.Running(e.paths, prof.Name, profile.Dir(e.paths, prof)) {
		return fmt.Errorf("%s is running; quit its Claude Code sessions first (`ccam agents` lists them, `ccam %s stop <id>` ends a background one)", prof.Name, prof.Name)
	}
	return nil
}

// canSave reports why cfg could not be saved: lines ccam does not read, or
// a config folder that takes no new file. Ask it before a step a failed
// save cannot undo, such as a logout.
func canSave(e *env, cfg *config.Config) error {
	path := e.paths.ConfigFile()
	if err := cfg.Writable(path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err == nil {
		err = fsutil.AtomicWrite(path, data, 0o600)
	}
	if err != nil {
		return fmt.Errorf("%s cannot be saved, so nothing was changed: %w", path, err)
	}
	return nil
}

// holdRefresh takes the lock `ccam usage --refresh` renews prof's login
// under, so that its folder is not moved or logged out while a renewed
// login is on its way; the func lets it go.
func holdRefresh(e *env, prof *config.Profile) (func(), error) {
	l, err := lock.TryFlock(usage.RefreshLock(launcher.SessionDir(e.paths, prof.Name)))
	if errors.Is(err, lock.ErrBusy) {
		return nil, fmt.Errorf("another ccam process is using the login of %s right now (a refresh, rename, remove or logout); try again in a moment", prof.Name)
	}
	if err != nil {
		return nil, err
	}
	return l.Unlock, nil
}

func (e *env) printf(format string, a ...any) { fmt.Fprintf(e.stdout, format, a...) }

// confirm asks a yes/no question; anything but y/yes is no.
func (e *env) confirm(question string) bool {
	fmt.Fprintf(e.stdout, "%s [y/N] ", question)
	line, _ := e.lines().readLine()
	ans := strings.ToLower(strings.TrimSpace(line))
	return ans == "y" || ans == "yes"
}

// ccamPath is the real ccam binary, which every shim links to.
func ccamPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func newRoot(e *env) *cobra.Command {
	root := &cobra.Command{
		Use:   "ccam",
		Short: "Claude Code multi-account manager",
		Long: "ccam adds and switches between several Claude Code accounts.\n\n" +
			"  ccam <name|number> [claude args]  run Claude Code as that account\n" +
			"  <shortcut> [claude args]          run by a shortcut command (ccam shortcut)\n" +
			"  ccam --fallback <name> -p \"…\"     run again as another account at a usage limit (ccam help run)",
		Version:       version(),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHome(e)
		},
	}
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if e.dryRun && !dryRunCommands[cmd.CommandPath()] {
			return fmt.Errorf("--dry-run is not supported by `%s`; nothing was changed", cmd.CommandPath())
		}
		return nil
	}
	root.SetIn(e.stdin)
	root.SetOut(e.stdout)
	root.SetErr(e.stderr)
	root.PersistentFlags().BoolVar(&e.dryRun, "dry-run", false, "print what would happen without doing it")
	root.CompletionOptions.DisableDefaultCmd = true
	root.ValidArgsFunction = profileNames(e) // `ccam <tab>` lists accounts
	root.AddCommand(
		sessionCmd(e), addCmd(e), listCmd(e), runCmd(e), useCmd(e), removeCmd(e), renameCmd(e),
		loginCmd(e), logoutCmd(e), shimsCmd(e), versionCmd(e), shareCmd(e), doctorCmd(e), mcpCmd(e), nextCmd(e), usageCmd(e), statuslineCmd(e), importCmd(e), migrateCmd(e), shortcutCmd(e), folderCmd(e), pickCmd(e), extCmd(e), groupCmd(e), hookCmd(e), agentsCmd(e), attachCmd(e), ccusageCmd(e), updateCmd(e), uninstallCmd(e), completionCmd(e),
	)
	for _, c := range root.Commands() {
		switch c.Name() {
		case "run", "use", "remove", "rename", "login", "logout", "share", "shortcut":
			c.ValidArgsFunction = profileNames(e)
		}
	}
	return root
}

// dryRunCommands honour --dry-run or change nothing. Every other command
// refuses the flag instead of quietly doing the real thing.
var dryRunCommands = map[string]bool{
	"ccam session list": true, "ccam session transfer": true, "ccam session verify": true, "ccam": true, "ccam run": true, "ccam next": true, "ccam share": true, "ccam migrate": true,
	"ccam shortcut": true, "ccam folder": true, "ccam pick": true, "ccam ext": true, "ccam update": true, "ccam doctor": true, "ccam usage": true,
	"ccam list": true, "ccam uninstall": true, "ccam mcp list": true, "ccam agents": true, "ccam attach": true, "ccam ccusage": true, "ccam group": true, "ccam group list": true, "ccam group show": true,
	"ccam group add": true, "ccam group set": true, "ccam group join": true, "ccam group leave": true, "ccam group remove": true, "ccam version": true, "ccam completion": true,
	"ccam help": true, "ccam __complete": true, "ccam __completeNoDesc": true,
}

func versionCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		Run:   func(cmd *cobra.Command, args []string) { e.printf("ccam %s\n", version()) },
	}
}

// Execute runs the CLI with args (without argv[0]) and returns the exit code.
func Execute(args []string) int {
	return ExecuteWith(args, paths.FromEnv(), os.Stdin, os.Stdout, os.Stderr)
}

func ExecuteWith(args []string, p paths.Paths, stdin io.Reader, stdout, stderr io.Writer) int {
	e := &env{paths: p, stdin: stdin, in: bufio.NewReader(stdin), stdout: stdout, stderr: stderr}
	root := newRoot(e)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		var exit exitError
		if errors.As(err, &exit) {
			return int(exit)
		}
		var code launcher.ExitCode // claude's own exit code where ccam waits for it
		if errors.As(err, &code) {
			return int(code)
		}
		var coded codedError
		if errors.As(err, &coded) {
			fmt.Fprintln(stderr, "ccam:", coded.msg)
			return coded.code
		}
		fmt.Fprintln(stderr, "ccam:", err)
		return 1
	}
	return 0
}

// exitError lets a command end with a specific code and no message.
type exitError int

func (e exitError) Error() string { return fmt.Sprintf("exit %d", int(e)) }
