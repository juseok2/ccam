package cli

import (
	"bytes"
	"github.com/jhyunwoo/ccam/internal/config"
	"github.com/jhyunwoo/ccam/internal/paths"
	"github.com/jhyunwoo/ccam/internal/profile"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionTransferCLI(t *testing.T) {
	t.Setenv("CCAM_HOME", t.TempDir())
	p := paths.FromEnv()
	cfg := &config.Config{}
	a, err := profile.Add(p, cfg, "source")
	if err != nil {
		t.Fatal(err)
	}
	src := profile.Dir(p, a)
	b, err := profile.Add(p, cfg, "target")
	if err != nil {
		t.Fatal(err)
	}
	dst := profile.Dir(p, b)
	if err := cfg.Save(p.ConfigFile()); err != nil {
		t.Fatal(err)
	}
	id := "12345678-1234-1234-1234-123456789abc"
	proj := "-tmp-test"
	dir := filepath.Join(src, "projects", proj)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(`{"type":"user","sessionId":"`+id+`","message":{"content":"hi"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	run := func(args ...string) int {
		out.Reset()
		errs.Reset()
		return ExecuteWith(args, p, strings.NewReader(""), &out, &errs)
	}
	args := []string{"session", "transfer", "source", "target", id, "--project-folder", proj}
	if code := run(append([]string{"--dry-run"}, args...)...); code != 0 {
		t.Fatal(errs.String())
	}
	if _, err := os.Stat(filepath.Join(dst, "projects")); !os.IsNotExist(err) {
		t.Fatal("dry run wrote session")
	}
	if code := run(args...); code != 0 {
		t.Fatal(errs.String())
	}
	if !strings.Contains(out.String(), "ccam target --resume "+id) {
		t.Fatal(out.String())
	}
	backups, err := os.ReadDir(filepath.Join(p.State, "session-backups"))
	if err != nil || len(backups) != 1 {
		t.Fatal(backups, err)
	}
	if code := run("session", "verify", filepath.Join(p.State, "session-backups", backups[0].Name())); code != 0 {
		t.Fatal(errs.String())
	}
	if code := run(args...); code == 0 {
		t.Fatal("overwrote existing session")
	}
	if code := run("session", "transfer", "source", "source", id, "--project-folder", proj); code == 0 {
		t.Fatal("same profile accepted")
	}
}
