// Package memory is what nem remembers from one session to the next: what was
// typed at each kind of prompt, the files opened most recently, where point
// was in each file when it was left, and the projects worked in.
//
// It is one small JSON file in the state directory, beside the backups. Saving
// merges with what is on disk rather than overwriting it, so two nem windows
// open at once add to one history instead of each erasing the other's.
package memory

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// FileName is the memory's file, in the state directory.
const FileName = "memory.json"

// How much is kept. A history longer than this is not scrolled through; a
// place in a file untouched for this many other files is not missed.
const (
	historySize  = 100
	recentSize   = 200
	placesSize   = 1000
	projectsSize = 100
)

// Place is where point was in a file, and the first line on screen.
type Place struct {
	Line int `json:"line"`
	Col  int `json:"col"`
	Top  int `json:"top"`
	// Used is when the place was recorded, in Unix seconds: the most recent
	// wins a merge, and the oldest go first when there are too many.
	Used int64 `json:"used"`
}

// Memory is the remembered state. The zero value is empty and does not
// persist; Load gives one that does.
type Memory struct {
	path string

	History  map[string][]string `json:"history"`
	Recent   []string            `json:"recent"`
	Places   map[string]Place    `json:"places"`
	Projects []string            `json:"projects"`

	// forgotten are projects forgotten this session, kept so that saving
	// does not bring them back from the copy on disk.
	forgotten map[string]bool
}

// Load reads the memory kept in dir, or starts an empty one there if there is
// none yet. A file that cannot be parsed is set aside rather than trusted or
// deleted: losing a history is a nuisance, but it is not worth an error that
// stops the editor starting.
func Load(dir string) (*Memory, error) {
	m := &Memory{path: filepath.Join(dir, FileName)}
	disk, err := read(m.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return m, nil
	case err != nil:
		_ = os.Rename(m.path, m.path+".bad")
		return m, nil
	}
	m.History, m.Recent, m.Places, m.Projects = disk.History, disk.Recent, disk.Places, disk.Projects
	return m, nil
}

func read(path string) (*Memory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Memory
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// AddHistory records s as the most recent entry at prompts of this kind.
func (m *Memory) AddHistory(kind, s string) {
	if kind == "" || s == "" {
		return
	}
	if m.History == nil {
		m.History = map[string][]string{}
	}
	m.History[kind] = pushFront(m.History[kind], s, historySize)
}

// HistoryOf is what has been entered at prompts of this kind, most recent
// first.
func (m *Memory) HistoryOf(kind string) []string { return m.History[kind] }

// AddRecent records path as the most recently opened file.
func (m *Memory) AddRecent(path string) {
	m.Recent = pushFront(m.Recent, path, recentSize)
}

// AddProject records root as the project most recently worked in.
func (m *Memory) AddProject(root string) {
	m.Projects = pushFront(m.Projects, root, projectsSize)
	delete(m.forgotten, root)
}

// ForgetProject takes root off the known projects.
func (m *Memory) ForgetProject(root string) {
	m.Projects = slices.DeleteFunc(m.Projects, func(p string) bool { return p == root })
	if m.forgotten == nil {
		m.forgotten = map[string]bool{}
	}
	m.forgotten[root] = true
}

// SetPlace records where point was in the file at path.
func (m *Memory) SetPlace(path string, p Place) {
	if m.Places == nil {
		m.Places = map[string]Place{}
	}
	if p.Used == 0 {
		p.Used = time.Now().Unix()
	}
	m.Places[path] = p
	trimPlaces(m.Places)
}

// PlaceOf is where point was left in the file at path.
func (m *Memory) PlaceOf(path string) (Place, bool) {
	p, ok := m.Places[path]
	return p, ok
}

// Save writes the memory, merged with whatever another session saved since
// this one loaded: this session's entries first, since they are the newer,
// then the other's that this one does not have. A memory that was not loaded
// from a directory is not saved anywhere.
func (m *Memory) Save() error {
	if m.path == "" {
		return nil
	}
	if disk, err := read(m.path); err == nil {
		for kind, entries := range disk.History {
			for _, s := range entries {
				if !slices.Contains(m.History[kind], s) {
					if m.History == nil {
						m.History = map[string][]string{}
					}
					m.History[kind] = appendCapped(m.History[kind], s, historySize)
				}
			}
		}
		for _, p := range disk.Recent {
			if !slices.Contains(m.Recent, p) {
				m.Recent = appendCapped(m.Recent, p, recentSize)
			}
		}
		for _, p := range disk.Projects {
			if !slices.Contains(m.Projects, p) && !m.forgotten[p] {
				m.Projects = appendCapped(m.Projects, p, projectsSize)
			}
		}
		for path, p := range disk.Places {
			if mine, ok := m.Places[path]; !ok || p.Used > mine.Used {
				if m.Places == nil {
					m.Places = map[string]Place{}
				}
				m.Places[path] = p
			}
		}
		trimPlaces(m.Places)
	}

	data, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return err
	}
	// Written beside the old file and renamed over it, so a crash mid-write
	// leaves the old memory rather than half a new one. Private: the history
	// holds whatever was typed at a prompt.
	tmp, err := os.CreateTemp(filepath.Dir(m.path), ".memory-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), m.path)
}

// pushFront puts s at the front of list, removing any older copy, and keeps
// at most limit entries.
func pushFront(list []string, s string, limit int) []string {
	out := make([]string, 0, min(len(list)+1, limit))
	out = append(out, s)
	for _, x := range list {
		if x != s && len(out) < limit {
			out = append(out, x)
		}
	}
	return out
}

// appendCapped appends s unless list already holds limit entries.
func appendCapped(list []string, s string, limit int) []string {
	if len(list) >= limit {
		return list
	}
	return append(list, s)
}

// trimPlaces drops the least recently used places beyond placesSize.
func trimPlaces(places map[string]Place) {
	if len(places) <= placesSize {
		return
	}
	type used struct {
		path string
		at   int64
	}
	all := make([]used, 0, len(places))
	for p, pl := range places {
		all = append(all, used{p, pl.Used})
	}
	slices.SortFunc(all, func(a, b used) int { return int(b.at - a.at) })
	for _, u := range all[placesSize:] {
		delete(places, u.path)
	}
}
