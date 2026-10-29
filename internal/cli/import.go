package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jhyunwoo/ccam/internal/config"
	"github.com/jhyunwoo/ccam/internal/profile"
)

func importCmd(e *env) *cobra.Command {
	var name string
	var yes, candidates bool
	cmd := &cobra.Command{
		Use:   "import [folder]",
		Short: "Register an existing ~/.claude or ~/.claude-N folder as an account, in place",
		Long: "The folder is registered where it is, never moved or copied (copied tokens log each other out).\n" +
			"Without a folder, finds unregistered ~/.claude and ~/.claude-* folders and asks about each.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if name != "" && len(args) == 0 {
				return fmt.Errorf("--name needs a folder: ccam import <folder> --name %s", name)
			}
			cfg, err := e.load()
			if err != nil {
				return err
			}
			if candidates {
				type row struct {
					Path  string `json:"path"`
					Name  string `json:"name"`
					Email string `json:"email"`
				}
				rows := []row{}
				for _, c := range profile.FindCandidates(e.paths, cfg) {
					rows = append(rows, row{c.Path, c.Name, c.Email})
				}
				return json.NewEncoder(e.stdout).Encode(rows)
			}
			var paths []string
			names := map[string]string{}
			if len(args) == 1 {
				paths = []string{args[0]}
				names[args[0]] = name
			} else {
				cands := profile.FindCandidates(e.paths, cfg)
				if len(cands) == 0 {
					e.printf("No folders to import (~/.claude, ~/.claude-*).\n")
					return nil
				}
				for _, c := range cands {
					label := c.Name
					if label == "" {
						label = "(needs a name: ccam import " + c.Path + " --name <name>)"
					}
					q := fmt.Sprintf("Import %s (%s) as %s?", c.Path, orNone(c.Email), label)
					if c.Name != "" && (yes || e.confirm(q)) {
						paths = append(paths, c.Path)
					} else if c.Name == "" {
						e.printf("%s %s\n", c.Path, label)
					}
				}
			}
			if len(paths) == 0 {
				return nil
			}
			// The questions may have waited for minutes: register into the
			// config as it is now, not the copy loaded before them.
			var imported []string
			notes := map[string][]string{}
			cfg, err = e.update(func(cfg *config.Config) error {
				for _, path := range paths {
					prof, n, err := profile.Import(e.paths, cfg, path, names[path])
					if err != nil && len(args) == 1 {
						return err
					}
					if err != nil {
						fmt.Fprintf(e.stderr, "Skipped %s: %v\n", path, err)
						continue
					}
					// A folder copied from another (cp -r ~/.claude ~/.claude-2)
					// holds the same refresh token: whichever renews it first
					// logs the other out.
					if q := sameLogin(e, cfg, prof); q != "" {
						n = append(n, fmt.Sprintf("%s holds the same login as %s (a copied folder): whichever renews it first logs the other out. "+
							"Run `ccam login %s` before using it (not `ccam logout`, which would log %s out too)", prof.Dir, q, prof.Name, q))
					}
					imported = append(imported, prof.Name)
					notes[prof.Name] = n
				}
				if len(imported) == 0 {
					return exitError(1) // each folder was skipped, and said why
				}
				return nil
			})
			if err != nil {
				return err
			}
			for _, n := range imported {
				prof, _ := cfg.Find(n)
				for _, note := range notes[n] {
					e.printf("· %s\n", note)
				}
				e.printf("✓ %s (%s) registered\n", prof.Name, orNone(prof.Email))
				printShortcut(e, prof)
			}
			if _, err := installShims(e, cfg); err != nil {
				fmt.Fprintln(e.stderr, "Could not create shortcuts:", err)
			}
			e.printf("Imported accounts start with nothing shared. To share: `ccam group join <group> <name>` (see `ccam group`)\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "account name for the folder given (default: derived from the folder name)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "import every folder found without asking")
	cmd.Flags().BoolVar(&candidates, "candidates", false, "print the folders that could be imported as JSON, importing nothing")
	return cmd
}

func orNone(s string) string {
	if s == "" {
		return "not logged in"
	}
	return s
}

// accountLabel is what lists show for an account: its email, its API
// endpoint, or "not logged in".
func accountLabel(p *config.Profile) string {
	if p.API != nil && p.Email == "" {
		return p.API.Label()
	}
	return orNone(p.Email)
}
