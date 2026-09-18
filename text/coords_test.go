package text

import "testing"

func TestPosOrdering(t *testing.T) {
	tests := []struct {
		name                 string
		p, q                 Pos
		before, after, equal bool
	}{
		{"same", Pos{1, 5}, Pos{1, 5}, false, false, true},
		{"earlier line", Pos{0, 9}, Pos{1, 0}, true, false, false},
		{"later line", Pos{2, 0}, Pos{1, 9}, false, true, false},
		{"same line earlier col", Pos{1, 2}, Pos{1, 7}, true, false, false},
		{"same line later col", Pos{1, 7}, Pos{1, 2}, false, true, false},
		{"zero vs zero", Pos{}, Pos{}, false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Before(tt.q); got != tt.before {
				t.Errorf("Before() = %v, want %v", got, tt.before)
			}
			if got := tt.p.After(tt.q); got != tt.after {
				t.Errorf("After() = %v, want %v", got, tt.after)
			}
			if got := tt.p.Equal(tt.q); got != tt.equal {
				t.Errorf("Equal() = %v, want %v", got, tt.equal)
			}
		})
	}
}

func TestPosOrderingIsTotal(t *testing.T) {
	// exactly one of Before/After/Equal must hold for any pair
	ps := []Pos{{0, 0}, {0, 1}, {1, 0}, {1, 5}, {2, 3}}
	for _, p := range ps {
		for _, q := range ps {
			n := 0
			if p.Before(q) {
				n++
			}
			if p.After(q) {
				n++
			}
			if p.Equal(q) {
				n++
			}
			if n != 1 {
				t.Errorf("p=%v q=%v: %d of Before/After/Equal held, want exactly 1", p, q, n)
			}
		}
	}
}

func TestMinMaxPos(t *testing.T) {
	a, b := Pos{1, 2}, Pos{3, 4}
	if got := MinPos(a, b); !got.Equal(a) {
		t.Errorf("MinPos(a,b) = %v, want %v", got, a)
	}
	if got := MinPos(b, a); !got.Equal(a) {
		t.Errorf("MinPos(b,a) = %v, want %v", got, a)
	}
	if got := MaxPos(a, b); !got.Equal(b) {
		t.Errorf("MaxPos(a,b) = %v, want %v", got, b)
	}
	if got := MaxPos(b, a); !got.Equal(b) {
		t.Errorf("MaxPos(b,a) = %v, want %v", got, b)
	}
}
