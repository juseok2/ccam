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
	"github.com/spf13/cobra"
)

func addSessionStop(root *cobra.Command, e *env) {
	var expected string
	cmd := &cobra.Command{Use: "running <account>", Short: "Inspect tracked local processes; optionally request graceful shutdown", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		_, prof, err := e.loadProfile(args[0])
		if err != nil {
			return err
		}
		markers, dir := launcher.SessionDir(e.paths, prof.Name), profile.Dir(e.paths, prof)
		current := session.Processes(markers, dir)
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
				return fmt.Errorf("실행 세션이 변경되었습니다. 목록을 다시 확인하세요")
			}
		}
		for _, p := range current {
			// Never signal an arbitrary program solely because a record names it.
			out, err := exec.Command("/bin/ps", "-p", fmt.Sprint(p.PID), "-o", "comm=").Output()
			name := filepath.Base(strings.TrimSpace(string(out)))
			if err != nil || (name != "claude" && name != "ccam") {
				return fmt.Errorf("PID %d의 Claude 실행 파일을 확인할 수 없어 자동 종료하지 않습니다. 해당 터미널에서 /exit 해주세요", p.PID)
			}
		}
		if e.dryRun {
			return fmt.Errorf("shutdown is not available in dry-run mode")
		}
		for _, p := range current {
			// Recheck profile membership and start identity immediately before signalling.
			for _, live := range session.Processes(markers, dir) {
				if live.PID == p.PID && live.Start == p.Start {
					if err := syscall.Kill(p.PID, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
						return err
					}
				}
			}
		}
		deadline := time.Now().Add(20 * time.Second)
		for launcher.Running(e.paths, prof.Name, dir) {
			if time.Now().After(deadline) {
				return fmt.Errorf("%s가 아직 종료되지 않았습니다. 강제 종료하지 않았으며 전환을 중단합니다. 터미널에서 /exit 후 다시 시도하세요", prof.Name)
			}
			time.Sleep(100 * time.Millisecond)
		}
		return json.NewEncoder(e.stdout).Encode([]session.Process{})
	}}
	cmd.Flags().StringVar(&expected, "stop-expected", "", "JSON process list previously reviewed; send SIGTERM and wait, never SIGKILL")
	root.AddCommand(cmd)
}
