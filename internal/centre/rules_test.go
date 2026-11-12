package centre

import (
	"reflect"
	"testing"
)

func flowKeys(rows []FlowRow) []string {
	keys := make([]string, len(rows))
	for i, r := range rows {
		keys[i] = r.Key
	}
	return keys
}

func TestSortFlowsRule(t *testing.T) {
	flow := func(k string, waiting, closed bool, updated string) FlowRow {
		return FlowRow{Key: k, Waiting: waiting, Closed: closed, UpdatedAt: updated}
	}
	rows := []FlowRow{
		flow("a/closed-new", false, true, "2026-11-10T00:00:00Z"),
		flow("a/open-untimed", false, false, ""),
		flow("a/open-old", false, false, "2026-10-01T00:00:00Z"),
		flow("b/wait-untimed", true, false, ""),
		flow("a/wait-untimed", true, false, "garbage"),
		flow("a/wait-old", true, false, "2026-11-01T00:00:00Z"),
		flow("a/wait-new", true, false, "2026-11-09T00:00:00Z"),
		flow("b/open-new", false, false, "2026-11-08T00:00:00Z"),
		flow("a/open-new", false, false, "2026-11-08T00:00:00Z"),
		flow("a/closed-old", false, true, "2026-09-01T00:00:00Z"),
	}
	SortFlows(rows)
	want := []string{
		"a/wait-new", "a/wait-old", "a/wait-untimed", "b/wait-untimed",
		"a/open-new", "b/open-new", "a/open-old", "a/open-untimed",
		"a/closed-new", "a/closed-old",
	}
	if got := flowKeys(rows); !reflect.DeepEqual(got, want) {
		t.Errorf("order\n got %v\nwant %v", got, want)
	}
}

func TestSortSprintsRule(t *testing.T) {
	sprint := func(k string, closed bool, updated string) SprintRow {
		return SprintRow{Key: k, Closed: closed, UpdatedAt: updated}
	}
	rows := []SprintRow{
		sprint("a/closed", true, "2026-11-10T00:00:00Z"),
		sprint("b/untimed", false, ""),
		sprint("a/untimed", false, ""),
		sprint("a/old", false, "2026-10-01T00:00:00Z"),
		sprint("b/new", false, "2026-11-05T00:00:00Z"),
		sprint("a/new", false, "2026-11-05T00:00:00Z"),
	}
	SortSprints(rows)
	var got []string
	for _, r := range rows {
		got = append(got, r.Key)
	}
	want := []string{"a/new", "b/new", "a/old", "a/untimed", "b/untimed", "a/closed"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order\n got %v\nwant %v", got, want)
	}
}

func TestPagesSplit(t *testing.T) {
	if got := Pages(0, 5); got == nil || len(got) != 0 {
		t.Errorf("Pages(0,5) = %#v, want empty non-nil", got)
	}
	cases := []struct {
		n, capacity int
		want        []Span
	}{
		{5, 5, []Span{{0, 5}}},
		{11, 5, []Span{{0, 5}, {5, 10}, {10, 11}}},
		{3, 0, []Span{{0, 1}, {1, 2}, {2, 3}}},
		{2, -4, []Span{{0, 1}, {1, 2}}},
		{4, 12, []Span{{0, 4}}},
	}
	for _, c := range cases {
		if got := Pages(c.n, c.capacity); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Pages(%d,%d) = %v, want %v", c.n, c.capacity, got, c.want)
		}
	}
}
