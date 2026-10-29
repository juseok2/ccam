package cli

import (
	"bytes"
	"encoding/json"
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
		Long:  "Copy one stopped conversation and its sidecars. Close Claude in both accounts first.\nNever copies login data, changes the default account, or switches on usage limits.\nUse --dry-run to validate without writing. Shared/symlinked projects are refused.",
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
			// Serialize transfers; the existing refresh locks also exclude account removal.
			l, err := lock.TryFlock(filepath.Join(e.paths.Runtime, "session-transfer.lock"))
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
			for _, prof := range []*config.Profile{from, to} {
				if err := refuseRunningIn(e, prof, runningCwd); err != nil && !inspectRunning {
					return err
				}
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
	copyCmd.Flags().StringVar(&runningCwd, "running-cwd", "", "refuse only sessions running in this project folder (or in an unknown folder); others keep running")
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
	}
	list := &cobra.Command{Use: "list <account>", Short: "List transferable session UUIDs and project folders", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		_, prof, err := e.loadProfile(args[0])
		if err != nil {
			return err
		}
		rows := []sessionRow{}
		base := filepath.Join(profile.Dir(e.paths, prof), "projects")
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
				if _, err := transfer.Snapshot(profile.Dir(e.paths, prof), project.Name(), id); err != nil {
					fmt.Fprintf(e.stderr, "Skipped %q / %q: %v\n", project.Name(), id, err)
					continue
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
					cwd := ""
					for _, line := range bytes.Split(data, []byte("\n")) {
						var row struct {
							Cwd string `json:"cwd"`
						}
						if json.Unmarshal(line, &row) == nil && filepath.IsAbs(row.Cwd) {
							cwd = row.Cwd
						}
					}
					item := sessionRow{ID: id, Project: project.Name(), Cwd: cwd, Updated: info.ModTime()}
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

// Only conversational text is shown: never tool arguments, results or thinking.
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
		prefix := "나: "
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
	if title == "" {
		title = "내용 미리보기가 없는 대화"
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
