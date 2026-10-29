package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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
	// Recent macOS kills a copied system binary until it is signed again.
	if out, err := exec.Command("/usr/bin/codesign", "-f", "-s", "-", bin).CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
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

func TestSessionStopOnlyInProjectFolder(t *testing.T) {
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
	data, err := os.ReadFile("/bin/sleep")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(bin, data, 0700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/usr/bin/codesign", "-f", "-s", "-", bin).CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	// Two Claude Code sessions/<pid>.json records: one in the project, one elsewhere.
	sessions := filepath.Join(profile.Dir(p, prof), "sessions")
	if err := os.MkdirAll(sessions, 0700); err != nil {
		t.Fatal(err)
	}
	// Started through a shell that exits at once, so the sessions are not
	// children of this process (those are taken for ccam's own usage probe).
	start := func(cwd string) int {
		raw, err := exec.Command("/bin/sh", "-c", "'"+bin+"' 60 >/dev/null 2>&1 & echo $!").Output()
		if err != nil {
			t.Fatal(err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
		rec, _ := json.Marshal(map[string]any{"pid": pid, "cwd": cwd})
		if err := os.WriteFile(filepath.Join(sessions, strconv.Itoa(pid)+".json"), rec, 0600); err != nil {
			t.Fatal(err)
		}
		return pid
	}
	inside, other := start("/work/p"), start("/work/other")
	var out, errs bytes.Buffer
	run := func(args ...string) int {
		out.Reset()
		errs.Reset()
		return ExecuteWith(args, p, strings.NewReader(""), &out, &errs)
	}
	if run("session", "running", "source", "--cwd", "/work/p") != 0 {
		t.Fatal(errs.String())
	}
	var found []session.Process
	if err := json.Unmarshal(out.Bytes(), &found); err != nil || len(found) != 1 || found[0].PID != inside {
		t.Fatal(out.String(), err)
	}
	if run("session", "running", "source", "--cwd", "/work/p", "--stop-expected", out.String()) != 0 {
		t.Fatal(errs.String())
	}
	if _, ok := session.StartTime(inside); ok {
		t.Fatal("session in the project still running")
	}
	if _, ok := session.StartTime(other); !ok {
		t.Fatal("session in another project stopped")
	}
}
