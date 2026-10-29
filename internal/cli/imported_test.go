package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jhyunwoo/ccam/internal/launcher"
)

// writeFiles writes each file under dir, making the folders it needs.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// homeClaude is the files of a plain Claude Code setup in c's home folder.
var homeClaude = map[string]string{
	"settings.json":       `{"model":"opus"}`,
	"CLAUDE.md":           "mine",
	"skills/s1/SKILL.md":  "my skill",
	"projects/-p/s.jsonl": `{"type":"user"}` + "\n",
	"history.jsonl":       `{"display":"hi"}` + "\n",
	".credentials.json":   `{"claudeAiOauth":{"accessToken":"t","refreshToken":"r","expiresAt":4102444800000}}`,
}

// importHome imports a plain Claude Code setup (~/.claude and ~/.claude.json
// with its own MCP server) as account main.
func importHome(t *testing.T, c *testCLI) string {
	t.Helper()
	dir := filepath.Join(c.p.Home, ".claude")
	writeFiles(t, dir, homeClaude)
	writeFiles(t, c.p.Home, map[string]string{".claude.json": `{"oauthAccount":{"emailAddress":"me@example.com","accountUuid":"u-me"},"mcpServers":{"own":{"command":"own-server"}}}`})
	c.ok("import", dir)
	return dir
}

// linksIntoGroups lists the entries of dir that link into ccam's groups.
func linksIntoGroups(t *testing.T, c *testCLI, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if target, err := os.Readlink(filepath.Join(dir, e.Name())); err == nil && strings.HasPrefix(target, c.p.GroupsRoot()) {
			out = append(out, e.Name())
		}
	}
	return out
}

// homeIntact checks that the imported folder holds its own files again.
func homeIntact(t *testing.T, c *testCLI, dir string) {
	t.Helper()
	for name, want := range homeClaude {
		path := filepath.Join(dir, name)
		if _, err := os.Readlink(path); err == nil {
			t.Errorf("%s is still a link", name)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			t.Errorf("%s: got %q, %v; want %q", name, got, err, want)
		}
	}
	var doc struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	data, err := os.ReadFile(filepath.Join(c.p.Home, ".claude.json"))
	if err != nil || json.Unmarshal(data, &doc) != nil {
		t.Fatalf("~/.claude.json: %v %s", err, data)
	}
	if _, ok := doc.MCPServers["own"]; !ok {
		t.Errorf("the MCP server moved into the group is not back in ~/.claude.json: %s", data)
	}
}

// An account registered in place stays where it is when it is removed, and
// plain claude goes on using it: it must hold its own files again, not
// links into ccam's groups, whose folders a later `group remove` moves to
// the trash and an uninstall deletes.
func TestRemoveImportedGivesTheFolderItsFilesBack(t *testing.T) {
	for _, strategy := range []string{"", "sync"} {
		t.Run("mcp_strategy="+strategy, func(t *testing.T) {
			c := newTestCLI(t, nil)
			cfg := c.cfg()
			cfg.MCPStrategy = strategy
			if err := cfg.Save(c.p.ConfigFile()); err != nil {
				t.Fatal(err)
			}
			dir := importHome(t, c)
			c.ok("group", "add", "home", "--items", "mcp,settings,memory,extensions", "--members", "main,work", "--yes")
			c.ok("group", "join", "team", "main", "--yes")
			c.ok("mcp", "import", "main", "own")
			if got := linksIntoGroups(t, c, dir); len(got) == 0 {
				t.Fatal("the setup shares nothing")
			}

			out := c.ok("remove", "main", "--yes")
			if !strings.Contains(out, "left its groups") {
				t.Errorf("the message does not say the folder left its groups:\n%s", out)
			}
			if _, ok := c.cfg().Find("main"); ok {
				t.Fatal("main is still registered")
			}
			if got := linksIntoGroups(t, c, dir); len(got) > 0 {
				t.Fatalf("still linked into the groups: %v", got)
			}
			homeIntact(t, c, dir)
			// Leaving the groups edited ~/.claude.json, which plain claude
			// goes on using: its backups stay, beside the folder.
			if kept, _ := filepath.Glob(filepath.Join(dir, ".ccam-backup", "mcp-*", "*")); len(kept) == 0 || !strings.Contains(out, "backups of its .claude.json are in") {
				t.Errorf("the .claude.json backups are gone (%v):\n%s", kept, out)
			}

			// What the groups do next, or deleting ccam's data, leaves it alone.
			c.ok("group", "remove", "home", "--leave", "--yes")
			if err := os.RemoveAll(c.p.Data); err != nil {
				t.Fatal(err)
			}
			homeIntact(t, c, dir)
		})
	}
}

// When the .claude.json backups cannot go into the imported folder, they
// stay where they are, and the message says where.
func TestRemoveImportedKeepsBackupsItCannotMove(t *testing.T) {
	c := newTestCLI(t, nil)
	dir := importHome(t, c)
	backups := filepath.Join(launcher.MCPStateDir(c.p, "main"), "backups")
	writeFiles(t, backups, map[string]string{"claude.json.1": "{}"})
	writeFiles(t, c.p.State, map[string]string{"mcp/main/other": "x"})
	writeFiles(t, dir, map[string]string{".ccam-backup": "a file, so nothing goes under it"})
	out := c.ok("remove", "main", "--yes")
	if !fileThere(filepath.Join(backups, "claude.json.1")) || !strings.Contains(out, backups) {
		t.Fatalf("the backups are gone, or not named:\n%s", out)
	}
	if fileThere(filepath.Join(launcher.MCPStateDir(c.p, "main"), "other")) {
		t.Fatal("the rest of the name's state stayed")
	}
}

// Leaving a group with --fresh gives an imported folder a copy all the
// same: what it shared were its own files before it joined, and plain
// claude would lose them.
func TestImportedFolderKeepsACopyWithFresh(t *testing.T) {
	c := newTestCLI(t, nil)
	dir := importHome(t, c)
	c.ok("group", "add", "home", "--items", "settings,memory,extensions", "--members", "main,work", "--yes")
	out := c.ok("group", "leave", "home", "main", "--fresh", "--yes")
	if !strings.Contains(out, "keeps a copy") {
		t.Errorf("the plan does not say why --fresh does not apply:\n%s", out)
	}
	if got := linksIntoGroups(t, c, dir); len(got) > 0 {
		t.Fatalf("still linked into the groups: %v", got)
	}
	for _, name := range []string{"settings.json", "CLAUDE.md", "skills/s1/SKILL.md"} {
		if got := readFile(t, filepath.Join(dir, name)); got != homeClaude[name] {
			t.Errorf("%s: %q", name, got)
		}
	}
	// A folder of ccam's own starts without them, as --fresh says.
	c.ok("group", "leave", "home", "work", "--fresh", "--yes")
	if _, err := os.Lstat(filepath.Join(c.p.ProfileDir("work"), "settings.json")); err == nil {
		t.Error("work kept settings.json despite --fresh")
	}
}

// ccam import may wait at its questions for minutes: what other ccam
// commands save meanwhile stays.
func TestImportKeepsWhatWasSavedDuringItsQuestions(t *testing.T) {
	c := newTestCLI(t, nil)
	writeFiles(t, filepath.Join(c.p.Home, ".claude-extra"), map[string]string{"settings.json": "{}"})
	meanwhile := func() {
		for _, args := range [][]string{{"add", "c6", "--no-login"}, {"rename", "side", "side2"}, {"use", "work"}} {
			if out, errOut, code := c.run("", args...); code != 0 {
				t.Errorf("%v: %d %s%s", args, code, out, errOut)
			}
		}
	}
	out, errOut, code := c.runWith(&startsOnRead{start: meanwhile, r: strings.NewReader("y\n")}, "import")
	if code != 0 {
		t.Fatalf("%d %s%s", code, out, errOut)
	}
	cfg := c.cfg()
	for _, n := range []string{"extra", "c6", "side2"} {
		if _, ok := cfg.Find(n); !ok {
			t.Errorf("%s is missing after the import", n)
		}
	}
	if _, ok := cfg.Find("side"); ok {
		t.Error("the import brought back the old name of side2")
	}
	if cfg.Default != "work" {
		t.Errorf("default %q, want work", cfg.Default)
	}
}

// A found folder whose name is an account's already is not offered under
// that name, so the others are imported, and nothing is reported as
// registered that is not.
func TestImportSkipsAFolderWhoseNameIsTaken(t *testing.T) {
	c := newTestCLI(t, nil)
	writeFiles(t, filepath.Join(c.p.Home, ".claude"), map[string]string{"settings.json": "{}"})
	writeFiles(t, filepath.Join(c.p.Home, ".claude-work"), map[string]string{"settings.json": "{}"})
	out, errOut, code := c.run("", "import", "--yes")
	if code != 0 {
		t.Fatalf("%d %s%s", code, out, errOut)
	}
	if !strings.Contains(out, "✓ main") || strings.Contains(out, "✓ work") || !strings.Contains(out, ".claude-work (needs a name") {
		t.Fatalf("output:\n%s", out)
	}
	if p, ok := c.cfg().Find("main"); !ok || p.Dir != filepath.Join(c.p.Home, ".claude") {
		t.Fatal("main is not registered")
	}
	if p, _ := c.cfg().Find("work"); p.Dir != "" {
		t.Fatal("the account work was replaced")
	}

	// Two folders that come to one name: the first is imported, the other
	// is skipped with the reason.
	writeFiles(t, filepath.Join(c.p.Home, ".claude-Dev"), map[string]string{"settings.json": "{}"})
	writeFiles(t, filepath.Join(c.p.Home, ".claude-dev"), map[string]string{"settings.json": "{}"})
	if entries, _ := filepath.Glob(filepath.Join(c.p.Home, ".claude-[Dd]ev")); len(entries) < 2 {
		t.Skip("the file system ignores case, so the two are one folder")
	}
	out, errOut, code = c.run("", "import", "--yes")
	if code != 0 || strings.Count(out, "✓ dev") != 1 || !strings.Contains(errOut, `an account named "dev" already exists`) {
		t.Fatalf("%d\n%s%s", code, out, errOut)
	}
}

// --name names the folder given; without one it would quietly name nothing.
func TestImportNameNeedsAFolder(t *testing.T) {
	c := newTestCLI(t, nil)
	writeFiles(t, filepath.Join(c.p.Home, ".claude-2"), map[string]string{"settings.json": "{}"})
	if _, errOut, code := c.run("", "import", "--name", "office", "--yes"); code == 0 || !strings.Contains(errOut, "--name needs a folder") {
		t.Fatalf("%d %s", code, errOut)
	}
	if _, ok := c.cfg().Find("acct2"); ok {
		t.Fatal("the folder was imported under another name")
	}
}

// With a .claude.json of its own, an imported ~/.claude is not the account
// plain claude uses (~/.claude.json): import says so.
func TestImportSaysWhenTheFolderHasItsOwnClaudeJSON(t *testing.T) {
	c := newTestCLI(t, nil)
	dir := filepath.Join(c.p.Home, ".claude")
	writeFiles(t, dir, map[string]string{".claude.json": `{"numStartups":1}`})
	writeFiles(t, c.p.Home, map[string]string{".claude.json": `{"oauthAccount":{"emailAddress":"me@example.com","accountUuid":"u-me"},"mcpServers":{"own":{"command":"x"}}}`})
	out := c.ok("import", dir)
	if !strings.Contains(out, "has a .claude.json of its own") {
		t.Fatalf("no note:\n%s", out)
	}
	if _, err := os.Readlink(filepath.Join(dir, ".claude.json")); err == nil {
		t.Fatal("the folder's own .claude.json was replaced")
	}
	noClaude(t, c)
	if out, _, _ := c.run("", "doctor"); !strings.Contains(out, "has a .claude.json of its own, so `ccam main` and plain `claude`") {
		t.Fatalf("doctor does not say it:\n%s", out)
	}
}

// list shows whom each folder is logged in as now, as Claude Code keeps it
// in .claude.json: a /login inside a session changes it, and so does a
// login at Claude Code's own prompt after `ccam add --no-login`.
func TestListShowsWhomTheFolderIsLoggedInAs(t *testing.T) {
	c := newTestCLI(t, nil)
	cfg := c.cfg()
	p, _ := cfg.Find("work")
	p.Email = "work@example.com" // recorded by an older ccam, without a login record
	cfg.Save(c.p.ConfigFile())
	logIn(t, c.p.ProfileDir("work"), "u9", "acme", "claude_team") // now u9@example.com
	logIn(t, c.p.ProfileDir("side"), "u3", "acme", "claude_team") // never logged in through ccam

	out := c.ok("list")
	for _, want := range []string{"u9@example.com (was work@example.com)", "u3@example.com", "(not logged in)"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	var rows []listRow
	if err := json.Unmarshal([]byte(c.ok("list", "--json")), &rows); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		want := map[string]string{"personal": "", "work": "u9@example.com", "side": "u3@example.com"}[r.Name]
		if r.Email != want {
			t.Errorf("%s: email %q, want %q", r.Name, r.Email, want)
		}
	}
}

// Import records the login as `ccam login` does, so that doctor and the
// next login notice a /login into another account inside a session.
func TestImportRecordsTheLogin(t *testing.T) {
	c := newTestCLI(t, nil)
	importHome(t, c)
	if p, _ := c.cfg().Find("main"); p.Login == nil || p.Login.AccountUUID != "u-me" || p.Email != "me@example.com" {
		t.Fatalf("recorded %+v", p)
	}
}

// A copy of plain claude's ~/.claude (cp -r ~/.claude ~/.claude-2), with
// ~/.claude left to plain claude, holds its login: import, logout and
// doctor say so, as for a copy of another account's folder.
func TestCopyOfPlainClaudesFolderIsNoticed(t *testing.T) {
	c := newTestCLI(t, nil)
	login := `{"claudeAiOauth":{"accessToken":"t","refreshToken":"same","expiresAt":4102444800000}}`
	writeFiles(t, filepath.Join(c.p.Home, ".claude"), map[string]string{".credentials.json": login})
	two := filepath.Join(c.p.Home, ".claude-2")
	writeFiles(t, two, map[string]string{".credentials.json": login})
	if out := c.ok("import", two, "--name", "two"); !strings.Contains(out, "same login as plain claude (~/.claude)") {
		t.Errorf("import: %s", out)
	}
	if out, _, _ := c.run("n\n", "logout", "two"); !strings.Contains(out, "plain claude (~/.claude) holds the same login") || !strings.Contains(out, "Cancelled.") {
		t.Errorf("logout: %s", out)
	}
	if out, _, _ := c.run("", "doctor"); !strings.Contains(out, "same login token as plain claude (~/.claude)") {
		t.Errorf("doctor: %s", out)
	}
}

func TestImportCandidatesListsWithoutImporting(t *testing.T) {
	c := newTestCLI(t, nil)
	dir := filepath.Join(c.p.Home, ".claude")
	writeFiles(t, dir, homeClaude)
	var rows []struct{ Path, Name string }
	if err := json.Unmarshal([]byte(c.ok("import", "--candidates")), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Path != dir || rows[0].Name == "" {
		t.Fatalf("candidates = %+v", rows)
	}
	c.ok("import", dir)
	if out := strings.TrimSpace(c.ok("import", "--candidates")); out != "[]" {
		t.Fatalf("after import: %s", out)
	}
}
