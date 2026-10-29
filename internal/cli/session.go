package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jhyunwoo/ccam/internal/config"
	"github.com/jhyunwoo/ccam/internal/launcher"
	"github.com/jhyunwoo/ccam/internal/lock"
	"github.com/jhyunwoo/ccam/internal/profile"
	"github.com/jhyunwoo/ccam/internal/session"
	"github.com/jhyunwoo/ccam/internal/transfer"
	"github.com/spf13/cobra"
)

func sessionCmd(e *env) *cobra.Command {
	root := &cobra.Command{Use: "session", Short: "Back up and manually transfer stopped conversations"}
	var project string
	var inspectRunning bool
	var runningCwd string
	copyCmd := &cobra.Command{
		Use:   "transfer <from-account> <to-account> <session-uuid>",
		Short: "Copy a stopped session, preserving both accounts' logins",
		Long:  "Copy one stopped conversation and its sidecars. Quit the Claude Code sessions writing this conversation first.\nNever copies login data, changes the default account, or switches on usage limits.\nUse --dry-run to validate without writing. When both accounts share conversation history nothing is copied.",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if inspectRunning && !e.dryRun {
				return fmt.Errorf("--inspect-running requires --dry-run")
			}
			cfg, from, err := e.loadProfile(args[0])
			if err != nil {
				return err
			}
			to, err := cfg.MustFind(args[1])
			if err != nil {
				return err
			}
			if from.Name == to.Name {
				return fmt.Errorf("choose two different accounts")
			}
			if !transfer.ValidID(args[2]) {
				return fmt.Errorf("invalid session UUID")
			}
			// Serialize transfers; the existing refresh locks also exclude account removal.
			l, err := lock.TryFlock(filepath.Join(e.paths.Runtime, "session-transfer.lock"))
			if errors.Is(err, lock.ErrBusy) {
				return fmt.Errorf("another session transfer is in progress; try again when it ends")
			}
			if err != nil {
				return err
			}
			defer l.Unlock()
			release, err := holdRefresh(e, from)
			if err != nil {
				return err
			}
			defer release()
			releaseTo, err := holdRefresh(e, to)
			if err != nil {
				return err
			}
			defer releaseTo()
			// Only processes that may be writing this conversation matter:
			// the copy reads one conversation and creates new files only.
			if err := refuseSessionRunning(e, from, args[2]); err != nil && !inspectRunning {
				return err
			}
			if err := refuseSessionRunning(e, to, args[2]); err != nil && !inspectRunning {
				return err
			}
			// --running-cwd also refuses every session in the project folder.
			if runningCwd != "" {
				for _, prof := range []*config.Profile{from, to} {
					if err := refuseRunningIn(e, prof, runningCwd); err != nil && !inspectRunning {
						return err
					}
				}
			}
			if transfer.Shared(profile.Dir(e.paths, from), profile.Dir(e.paths, to)) {
				// Every account reading this history could be writing it. The
				// switch stops sessions of the two accounts only, so another
				// account's is refused in the preflight too.
				for i := range cfg.Profiles {
					other := &cfg.Profiles[i]
					if other.Name == from.Name || other.Name == to.Name || !transfer.Shared(profile.Dir(e.paths, from), profile.Dir(e.paths, other)) {
						continue
					}
					if err := refuseSessionRunning(e, other, args[2]); err != nil {
						return err
					}
				}
				if err := transfer.Locate(profile.Dir(e.paths, from), project, args[2]); err != nil {
					return err
				}
				e.printf("Both accounts share conversation history; nothing to copy.\n")
				e.printf("In the original project directory, resume explicitly:\n  ccam %s --resume %s\n", to.Name, args[2])
				return nil
			}
			backup, err := transfer.Copy(profile.Dir(e.paths, from), profile.Dir(e.paths, to), filepath.Join(e.paths.State, "session-backups"), project, args[2], e.dryRun)
			if err != nil {
				if backup != "" {
					e.printf("Backup retained: %s\n", backup)
				}
				return err
			}
			if e.dryRun {
				e.printf("Validated; no session files written.\n")
				return nil
			}
			e.printf("Copied and verified. Source and logins unchanged.\nBackup: %s\n", backup)
			e.printf("In the original project directory, resume explicitly:\n  ccam %s --resume %s\n", to.Name, args[2])
			return nil
		},
	}
	copyCmd.Flags().StringVar(&project, "project-folder", "", "Exact folder name under the source profile's projects directory")
	copyCmd.Flags().BoolVar(&inspectRunning, "inspect-running", false, "preflight files without stopping writers; requires --dry-run; transfer revalidates after shutdown")
	copyCmd.Flags().StringVar(&runningCwd, "running-cwd", "", "also refuse every session running in this project folder (or in an unknown folder)")
	_ = copyCmd.MarkFlagRequired("project-folder")
	verify := &cobra.Command{Use: "verify <backup-directory>", Short: "Verify a saved session backup", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := transfer.Verify(args[0]); err != nil {
			return err
		}
		e.printf("Backup verified.\n")
		return nil
	}}
	var asJSON, previews bool
	type sessionRow struct {
		ID      string    `json:"id"`
		Title   string    `json:"title,omitempty"`
		Preview string    `json:"preview,omitempty"`
		Project string    `json:"project"`
		Cwd     string    `json:"cwd"`
		Updated time.Time `json:"updated"`
		// Transferable is false when the conversation can be resumed in its
		// own account but not copied; Reason says why.
		Transferable bool   `json:"transferable"`
		Reason       string `json:"reason,omitempty"`
	}
	list := &cobra.Command{Use: "list <account>", Short: "List session UUIDs, project folders and whether each can be transferred", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		_, prof, err := e.loadProfile(args[0])
		if err != nil {
			return err
		}
		rows := []sessionRow{}
		base := filepath.Join(profile.Dir(e.paths, prof), "projects")
		if info, err := os.Lstat(base); err == nil && info.Mode()&os.ModeSymlink != 0 {
			// A shared history group links projects/; a broken link must
			// not read as "no conversations".
			real, err := filepath.EvalSymlinks(base)
			if err != nil {
				return fmt.Errorf("the projects folder of %s is a broken link (%v); check its group with ccam doctor", prof.Name, err)
			}
			base = real
		}
		entries, err := os.ReadDir(base)
		if os.IsNotExist(err) {
			if asJSON {
				return json.NewEncoder(e.stdout).Encode(rows)
			}
			return nil
		}
		if err != nil {
			return err
		}
		for _, project := range entries {
			if !project.IsDir() {
				continue
			}
			files, err := os.ReadDir(filepath.Join(base, project.Name()))
			if err != nil {
				return err
			}
			for _, f := range files {
				if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
					continue
				}
				id := strings.TrimSuffix(f.Name(), ".jsonl")
				if !transfer.ValidID(id) {
					continue
				}
				reason := ""
				if _, err := transfer.Snapshot(profile.Dir(e.paths, prof), project.Name(), id); err != nil {
					reason = err.Error()
					if !asJSON {
						fmt.Fprintf(e.stderr, "Not transferable %q / %q: %v\n", project.Name(), id, err)
						continue
					}
				}
				if asJSON {
					path := filepath.Join(base, project.Name(), f.Name())
					data, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					info, err := os.Stat(path)
					if err != nil {
						return err
					}
					item := sessionRow{ID: id, Project: project.Name(), Cwd: projectCwd(data, project.Name()), Updated: info.ModTime(), Transferable: reason == "", Reason: reason}
					if previews {
						item.Title, item.Preview = conversationPreview(data)
					}
					rows = append(rows, item)
				} else {
					e.printf("%s  --project-folder %q\n", id, project.Name())
				}
			}
		}
		if asJSON {
			return json.NewEncoder(e.stdout).Encode(rows)
		}
		return nil
	}}
	list.Flags().BoolVar(&asJSON, "json", false, "print session metadata as JSON (no message contents)")
	list.Flags().BoolVar(&previews, "preview", false, "include local conversation title and text excerpts in JSON")
	root.AddCommand(copyCmd, verify, list)
	addSessionStop(root, e)
	return root
}

// refuseSessionRunning fails when a process of prof may be writing
// conversation id (see session.ForSession).
func refuseSessionRunning(e *env, prof *config.Profile, id string) error {
	running := session.ForSession(session.Processes(launcher.SessionDir(e.paths, prof.Name), profile.Dir(e.paths, prof)), id)
	if len(running) > 0 {
		return fmt.Errorf("%s has a Claude Code session that may be writing this conversation (PID %d); quit it first", prof.Name, running[0].PID)
	}
	return nil
}

// projectCwd is the folder to resume in: the recorded cwd whose Claude Code
// project folder name is project (it names any character but ASCII letters
// and digits "-"), else the first recorded one. The last cwd may be a
// subfolder the conversation moved into.
func projectCwd(data []byte, project string) string {
	first := ""
	for _, line := range bytes.Split(data, []byte("\n")) {
		var row struct {
			Cwd string `json:"cwd"`
		}
		if json.Unmarshal(line, &row) != nil || !filepath.IsAbs(row.Cwd) {
			continue
		}
		if projectName(row.Cwd) == project {
			return row.Cwd
		}
		if first == "" {
			first = row.Cwd
		}
	}
	return first
}

func projectName(cwd string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, cwd)
}

// Only conversational text is shown: never tool arguments, results or thinking.
// The latest excerpt starts with "You: " or "Claude: "; the title is empty
// when the conversation has no text to show (the app words both).
func conversationPreview(data []byte) (string, string) {
	var first, latest, summary, custom string
	for _, line := range bytes.Split(data, []byte("\n")) {
		var row struct {
			Type        string `json:"type"`
			Summary     string `json:"summary"`
			CustomTitle string `json:"customTitle"`
			IsMeta      bool   `json:"isMeta"`
			Message     struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &row) != nil {
			continue
		}
		if row.Type == "summary" {
			summary = excerpt(row.Summary, 90)
		}
		if row.Type == "custom-title" {
			custom = excerpt(row.CustomTitle, 90)
		}
		if row.IsMeta || (row.Type != "user" && row.Type != "assistant") {
			continue
		}
		var text string
		if json.Unmarshal(row.Message.Content, &text) != nil {
			var blocks []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if json.Unmarshal(row.Message.Content, &blocks) != nil {
				continue
			}
			for _, block := range blocks {
				if block.Type == "text" {
					text += " " + block.Text
				}
			}
		}
		text = strings.TrimSpace(text)
		// CLI control messages are not useful conversation labels.
		if text == "" || strings.HasPrefix(text, "<") {
			continue
		}
		if row.Type == "user" && first == "" {
			first = excerpt(text, 90)
		}
		prefix := "You: "
		if row.Type == "assistant" {
			prefix = "Claude: "
		}
		latest = prefix + excerpt(text, 220)
	}
	title := first
	if summary != "" {
		title = summary
	}
	if custom != "" {
		title = custom
	}
	return title, latest
}
func excerpt(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	chars := []rune(text)
	if len(chars) > limit {
		return string(chars[:limit]) + "…"
	}
	return text
}

// refuseRunningIn is refuseRunning limited to the sessions that may write a
// conversation of project cwd. Without cwd it is refuseRunning.
func refuseRunningIn(e *env, prof *config.Profile, cwd string) error {
	if cwd == "" {
		return refuseRunning(e, prof)
	}
	markers, dir := launcher.SessionDir(e.paths, prof.Name), profile.Dir(e.paths, prof)
	if n := len(session.InScope(session.Processes(markers, dir), cwd)); n > 0 {
		return fmt.Errorf("%s has %d session(s) running in %s; quit them first", prof.Name, n, cwd)
	}
	return nil
}
