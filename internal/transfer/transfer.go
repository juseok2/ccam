// Package transfer copies a stopped session without copying account credentials.
package transfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var sessionID = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

const maxBytes = 128 << 20

type Entry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int    `json:"size"`
	data   []byte
}
type Manifest struct {
	Version int     `json:"version"`
	Session string  `json:"session"`
	Project string  `json:"project"`
	Files   []Entry `json:"files"`
}

// plain rejects symlinks in every existing component, including shared projects.
// Callers must stop Claude before transfer; this is not a lock on other programs.
func plain(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for {
		info, err := os.Lstat(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink not supported: %s", path)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}

// canonicalRoot resolves an explicitly selected root (including macOS /var).
// Symlinks below this boundary are still rejected.
func canonicalRoot(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(path)
	if err == nil {
		return real, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	real, err = canonicalRoot(parent)
	return filepath.Join(real, filepath.Base(path)), err
}

// ValidID reports whether id has the shape of a Claude Code session UUID.
func ValidID(id string) bool { return sessionID.MatchString(id) }

// projectsRoot is root's projects folder, resolved once when it is a link:
// ccam shares conversation history by linking projects/ to a group folder.
// Links below it are still rejected.
func projectsRoot(root string) (string, error) {
	dir := filepath.Join(root, "projects")
	info, err := os.Lstat(dir)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return dir, nil
	}
	return filepath.EvalSymlinks(dir)
}

// Shared reports whether two profile folders read the same projects folder,
// so a conversation of one is already the other's and nothing is copied.
func Shared(a, b string) bool {
	x, err := projectsRoot(a)
	if err != nil {
		return false
	}
	y, err := projectsRoot(b)
	if err != nil {
		return false
	}
	ix, err := os.Stat(x)
	if err != nil {
		return false
	}
	iy, err := os.Stat(y)
	return err == nil && os.SameFile(ix, iy)
}

// Locate checks that root holds conversation id of project as a regular
// file, without the copy checks Snapshot makes: enough to resume it.
func Locate(root, project, id string) error {
	if !validComponent(project) || !ValidID(id) {
		return fmt.Errorf("invalid project folder or session UUID")
	}
	root, err := canonicalRoot(root)
	if err != nil {
		return err
	}
	projects, err := projectsRoot(root)
	if err != nil {
		return err
	}
	info, err := os.Lstat(filepath.Join(projects, project, id+".jsonl"))
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", id+".jsonl")
	}
	return nil
}

func digest(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func validComponent(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, `/\`)
}

// Snapshot reads only the selected transcript and its session sidecar directory.
// All JSONL is validated, but no content (including thinking/signatures) is edited.
func Snapshot(root, project, id string) (*Manifest, error) {
	if !validComponent(project) || !sessionID.MatchString(id) {
		return nil, fmt.Errorf("invalid project folder or session UUID")
	}
	root, err := canonicalRoot(root)
	if err != nil {
		return nil, err
	}
	projects, err := projectsRoot(root)
	if err != nil {
		return nil, err
	}
	base := filepath.Join(projects, project)
	if err := plain(base); err != nil {
		return nil, err
	}
	m := &Manifest{Version: 1, Session: id, Project: project}
	total := int64(0)
	add := func(path string) error {
		if err := plain(path); err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("not a regular file: %s", path)
		}
		total += info.Size()
		if total > maxBytes {
			return fmt.Errorf("session exceeds 128 MiB MVP limit")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if int64(len(data)) != info.Size() {
			return fmt.Errorf("session changed while reading; stop Claude first")
		}
		if strings.HasSuffix(path, ".jsonl") {
			lines := bytes.Split(data, []byte("\n"))
			count := 0
			conversation := false
			for i, line := range lines {
				if len(bytes.TrimSpace(line)) == 0 {
					continue
				}
				var row map[string]json.RawMessage
				if json.Unmarshal(line, &row) != nil || row == nil {
					return fmt.Errorf("invalid JSONL at %s line %d", filepath.Base(path), i+1)
				}
				if raw, ok := row["sessionId"]; ok {
					var sid string
					if json.Unmarshal(raw, &sid) != nil || sid != id {
						return fmt.Errorf("session ID mismatch in %s", filepath.Base(path))
					}
				}
				if string(row["type"]) == `"user"` || string(row["type"]) == `"assistant"` {
					if _, ok := row["sessionId"]; ok {
						conversation = true
					}
				}
				count++
			}
			if filepath.Base(path) == id+".jsonl" && !conversation {
				return fmt.Errorf("no conversation with matching session ID")
			}
			if count == 0 {
				return fmt.Errorf("empty transcript: %s", filepath.Base(path))
			}
		}
		rel, _ := filepath.Rel(base, path)
		m.Files = append(m.Files, Entry{Path: rel, SHA256: digest(data), Size: len(data), data: data})
		return nil
	}
	if err := add(filepath.Join(base, id+".jsonl")); err != nil {
		return nil, err
	}
	side := filepath.Join(base, id)
	if info, err := os.Lstat(side); err == nil {
		if !info.IsDir() {
			return nil, fmt.Errorf("session sidecar is not a directory")
		}
		if err := plain(side); err != nil {
			return nil, err
		}
		err = filepath.WalkDir(side, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			return add(path)
		})
		if err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return m, nil
}

func same(a, b *Manifest) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

// Copy keeps a verified backup before publication, never overwrites a target,
// and publishes the transcript last. A failed publication removes only files
// created by this call. Backups survive failures for manual recovery.
func Copy(src, dst, backups, project, id string, dry bool) (backup string, err error) {
	src, err = canonicalRoot(src)
	if err != nil {
		return "", err
	}
	dst, err = canonicalRoot(dst)
	if err != nil {
		return "", err
	}
	backups, err = canonicalRoot(backups)
	if err != nil {
		return "", err
	}
	if Shared(src, dst) {
		return "", fmt.Errorf("both accounts share conversation history; nothing to copy")
	}
	m, err := Snapshot(src, project, id)
	if err != nil {
		return "", err
	}
	if err = plain(dst); err != nil {
		return "", err
	}
	projects, err := projectsRoot(dst)
	if err != nil {
		return "", err
	}
	dest := filepath.Join(projects, project)
	if err = plain(dest); err != nil {
		return "", err
	}
	for _, name := range []string{id + ".jsonl", id} {
		_, e := os.Lstat(filepath.Join(dest, name))
		if e == nil {
			return "", fmt.Errorf("target session already exists; nothing overwritten")
		}
		if !os.IsNotExist(e) {
			return "", e
		}
	}
	if dry {
		return "", nil
	}
	if err = plain(backups); err != nil {
		return "", err
	}
	if err = os.MkdirAll(backups, 0700); err != nil {
		return "", err
	}
	backup, err = os.MkdirTemp(backups, "session-")
	if err != nil {
		return "", err
	}
	for _, entry := range m.Files {
		path := filepath.Join(backup, "projects", project, entry.Path)
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return backup, err
		}
		if err = writeNew(path, entry.data); err != nil {
			return backup, err
		}
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	if err = writeNew(filepath.Join(backup, "manifest.json"), data); err != nil {
		return backup, err
	}
	if err = Verify(backup); err != nil {
		return backup, err
	}
	// Detect an append or sidecar change during backup, before touching the target.
	now, e := Snapshot(src, project, id)
	if e != nil {
		return backup, e
	}
	if !same(m, now) {
		return backup, fmt.Errorf("source changed; stop Claude and retry")
	}
	if err = os.MkdirAll(dest, 0700); err != nil {
		return backup, err
	}
	var created []string
	sideCreated := false
	defer func() {
		if err != nil {
			for _, p := range created {
				_ = os.Remove(p)
			}
			if sideCreated {
				_ = os.RemoveAll(filepath.Join(dest, id))
			}
		}
	}()
	if len(m.Files) > 1 {
		if err = os.Mkdir(filepath.Join(dest, id), 0700); err != nil {
			return backup, err
		}
		sideCreated = true
	}
	// Sidecars first. The main transcript is the commit marker.
	order := append(append([]Entry{}, m.Files[1:]...), m.Files[0])
	for _, entry := range order {
		path := filepath.Join(dest, entry.Path)
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return backup, err
		}
		if err = writeNew(path, entry.data); err != nil {
			return backup, err
		}
		created = append(created, path)
	}
	copied, e := Snapshot(dst, project, id)
	if e != nil {
		return backup, e
	}
	if !same(m, copied) {
		return backup, fmt.Errorf("target verification failed")
	}
	return backup, nil
}

// Link publishes a complete synced file atomically and fails if the target exists.
func writeNew(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".ccam-transfer-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Link(f.Name(), path)
}

// Verify checks every byte of a backup against its manifest, including sidecars.
func Verify(backup string) error {
	backup, err := canonicalRoot(backup)
	if err != nil {
		return err
	}
	if err := plain(filepath.Join(backup, "manifest.json")); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(backup, "manifest.json"))
	if err != nil {
		return err
	}
	var expected Manifest
	if err = json.Unmarshal(data, &expected); err != nil {
		return err
	}
	if expected.Version != 1 {
		return fmt.Errorf("unsupported backup version")
	}
	actual, err := Snapshot(backup, expected.Project, expected.Session)
	if err != nil {
		return err
	}
	if !same(&expected, actual) {
		return fmt.Errorf("backup checksum mismatch")
	}
	return nil
}
