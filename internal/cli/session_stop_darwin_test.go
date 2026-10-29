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
	"time"

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
	// A copied system binary keeps no valid signature: Apple Silicon kills it
	// at exec unless it is signed again (ad hoc is enough).
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", bin).CombinedOutput(); err != nil {
		t.Skip("codesign:", err, string(out))
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

// detachedClaude starts a signed copy of sleep named claude whose parent is
// not this test (Claude Code's own records of the test's children are
// ignored as usage probes), and returns its PID.
func detachedClaude(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile("/bin/sleep")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(bin, data, 0700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", bin).CombinedOutput(); err != nil {
		t.Skip("codesign:", err, string(out))
	}
	out, err := exec.Command("/bin/sh", "-c", `"$0" 60 >/dev/null 2>&1 & echo $!`, bin).Output()
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	for i := 0; i < 50; i++ {
		if _, ok := session.StartTime(pid); ok {
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("detached process did not start")
	return 0
}

func TestSessionStopOnlyTheConversation(t *testing.T) {
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
	const want, other = "12345678-1234-1234-1234-123456789abc", "87654321-4321-4321-4321-cba987654321"
	mine, theirs := detachedClaude(t), detachedClaude(t)
	records := filepath.Join(profile.Dir(p, prof), "sessions")
	if err := os.MkdirAll(records, 0700); err != nil {
		t.Fatal(err)
	}
	for pid, id := range map[int]string{mine: want, theirs: other} {
		rec := `{"pid":` + strconv.Itoa(pid) + `,"sessionId":"` + id + `","cwd":"/tmp/x"}`
		if err := os.WriteFile(filepath.Join(records, strconv.Itoa(pid)+".json"), []byte(rec), 0600); err != nil {
			t.Fatal(err)
		}
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
	var all []session.Process
	if err := json.Unmarshal(out.Bytes(), &all); err != nil || len(all) != 2 {
		t.Fatal("both processes without --session:", out.String(), err)
	}
	if run("session", "running", "source", "--session", want) != 0 {
		t.Fatal(errs.String())
	}
	var found []session.Process
	if err := json.Unmarshal(out.Bytes(), &found); err != nil || len(found) != 1 || found[0].PID != mine || found[0].Session != want {
		t.Fatal("only the conversation's process:", out.String(), err)
	}
	if run("session", "running", "source", "--session", want, "--stop-expected", out.String()) != 0 {
		t.Fatal(errs.String())
	}
	if _, ok := session.StartTime(mine); ok {
		t.Fatal("the conversation's process still running")
	}
	if _, ok := session.StartTime(theirs); !ok {
		t.Fatal("another conversation's process was stopped")
	}
	if run("session", "running", "source", "--session", "../bad") == 0 {
		t.Fatal("invalid session UUID accepted")
	}
}

func newSessionEnv(t *testing.T, names ...string) (paths.Paths, map[string]*config.Profile) {
	t.Helper()
	t.Setenv("CCAM_HOME", t.TempDir())
	p := paths.FromEnv()
	cfg := &config.Config{}
	profs := map[string]*config.Profile{}
	for _, n := range names {
		prof, err := profile.Add(p, cfg, n)
		if err != nil {
			t.Fatal(err)
		}
		profs[n] = prof
	}
	if err := cfg.Save(p.ConfigFile()); err != nil {
		t.Fatal(err)
	}
	return p, profs
}

func record(t *testing.T, dir string, pid int, id string) {
	t.Helper()
	records := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(records, 0700); err != nil {
		t.Fatal(err)
	}
	rec := `{"pid":` + strconv.Itoa(pid) + `,"sessionId":"` + id + `","cwd":"/tmp/x"}`
	if err := os.WriteFile(filepath.Join(records, strconv.Itoa(pid)+".json"), []byte(rec), 0600); err != nil {
		t.Fatal(err)
	}
}

// ccam waiting on its claude (relay) is marked by ccam's own pid: it takes
// the conversation its child records, so another conversation's switch
// leaves it alone. The daemon is never stopped for one conversation.
func TestSessionRunningRelayParentAndDaemon(t *testing.T) {
	p, profs := newSessionEnv(t, "source")
	dir := profile.Dir(p, profs["source"])
	const want, other = "12345678-1234-1234-1234-123456789abc", "87654321-4321-4321-4321-cba987654321"
	child := detachedClaude(t)
	// The detached child's parent is launchd; stand a plain process in as
	// ccam: start a shell that waits on its own claude child.
	bin := filepath.Join(t.TempDir(), "claude")
	data, _ := os.ReadFile("/bin/sleep")
	_ = os.WriteFile(bin, data, 0700)
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", bin).CombinedOutput(); err != nil {
		t.Skip("codesign:", err, string(out))
	}
	relay := exec.Command("/bin/sh", "-c", `"$0" 60 & wait`, bin)
	if err := relay.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = relay.Process.Kill(); _ = relay.Wait() }()
	var relayChild int
	for i := 0; i < 50 && relayChild == 0; i++ {
		out, _ := exec.Command("/usr/bin/pgrep", "-P", strconv.Itoa(relay.Process.Pid)).Output()
		relayChild, _ = strconv.Atoi(strings.TrimSpace(string(out)))
		time.Sleep(20 * time.Millisecond)
	}
	if relayChild == 0 {
		t.Fatal("relay child did not start")
	}
	defer syscall.Kill(relayChild, syscall.SIGKILL)
	start, _ := session.StartTime(relay.Process.Pid)
	markers := launcher.SessionDir(p, "source")
	if err := os.MkdirAll(markers, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(markers, strconv.Itoa(relay.Process.Pid)), []byte(start), 0600); err != nil {
		t.Fatal(err)
	}
	record(t, dir, relayChild, other)
	// child stands in for the daemon: daemon.lock names it.
	if err := os.WriteFile(filepath.Join(dir, "daemon.lock"), []byte(`{"pid":`+strconv.Itoa(child)+`}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	if ExecuteWith([]string{"session", "running", "source", "--session", want}, p, strings.NewReader(""), &out, &errs) != 0 {
		t.Fatal(errs.String())
	}
	var found []session.Process
	if err := json.Unmarshal(out.Bytes(), &found); err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatal("relay parent of another conversation or the daemon listed:", out.String())
	}
	out.Reset()
	if ExecuteWith([]string{"session", "running", "source", "--session", other}, p, strings.NewReader(""), &out, &errs) != 0 {
		t.Fatal(errs.String())
	}
	if err := json.Unmarshal(out.Bytes(), &found); err != nil || len(found) != 2 {
		t.Fatal("the relay parent and its claude belong to their conversation:", out.String())
	}
}

// A refusal for one process stops none of them.
func TestSessionStopConfirmsAllFirst(t *testing.T) {
	p, profs := newSessionEnv(t, "source")
	dir := profile.Dir(p, profs["source"])
	const want = "12345678-1234-1234-1234-123456789abc"
	claude := detachedClaude(t)
	plain := exec.Command("/bin/sh", "-c", `/bin/sleep 60 >/dev/null 2>&1 & echo $!`)
	o, err := plain.Output()
	if err != nil {
		t.Fatal(err)
	}
	notClaude, _ := strconv.Atoi(strings.TrimSpace(string(o)))
	defer syscall.Kill(notClaude, syscall.SIGKILL)
	record(t, dir, claude, want)
	record(t, dir, notClaude, want)
	var out, errs bytes.Buffer
	if ExecuteWith([]string{"session", "running", "source", "--session", want}, p, strings.NewReader(""), &out, &errs) != 0 {
		t.Fatal(errs.String())
	}
	reviewed := out.String()
	if ExecuteWith([]string{"session", "running", "source", "--session", want, "--stop-expected", reviewed}, p, strings.NewReader(""), &out, &errs) == 0 {
		t.Fatal("a process that is not Claude Code was accepted")
	}
	if _, ok := session.StartTime(claude); !ok {
		t.Fatal("claude stopped although the shutdown was refused")
	}
}

// With three accounts on one history, a third writing the conversation
// blocks the switch between the other two.
func TestSessionTransferSharedThirdAccount(t *testing.T) {
	p, profs := newSessionEnv(t, "a", "b", "c")
	group := filepath.Join(t.TempDir(), "projects")
	id, proj := "12345678-1234-1234-1234-123456789abc", "-tmp-test"
	if err := os.MkdirAll(filepath.Join(group, proj), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(group, proj, id+".jsonl"), []byte(`{"type":"user","sessionId":"`+id+`","message":{"content":"hi"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a", "b", "c"} {
		d := profile.Dir(p, profs[n])
		_ = os.RemoveAll(filepath.Join(d, "projects"))
		if err := os.Symlink(group, filepath.Join(d, "projects")); err != nil {
			t.Skip(err)
		}
	}
	record(t, profile.Dir(p, profs["c"]), detachedClaude(t), id)
	var out, errs bytes.Buffer
	if ExecuteWith([]string{"session", "transfer", "a", "b", id, "--project-folder", proj}, p, strings.NewReader(""), &out, &errs) == 0 {
		t.Fatal("switched while a third account sharing the history writes the conversation:", out.String())
	}
	if !strings.Contains(errs.String(), "c has a Claude Code session") {
		t.Fatal(errs.String())
	}
	// The app's preflight (before it stops anything) must refuse it too.
	errs.Reset()
	if ExecuteWith([]string{"--dry-run", "session", "transfer", "a", "b", id, "--project-folder", proj, "--inspect-running"}, p, strings.NewReader(""), &out, &errs) == 0 {
		t.Fatal("preflight passed although a third account writes the conversation")
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
