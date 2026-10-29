package cli

import (
	"bytes"
	"encoding/json"
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

func TestSessionListJSON(t *testing.T) {
	t.Setenv("CCAM_HOME", t.TempDir())
	p := paths.FromEnv()
	cfg := &config.Config{}
	a, err := profile.Add(p, cfg, "source")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(p.ConfigFile()); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	run := func() []map[string]any {
		t.Helper()
		out.Reset()
		errs.Reset()
		if ExecuteWith([]string{"session", "list", "source", "--json"}, p, strings.NewReader(""), &out, &errs) != 0 {
			t.Fatal(errs.String())
		}
		var rows []map[string]any
		if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
			t.Fatal(err, out.String())
		}
		if rows == nil {
			t.Fatal("expected JSON array, not null")
		}
		return rows
	}
	if len(run()) != 0 {
		t.Fatal("expected empty array")
	}
	dir := filepath.Join(profile.Dir(p, a), "projects", "-tmp-example")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	id := "12345678-1234-1234-1234-123456789abc"
	data := `{"type":"user","sessionId":"` + id + `","cwd":"/tmp/example","message":{"content":"PRIVATE TRANSCRIPT"}}`
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.jsonl"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	rows := run()
	if len(rows) != 1 || rows[0]["id"] != id || rows[0]["cwd"] != "/tmp/example" || rows[0]["project"] != "-tmp-example" || rows[0]["updated"] == "" {
		t.Fatal(rows)
	}
	if strings.Contains(out.String(), "PRIVATE TRANSCRIPT") {
		t.Fatal("transcript exposed")
	}
	if !strings.Contains(errs.String(), "Skipped") {
		t.Fatal("missing diagnostics")
	}
}
