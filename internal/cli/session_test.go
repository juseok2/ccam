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

func TestConversationPreview(t *testing.T) {
	data := []byte(`{"type":"user","isMeta":true,"message":{"content":"hidden metadata"}}
{"type":"user","message":{"content":"재고 화면에서   저장 오류를 고쳐줘"}}
{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"hidden reasoning"},{"type":"tool_use","input":{"secret":"hidden tool"}},{"type":"text","text":"저장 로직을 수정했습니다.\n테스트도 통과했습니다."}]}}
{"type":"user","message":{"content":[{"type":"tool_result","content":"hidden result"}]}}`)
	title, preview := conversationPreview(data)
	if title != "재고 화면에서 저장 오류를 고쳐줘" || preview != "Claude: 저장 로직을 수정했습니다. 테스트도 통과했습니다." {
		t.Fatal(title, preview)
	}
	data = append(data, []byte("\n"+`{"type":"summary","summary":"재고 저장 오류 수정"}`+"\n"+`{"type":"custom-title","customTitle":"내 재고 작업"}`)...)
	title, _ = conversationPreview(data)
	if title != "내 재고 작업" {
		t.Fatal(title)
	}
	if excerpt("가나다라마바사", 3) != "가나다…" {
		t.Fatal("unicode truncation")
	}
	title, preview = conversationPreview([]byte("invalid\n" + `{"type":"user","message":{"content":"<command-name>/exit</command-name>"}}`))
	if title != "내용 미리보기가 없는 대화" || preview != "" {
		t.Fatal(title, preview)
	}
}
