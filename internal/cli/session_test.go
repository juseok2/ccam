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
	// A conversation that cannot be copied is still listed, with the reason,
	// so it can at least be resumed in its own account.
	bad := "87654321-4321-4321-4321-cba987654321"
	if err := os.WriteFile(filepath.Join(dir, bad+".jsonl"), []byte(`{"type":"user","sessionId":"`+bad+`","cwd":"/tmp/example"}`+"\n{truncated"), 0600); err != nil {
		t.Fatal(err)
	}
	rows := run()
	byID := map[string]map[string]any{}
	for _, r := range rows {
		byID[r["id"].(string)] = r
	}
	good := byID[id]
	if len(rows) != 2 || good == nil || good["cwd"] != "/tmp/example" || good["project"] != "-tmp-example" || good["updated"] == "" || good["transferable"] != true || good["reason"] != nil {
		t.Fatal(rows)
	}
	if r := byID[bad]; r == nil || r["transferable"] != false || !strings.Contains(r["reason"].(string), "invalid JSONL") {
		t.Fatal("untransferable conversation:", r)
	}
	if strings.Contains(out.String(), "PRIVATE TRANSCRIPT") {
		t.Fatal("transcript exposed")
	}
}

func TestProjectCwd(t *testing.T) {
	data := []byte(`{"cwd":"/Users/me/app"}
{"cwd":"relative"}
{"cwd":"/Users/me/app/web"}`)
	if got := projectCwd(data, "-Users-me-app"); got != "/Users/me/app" {
		t.Fatal("the folder the project is named after:", got)
	}
	if got := projectCwd(data, "-Users-me-other"); got != "/Users/me/app" {
		t.Fatal("first recorded folder:", got)
	}
	if projectName("/Users/me/한글 폴더_v1.2") != "-Users-me-------v1-2" {
		t.Fatal(projectName("/Users/me/한글 폴더_v1.2"))
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
	if title != "" || preview != "" {
		t.Fatal(title, preview)
	}
	_, preview = conversationPreview([]byte(`{"type":"user","message":{"content":"마지막 질문"}}`))
	if preview != "You: 마지막 질문" {
		t.Fatal(preview)
	}
}

// Accounts sharing conversation history (projects/ linked to one folder):
// the transfer copies nothing, keeps no backup and prints the resume command.
func TestSessionTransferSharedHistory(t *testing.T) {
	t.Setenv("CCAM_HOME", t.TempDir())
	p := paths.FromEnv()
	cfg := &config.Config{}
	a, err := profile.Add(p, cfg, "source")
	if err != nil {
		t.Fatal(err)
	}
	b, err := profile.Add(p, cfg, "target")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(p.ConfigFile()); err != nil {
		t.Fatal(err)
	}
	group := filepath.Join(t.TempDir(), "projects")
	id, proj := "12345678-1234-1234-1234-123456789abc", "-tmp-test"
	if err := os.MkdirAll(filepath.Join(group, proj), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(group, proj, id+".jsonl"), []byte(`{"type":"user","sessionId":"`+id+`","cwd":"/tmp/test","message":{"content":"hi"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, prof := range []*config.Profile{a, b} {
		_ = os.RemoveAll(filepath.Join(profile.Dir(p, prof), "projects"))
		if err := os.Symlink(group, filepath.Join(profile.Dir(p, prof), "projects")); err != nil {
			t.Skip(err)
		}
	}
	var out, errs bytes.Buffer
	if ExecuteWith([]string{"session", "list", "target", "--json"}, p, strings.NewReader(""), &out, &errs) != 0 {
		t.Fatal(errs.String())
	}
	if !strings.Contains(out.String(), id) || !strings.Contains(out.String(), `"transferable":true`) {
		t.Fatal("shared conversation not listed:", out.String())
	}
	out.Reset()
	errs.Reset()
	if ExecuteWith([]string{"session", "transfer", "source", "target", id, "--project-folder", proj}, p, strings.NewReader(""), &out, &errs) != 0 {
		t.Fatal(errs.String())
	}
	if !strings.Contains(out.String(), "nothing to copy") || !strings.Contains(out.String(), "ccam target --resume "+id) {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(filepath.Join(p.State, "session-backups")); !os.IsNotExist(err) {
		t.Fatal("backup written for a shared history")
	}
	// Nothing is copied, so a conversation the copy checks refuse (a cut
	// last line) still switches.
	f, err := os.OpenFile(filepath.Join(group, proj, id+".jsonl"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("\n{cut")
	_ = f.Close()
	out.Reset()
	errs.Reset()
	if ExecuteWith([]string{"session", "transfer", "source", "target", id, "--project-folder", proj}, p, strings.NewReader(""), &out, &errs) != 0 {
		t.Fatal("shared history refused a conversation it does not copy:", errs.String())
	}
}
