package main

import (
	"strings"
	"testing"
)

func TestD29StripCheck(t *testing.T) {
	cases := []struct {
		name string
		p    d29Page
		want string
	}{
		{"whole strip", d29Page{StripClip: 0, StripRows: 4, StripWhole: 4}, ""},
		{"cap cuts the last row", d29Page{StripClip: 15, StripRows: 4, StripWhole: 3}, "clipped by 15 px"},
		{"fit pass shrank, two rows whole", d29Page{StripShrunk: true, StripClip: 15, StripRows: 4, StripWhole: 2}, ""},
		{"fit pass shrank below the floor", d29Page{StripShrunk: true, StripClip: 60, StripRows: 4, StripWhole: 1}, "below its floor"},
		{"one row only, cut", d29Page{StripShrunk: true, StripClip: 10, StripRows: 1, StripWhole: 0}, "below its floor"},
		{"one row only, whole", d29Page{StripShrunk: true, StripRows: 1, StripWhole: 1}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := d29StripCheck(c.p)
			if c.want == "" {
				if err != nil {
					t.Fatalf("want pass, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want an error with %q, got %v", c.want, err)
			}
		})
	}
}
