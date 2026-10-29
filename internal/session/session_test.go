package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

func TestRegisterAndActive(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "s")
	if Active(dir) {
		t.Fatal("empty dir is not active")
	}
	if err := Register(dir); err != nil {
		t.Fatal(err)
	}
	if !Active(dir) {
		t.Fatal("this process should count as running")
	}
}

func TestStaleMarkersAreRemoved(t *testing.T) {
	dir := t.TempDir()
	// A finished process.
	self, _ := os.Executable()
	c := exec.Command(self, "-test.run=^$") // this test binary, running no tests
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	dead := filepath.Join(dir, strconv.Itoa(c.Process.Pid))
	os.WriteFile(dead, []byte("1"), 0o600)
	// A live PID with the wrong start time: the PID was reused.
	reused := filepath.Join(dir, strconv.Itoa(os.Getpid()))
	os.WriteFile(reused, []byte("0"), 0o600)
	if Active(dir) {
		t.Fatal("stale markers must not count")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("stale markers left: %v", entries)
	}
}

func TestStartTimeHandlesSpacesInName(t *testing.T) {
	if _, ok := StartTime(os.Getpid()); !ok {
		t.Fatal("own start time")
	}
	if _, ok := StartTime(-1); ok {
		t.Fatal("bogus pid")
	}
}

func TestParentAndExe(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("no process tree here")
	}
	if p, ok := Parent(os.Getpid()); !ok || p != os.Getppid() {
		t.Fatalf("parent %d %v, want %d", p, ok, os.Getppid())
	}
	self, _ := os.Executable()
	self, _ = filepath.EvalSymlinks(self)
	if exe, ok := Exe(os.Getpid()); !ok || exe != self {
		t.Fatalf("exe %q %v, want %q", exe, ok, self)
	}
	c := exec.Command(self, "-test.run=^$")
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	if _, ok := Parent(c.Process.Pid); ok {
		t.Fatal("the parent of a finished process")
	}
	if _, ok := Exe(c.Process.Pid); ok {
		t.Fatal("the program of a finished process")
	}
}

func TestInScopeKeepsProjectAndUnknownFolders(t *testing.T) {
	procs := []Process{{PID: 2, Cwd: "/p"}, {PID: 3, Cwd: "/other"}, {PID: 4}, {PID: 5, Cwd: "/p/"}}
	got := InScope(procs, "/p")
	if len(got) != 3 || got[0].PID != 2 || got[1].PID != 4 || got[2].PID != 5 {
		t.Fatalf("InScope = %+v", got)
	}
}
