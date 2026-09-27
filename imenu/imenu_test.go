package imenu

import (
	"strings"
	"testing"
)

func names(es []Entry) string {
	var out []string
	for _, e := range es {
		out = append(out, e.Name)
	}
	return strings.Join(out, " | ")
}

func TestFor(t *testing.T) {
	for _, tc := range []struct {
		path, text, want string
	}{
		{"x.go", `package x

// func notThis() in a comment
type Buffer struct{}

func New() *Buffer {
	return nil
}

func (b *Buffer) Insert(s string) {}
func (Buffer) String() string { return "" }`,
			"type Buffer | func New | (*Buffer) Insert | (Buffer) String"},
		{"app.py", `class Shape:
    def area(self):
        return f(x)

async def main():
    pass`,
			"class Shape | def area | def main"},
		{"app.ts", `export class Store {
  constructor(x) {
    if (x) {
    }
  }
  async load(id: string): Promise<void> {
  }
}
export function helper(a) {}
const handler = async (req) => {}
export type Id = string
interface Props {}`,
			"class Store | constructor | load | function helper | handler | type Id | interface Props"},
		{"lib.rs", `pub struct Point { x: i32 }
impl Point {
    pub fn new() -> Self {}
}
pub(crate) async fn run() {}
enum Kind {}`,
			"struct Point | impl Point | fn new | fn run | enum Kind"},
		{"main.c", `#define MAX 10
struct node {
static int helper(int a,
                  int b)
int main(int argc, char **argv) {
    if (argc) {
        foo(1,
    }
}`,
			"MAX | struct node | helper | main"},
		{"README.md", "# Title\n\n## Usage\n```\n# not a heading\n```\n### Deep ###", "Title |   Usage |     Deep"},
		{"Makefile", "all: build\n\tgo build\nbuild:\n.PHONY: all\nX := 1", "all | build | .PHONY"},
		{"notes.txt", "func main() {}", ""},
	} {
		if got := names(For(tc.path, strings.Split(tc.text, "\n"))); got != tc.want {
			t.Errorf("For(%s) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestEntriesKeepTheirLines(t *testing.T) {
	es := For("a.go", []string{"package a", "", "func One() {}", "", "func Two() {}"})
	if len(es) != 2 || es[0].Line != 2 || es[1].Line != 4 {
		t.Errorf("entries %+v", es)
	}
}
