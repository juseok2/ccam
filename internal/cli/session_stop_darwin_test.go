package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jhyunwoo/ccam/internal/config"
	"github.com/jhyunwoo/ccam/internal/launcher"
	"github.com/jhyunwoo/ccam/internal/paths"
	"github.com/jhyunwoo/ccam/internal/profile"
	"github.com/jhyunwoo/ccam/internal/session"
)

func TestSessionStopReviewedProcesses(t *testing.T) {
	t.Setenv("CCAM_HOME", t.TempDir())
	p := paths.FromEnv()
	cfg := &config.Config{}
	prof, err := profile.Add(p, cfg, "source")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(p.ConfigFile()); err != nil {
		t.Fatal(err)
	}
	// A harmless fixture executable, named claude, with ordinary SIGTERM behavior.
	data, err := os.ReadFile("/bin/sleep")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(bin, data, 0700); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(bin, "60")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	unrelated := exec.Command("/bin/sleep", "60")
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unrelated.Process.Kill(); _ = unrelated.Wait() }()
	start, ok := session.StartTime(child.Process.Pid)
	if !ok {
		t.Fatal("missing process")
	}
	dir := launcher.SessionDir(p, prof.Name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(child.Process.Pid)), []byte(start), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	run := func(args ...string) int {
		out.Reset()
		errs.Reset()
		return ExecuteWith(args, p, strings.NewReader(""), &out, &errs)
	}
	if run("session", "running", "source") != 0 {
		t.Fatal(errs.String())
	}
	var found []session.Process
	if err := json.Unmarshal(out.Bytes(), &found); err != nil || len(found) != 1 {
		t.Fatal(out.String(), err)
	}
	reviewed := out.String()
	if run("session", "running", "source", "--stop-expected", "[]") == 0 {
		t.Fatal("unreviewed process stopped")
	}
	if _, ok := session.StartTime(child.Process.Pid); !ok {
		t.Fatal("child terminated before approval")
	}
	if run("session", "running", "source", "--stop-expected", reviewed) != 0 {
		t.Fatal(errs.String())
	}
	if _, ok := session.StartTime(child.Process.Pid); ok {
		t.Fatal("child still running")
	}
	if _, ok := session.StartTime(unrelated.Process.Pid); !ok {
		t.Fatal("unrelated process stopped")
	}
}
