package timefmt

import "testing"

func TestShort(t *testing.T) {
	cases := []struct {
		sec  int64
		want string
	}{
		{-5, "0 s"}, {0, "0 s"}, {45, "45 s"}, {59, "59 s"},
		{60, "1 min"}, {719, "11 min"}, {3599, "59 min"},
		{3600, "1 h"}, {18000, "5 h"}, {86399, "23 h"},
		{86400, "1 d"}, {19 * 86400, "19 d"},
	}
	for _, c := range cases {
		if got := Short(c.sec); got != c.want {
			t.Errorf("Short(%d) = %q, want %q", c.sec, got, c.want)
		}
	}
}

func TestExact(t *testing.T) {
	cases := []struct {
		sec  int64
		want string
	}{
		{0, "0 s"}, {-1, "0 s"}, {45, "45 s"}, {60, "1 min"},
		{3852, "1 h 4 min 12 s"}, {3600, "1 h"},
		{86400 + 3600 + 5, "1 d 1 h 5 s"},
	}
	for _, c := range cases {
		if got := Exact(c.sec); got != c.want {
			t.Errorf("Exact(%d) = %q, want %q", c.sec, got, c.want)
		}
	}
}
