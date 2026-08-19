package engine

import "testing"

func TestCompareDEBRealCases(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.191~", "1.187.2~", 1},
		{"1.191~", "1.191", -1},
		{"1.5r6", "1.5.0", 1},
		{"12.3", "12.1.1~", 1},
		{"2.16.0-0ubuntu2", "2.16.0", 1},
	}
	for _, c := range cases {
		if got := compareDEB(c.a, c.b); got != c.want {
			t.Errorf("compareDEB(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestCompareRPMEdge(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.10", -1},   // 10 > 3 numerically
		{"8.5-1.el8", "8.5-1", 1}, // "release" 1.el8 > 1
		{"1.0-1", "1.0-2", -1},
	}
	for _, c := range cases {
		if got := compareRPM(c.a, c.b); got != c.want {
			t.Errorf("compareRPM(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
