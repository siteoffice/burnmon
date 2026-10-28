package gitrefs

import "testing"

const (
	b6Branch  = "sprint/burnmon-b6-map-and-waits"
	b61Branch = "sprint/burnmon-b6-1-history-lines-failed-box"
)

// realTopology builds the shape of the real B6 and B6.1 history (item 11).
// A commit's name is the hash of its content, so a real sha cannot be
// recreated: the map keeps the real short sha as key and the generated name as
// value. Real short shas and branch names, approved by Wilco 2026-10-28 for
// item 11 only; no paths or user data.
//
// Shape: main b7901f2 has parents edca323 (first) and 24cee32 (second).
// 24cee32 -> eaa562d -> f092399 -> 087452c -> ... -> 9380f7a -> edca323.
// Two negative branches: example-fresh sits on edca323 (a main first-parent
// commit, no commit of its own); example-ff sits on example-ffc, one
// first-parent commit below edca323.
func realTopology(t *testing.T) (repo string, c map[string]string) {
	t.Helper()
	repo, gd := newRepo(t)
	c = map[string]string{}
	root := writeCommit(t, gd)
	c["example-ffc"] = writeCommit(t, gd, root)
	c["edca323"] = writeCommit(t, gd, c["example-ffc"])
	// B6 chain, oldest first, each on the previous one.
	chain := []string{"9380f7a", "98cf39c", "9be6442", "b6c0c62", "eebc7c5", "f0c8fd0",
		"2a734c4", "d653e42", "11a504e", "4096f94", "68ca30b", "56eb94a", "384d365",
		"7016a7b", "1116449", "087452c"}
	prev := c["edca323"]
	for _, name := range chain {
		c[name] = writeCommit(t, gd, prev)
		prev = c[name]
	}
	// B6.1 stacked on B6: f092399 -> eaa562d -> 24cee32.
	c["f092399"] = writeCommit(t, gd, c["087452c"])
	c["eaa562d"] = writeCommit(t, gd, c["f092399"])
	c["24cee32"] = writeCommit(t, gd, c["eaa562d"])
	c["b7901f2"] = writeCommit(t, gd, c["edca323"], c["24cee32"])
	setRef(t, gd, "refs/heads/main", c["b7901f2"])
	setRef(t, gd, "refs/heads/"+b6Branch, c["087452c"])
	setRef(t, gd, "refs/heads/"+b61Branch, c["24cee32"])
	setRef(t, gd, "refs/heads/example-fresh", c["edca323"])
	setRef(t, gd, "refs/heads/example-ff", c["example-ffc"])
	return repo, c
}

// B6.1 was cut from B6's head, so merging B6.1 brought B6 in through the
// merge's second parent. Both must read merged.
func TestStackedBranchReadsMerged(t *testing.T) {
	repo, _ := realTopology(t)
	ck := NewChecker()
	expect(t, ck, repo, b61Branch, true, true)
	expect(t, ck, repo, b6Branch, true, true)
	for _, br := range []string{b61Branch, b6Branch} {
		m, k := ck.MergedOrGone(repo, br)
		if !m || !k {
			t.Fatalf("MergedOrGone(%q) = (%v, %v), want (true, true)", br, m, k)
		}
	}
}

func TestFreshBranchAtMainCommitStaysBlocked(t *testing.T) {
	repo, _ := realTopology(t)
	expect(t, NewChecker(), repo, "example-fresh", false, true)
}

func TestFastForwardedBranchStaysBlocked(t *testing.T) {
	repo, _ := realTopology(t)
	expect(t, NewChecker(), repo, "example-ff", false, true)
}
