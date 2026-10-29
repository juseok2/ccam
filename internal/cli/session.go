package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jhyunwoo/ccam/internal/lock"
	"github.com/jhyunwoo/ccam/internal/profile"
	"github.com/jhyunwoo/ccam/internal/transfer"
	"github.com/spf13/cobra"
)

func sessionCmd(e *env) *cobra.Command {
	root := &cobra.Command{Use: "session", Short: "Back up and manually transfer stopped conversations"}
	var project string
	copyCmd := &cobra.Command{
		Use:   "transfer <from-account> <to-account> <session-uuid>",
		Short: "Copy a stopped session, preserving both accounts' logins",
		Long:  "Copy one stopped conversation and its sidecars. Close Claude in both accounts first.\nNever copies login data, changes the default account, or switches on usage limits.\nUse --dry-run to validate without writing. Shared/symlinked projects are refused.",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
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
			if err := refuseRunning(e, from); err != nil {
				return err
			}
			if err := refuseRunning(e, to); err != nil {
				return err
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
	_ = copyCmd.MarkFlagRequired("project-folder")
	verify := &cobra.Command{Use: "verify <backup-directory>", Short: "Verify a saved session backup", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := transfer.Verify(args[0]); err != nil {
			return err
		}
		e.printf("Backup verified.\n")
		return nil
	}}
	var asJSON bool
	type sessionRow struct {
		ID      string    `json:"id"`
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
					rows = append(rows, sessionRow{ID: id, Project: project.Name(), Cwd: cwd, Updated: info.ModTime()})
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
	root.AddCommand(copyCmd, verify, list)
	return root
}
