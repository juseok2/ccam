package transfer

import (
	"os"
	"path/filepath"
	"testing"
)

const testID = "12345678-1234-1234-1234-123456789abc"
const project = "-tmp-project"

func put(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0600); err != nil {
		t.Fatal(err)
	}
}
func fixture(t *testing.T) (string, string, string) {
	t.Helper()
	base := t.TempDir()
	src := filepath.Join(base, "src")
	dst := filepath.Join(base, "dst")
	backup := filepath.Join(base, "backups")
	put(t, filepath.Join(src, "projects", project, testID+".jsonl"), `{"type":"user","sessionId":"`+testID+`","message":{"content":"hello"}}`+"\n")
	put(t, filepath.Join(src, "projects", project, testID, "subagents", "agent-a.jsonl"), `{"type":"assistant","sessionId":"`+testID+`","message":{"content":"answer"}}`+"\n")
	put(t, filepath.Join(src, ".credentials.json"), "source-secret")
	put(t, filepath.Join(dst, ".credentials.json"), "target-secret")
	return src, dst, backup
}
func TestCopyAndVerify(t *testing.T) {
	src, dst, backups := fixture(t)
	before, err := Snapshot(src, project, testID)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := Copy(src, dst, backups, project, testID, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(backup); err != nil {
		t.Fatal(err)
	}
	after, err := Snapshot(src, project, testID)
	if err != nil || !same(before, after) {
		t.Fatal("source changed", err)
	}
	target, err := Snapshot(dst, project, testID)
	if err != nil || !same(before, target) {
		t.Fatal("target differs", err)
	}
	for root, want := range map[string]string{src: "source-secret", dst: "target-secret"} {
		b, _ := os.ReadFile(filepath.Join(root, ".credentials.json"))
		if string(b) != want {
			t.Fatal("credentials changed")
		}
	}
	if _, err := Copy(src, dst, backups, project, testID, false); err == nil {
		t.Fatal("overwrote target")
	}
	put(t, filepath.Join(backup, "projects", project, testID+".jsonl"), `{"tampered":true}`)
	if Verify(backup) == nil {
		t.Fatal("missed tampering")
	}
}
func TestDryRun(t *testing.T) {
	src, dst, backups := fixture(t)
	if _, err := Copy(src, dst, backups, project, testID, true); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{backups, filepath.Join(dst, "projects")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("dry run wrote %s", p)
		}
	}
}
func TestRejectInvalidInput(t *testing.T) {
	for _, tc := range []struct{ name, project, id string }{
		{"traversal", "../outside", testID}, {"id traversal", project, "../secret"}, {"empty", "", testID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, dst, b := fixture(t)
			if _, err := Copy(src, dst, b, tc.project, tc.id, false); err == nil {
				t.Fatal("accepted invalid input")
			}
		})
	}
}
func TestRejectBadTranscript(t *testing.T) {
	for _, data := range []string{"", `{"partial":`, "null", `{}`, `{"sessionId":"wrong"}`} {
		t.Run(data, func(t *testing.T) {
			src, dst, b := fixture(t)
			put(t, filepath.Join(src, "projects", project, testID+".jsonl"), data)
			if _, err := Copy(src, dst, b, project, testID, false); err == nil {
				t.Fatal("accepted bad transcript")
			}
			if _, err := os.Stat(b); !os.IsNotExist(err) {
				t.Fatal("created backup on invalid input")
			}
		})
	}
}
func TestRejectSymlinks(t *testing.T) {
	for _, target := range []bool{false, true} {
		t.Run(map[bool]string{false: "source", true: "target"}[target], func(t *testing.T) {
			src, dst, b := fixture(t)
			root := src
			if target {
				root = dst
			}
			p := filepath.Join(root, "projects")
			if err := os.RemoveAll(p); err != nil {
				t.Fatal(err)
			}
			outside := t.TempDir()
			if err := os.Symlink(outside, p); err != nil {
				t.Skip(err)
			}
			if _, err := Copy(src, dst, b, project, testID, false); err == nil {
				t.Fatal("followed symlink")
			}
		})
	}
}
func TestSidecarSymlink(t *testing.T) {
	src, dst, b := fixture(t)
	p := filepath.Join(src, "projects", project, testID, "secret")
	if err := os.Symlink(filepath.Join(src, ".credentials.json"), p); err != nil {
		t.Skip(err)
	}
	if _, err := Copy(src, dst, b, project, testID, false); err == nil {
		t.Fatal("copied symlink")
	}
}
func TestAtomicNoClobber(t *testing.T) {
	p := filepath.Join(t.TempDir(), "file")
	put(t, p, "original")
	if writeNew(p, []byte("changed")) == nil {
		t.Fatal("overwrote")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "original" {
		t.Fatal("changed")
	}
}
