package memory

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	m.AddHistory("command", "find-file")
	m.AddHistory("command", "dired")
	m.AddHistory("command", "find-file") // moves to the front, once
	m.AddRecent("/a.go")
	m.AddRecent("/b.go")
	m.SetPlace("/a.go", Place{Line: 10, Col: 2, Top: 4})
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	back, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := back.HistoryOf("command"); !slices.Equal(got, []string{"find-file", "dired"}) {
		t.Errorf("history %q", got)
	}
	if got := back.Recent; !slices.Equal(got, []string{"/b.go", "/a.go"}) {
		t.Errorf("recent %q", got)
	}
	if p, ok := back.PlaceOf("/a.go"); !ok || p.Line != 10 || p.Col != 2 || p.Top != 4 {
		t.Errorf("place %+v, %v", p, ok)
	}
	if fi, err := os.Stat(filepath.Join(dir, FileName)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("memory file mode %v (%v), want 0600", fi.Mode().Perm(), err)
	}
}

// Two sessions saving in turn keep each other's history: the later one's
// entries first, then the earlier one's.
func TestSaveMerges(t *testing.T) {
	dir := t.TempDir()
	a, _ := Load(dir)
	b, _ := Load(dir)

	a.AddHistory("file", "a.txt")
	a.SetPlace("/x", Place{Line: 1, Used: 100})
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	b.AddHistory("file", "b.txt")
	b.SetPlace("/x", Place{Line: 2, Used: 50}) // older than a's
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}

	got, _ := Load(dir)
	if h := got.HistoryOf("file"); !slices.Equal(h, []string{"b.txt", "a.txt"}) {
		t.Errorf("merged history %q", h)
	}
	if p, _ := got.PlaceOf("/x"); p.Line != 1 {
		t.Errorf("merged place line %d, want the more recent 1", p.Line)
	}
}

// A memory file that cannot be read is set aside, and nem starts with an
// empty memory rather than refusing to start.
func TestUnreadableFileIsSetAside(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := Load(dir)
	if err != nil || len(m.HistoryOf("x")) != 0 {
		t.Fatalf("Load = %+v, %v", m, err)
	}
	if _, err := os.Stat(path + ".bad"); err != nil {
		t.Errorf("the unreadable file was not kept aside: %v", err)
	}
}

// Histories are bounded, and a zero Memory remembers within a session but
// writes nothing.
func TestLimits(t *testing.T) {
	var m Memory
	for i := range historySize + 20 {
		m.AddHistory("k", string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	if n := len(m.HistoryOf("k")); n != historySize {
		t.Errorf("history holds %d, want %d", n, historySize)
	}
	if err := m.Save(); err != nil {
		t.Errorf("saving an unpersisted memory: %v", err)
	}
	for i := range placesSize + 5 {
		m.SetPlace(string(rune(i)), Place{Used: int64(i + 1)})
	}
	if len(m.Places) != placesSize {
		t.Errorf("%d places kept, want %d", len(m.Places), placesSize)
	}
	if _, ok := m.PlaceOf(string(rune(0))); ok {
		t.Error("the oldest place was kept over newer ones")
	}
}

// Projects are kept most recent first and merged like the recent files, and
// one forgotten here is not brought back by another session's copy.
func TestProjects(t *testing.T) {
	dir := t.TempDir()
	a, _ := Load(dir)
	a.AddProject("/p")
	a.AddProject("/q")
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}

	b, _ := Load(dir)
	b.AddProject("/r")
	b.ForgetProject("/q")
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	got, _ := Load(dir)
	if !slices.Equal(got.Projects, []string{"/r", "/p"}) {
		t.Errorf("projects %q, want /r then /p, without the forgotten /q", got.Projects)
	}

	got.AddProject("/q")
	if !slices.Equal(got.Projects, []string{"/q", "/r", "/p"}) {
		t.Errorf("projects %q after adding /q back", got.Projects)
	}
}
