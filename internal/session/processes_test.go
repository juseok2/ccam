package session

import "testing"

func TestForSession(t *testing.T) {
	all := []Process{{PID: 2, Session: "a"}, {PID: 3, Session: "b"}, {PID: 4}}
	got := ForSession(all, "a")
	if len(got) != 2 || got[0].PID != 2 || got[1].PID != 4 {
		t.Fatal("keep the conversation's process and those recording none:", got)
	}
	if len(ForSession(nil, "a")) != 0 {
		t.Fatal("nil list")
	}
}

func TestForSessionLeavesTheDaemon(t *testing.T) {
	got := ForSession([]Process{{PID: 2, Daemon: true}, {PID: 3}}, "a")
	if len(got) != 1 || got[0].PID != 3 {
		t.Fatal("the daemon must not be stopped for one conversation:", got)
	}
}
