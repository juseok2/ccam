package cli

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jhyunwoo/ccam/internal/launcher"
	"github.com/jhyunwoo/ccam/internal/profile"
	"github.com/jhyunwoo/ccam/internal/session"
	"github.com/jhyunwoo/ccam/internal/transfer"
	"github.com/spf13/cobra"
)

func addSessionStop(root *cobra.Command, e *env) {
	var expected, only, scope string
	cmd := &cobra.Command{Use: "running <account>", Short: "Inspect tracked local processes; optionally request graceful shutdown", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if only != "" && !transfer.ValidID(only) {
			return fmt.Errorf("invalid session UUID")
		}
		_, prof, err := e.loadProfile(args[0])
		if err != nil {
			return err
		}
		markers, dir := launcher.SessionDir(e.paths, prof.Name), profile.Dir(e.paths, prof)
		tracked := func() []session.Process {
			all := session.Processes(markers, dir)
			if only != "" {
				all = session.ForSession(all, only)
			}
			if scope != "" {
				all = session.InScope(all, scope)
			}
			return all
		}
		current := tracked()
		if !cmd.Flags().Changed("stop-expected") {
			return json.NewEncoder(e.stdout).Encode(current)
		}
		var approved []session.Process
		if err := json.Unmarshal([]byte(expected), &approved); err != nil {
			return err
		}
		// New processes must be shown to the user before touching any of them.
		for _, p := range current {
			matched := false
			for _, a := range approved {
				if p.PID == a.PID && p.Start == a.Start {
					matched = true
				}
			}
			if !matched {
				return fmt.Errorf("the running sessions changed; review the list again")
			}
		}
		if e.dryRun {
			return fmt.Errorf("shutdown is not available in dry-run mode")
		}
		// Confirm every program before signalling any, so a refusal stops
		// nothing half-way.
		for _, p := range current {
			if !isClaude(p.PID) {
				return fmt.Errorf("could not confirm that PID %d is Claude Code, so nothing was stopped; type /exit in its terminal", p.PID)
			}
		}
		for _, p := range current {
			// Recheck profile membership, start identity and the program
			// immediately before signalling: never signal an arbitrary
			// program solely because a record names it.
			for _, live := range tracked() {
				if live.PID != p.PID || live.Start != p.Start {
					continue
				}
				if !isClaude(p.PID) {
					return fmt.Errorf("could not confirm that PID %d is Claude Code, so it was not stopped; type /exit in its terminal", p.PID)
				}
				if err := syscall.Kill(p.PID, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
					return err
				}
			}
		}
		deadline := time.Now().Add(20 * time.Second)
		stillRunning := func() bool {
			if only == "" && scope == "" {
				return launcher.Running(e.paths, prof.Name, dir)
			}
			return len(tracked()) > 0
		}
		for stillRunning() {
			if time.Now().After(deadline) {
				return fmt.Errorf("%s is still running; nothing was force-stopped and the switch was cancelled; type /exit in its terminal and retry", prof.Name)
			}
			time.Sleep(100 * time.Millisecond)
		}
		return json.NewEncoder(e.stdout).Encode([]session.Process{})
	}}
	cmd.Flags().StringVar(&expected, "stop-expected", "", "JSON process list previously reviewed; send SIGTERM and wait, never SIGKILL")
	cmd.Flags().StringVar(&scope, "cwd", "", "only processes running in this project folder (or in an unknown folder)")
	cmd.Flags().StringVar(&only, "session", "", "only processes that may be writing this conversation (those recording no conversation included)")
	root.AddCommand(cmd)
}

// isClaude accepts the native binary (any install, reached through a link or
// not) and the npm package, which runs as node with Claude Code's cli.js.
func isClaude(pid int) bool {
	out, err := exec.Command("/bin/ps", "-p", fmt.Sprint(pid), "-o", "comm=").Output()
	if err != nil {
		return false
	}
	switch filepath.Base(strings.TrimSpace(string(out))) {
	case "claude", "ccam":
		return true
	case "node":
		args, err := exec.Command("/bin/ps", "-p", fmt.Sprint(pid), "-o", "args=").Output()
		return err == nil && strings.Contains(string(args), "@anthropic-ai/claude-code/")
	}
	return false
}
