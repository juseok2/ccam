package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Process identifies one instance, not just a potentially reused PID.
type Process struct {
	PID   int    `json:"pid"`
	Start string `json:"start"`
	Cwd   string `json:"cwd"`
}

func Processes(markers, dir string) []Process {
	found := map[int]Process{}
	entries, _ := os.ReadDir(markers)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 1 || pid == os.Getpid() {
			continue
		}
		path := filepath.Join(markers, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxRecord {
			continue
		}
		want, err := os.ReadFile(path)
		now, ok := StartTime(pid)
		if err == nil && ok && now == string(want) {
			found[pid] = Process{PID: pid, Start: now}
		}
	}
	paths := []string{filepath.Join(dir, "daemon.lock")}
	entries, _ = os.ReadDir(filepath.Join(dir, "sessions"))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			paths = append(paths, filepath.Join(dir, "sessions", entry.Name()))
		}
	}
	for _, path := range paths {
		if !running(path) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var row struct {
			PID int    `json:"pid"`
			Cwd string `json:"cwd"`
		}
		if json.Unmarshal(data, &row) != nil || row.PID == os.Getpid() {
			continue
		}
		if now, ok := StartTime(row.PID); ok {
			found[row.PID] = Process{PID: row.PID, Start: now, Cwd: row.Cwd}
		}
	}
	result := []Process{}
	for _, p := range found {
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PID < result[j].PID })
	return result
}

// InScope keeps the processes that may write a conversation of project cwd:
// those running in cwd, and those whose folder is unknown (a launcher marker,
// the daemon), which are always treated as in scope.
func InScope(procs []Process, cwd string) []Process {
	out := []Process{}
	for _, p := range procs {
		if p.Cwd == "" || filepath.Clean(p.Cwd) == filepath.Clean(cwd) {
			out = append(out, p)
		}
	}
	return out
}
