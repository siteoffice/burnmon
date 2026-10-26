package controls

import (
	"errors"
	"regexp"
	"strconv"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newPreviews(c *fakeClock) *Previews { return &Previews{Now: c.now} }

var t0 = time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)

func stopTarget(id string) Target {
	return Target{Control: Stop, AgentID: id, PID: 42, StartedAt: 1, Project: "other"}
}

func TestTakeWithinWindowRunsOnce(t *testing.T) {
	c := &fakeClock{t: t0}
	p := newPreviews(c)
	argv := []string{`C:\x\claude.exe`, "stop", "ab12"}
	pend := p.Add(stopTarget("ab12"), argv)
	c.t = t0.Add(time.Second)
	got, err := p.Take(pend.Token)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Argv) != 3 || got.Argv[1] != "stop" || got.Argv[2] != "ab12" {
		t.Fatalf("argv %v", got.Argv)
	}
	if _, err := p.Take(pend.Token); !errors.Is(err, ErrNoPreview) {
		t.Fatalf("reuse: %v", err)
	}
}

func TestTakeAfterFiveSecondsExpires(t *testing.T) {
	c := &fakeClock{t: t0}
	p := newPreviews(c)
	a := p.Add(stopTarget("ab12"), []string{"x"})
	c.t = t0.Add(PreviewTTL + time.Millisecond)
	if _, err := p.Take(a.Token); !errors.Is(err, ErrExpired) {
		t.Fatalf("want expired, got %v", err)
	}
	if _, err := p.Take(a.Token); !errors.Is(err, ErrNoPreview) {
		t.Fatalf("again: %v", err)
	}
	b := p.Add(stopTarget("ab12"), []string{"x"})
	c.t = c.t.Add(PreviewTTL)
	if _, err := p.Take(b.Token); err != nil {
		t.Fatalf("exactly 5 s must run: %v", err)
	}
}

func TestTakeTooSoonKeepsToken(t *testing.T) {
	c := &fakeClock{t: t0}
	p := newPreviews(c)
	a := p.Add(stopTarget("ab12"), []string{"x"})
	c.t = t0.Add(50 * time.Millisecond)
	if _, err := p.Take(a.Token); !errors.Is(err, ErrTooSoon) {
		t.Fatalf("want too soon, got %v", err)
	}
	c.t = t0.Add(399 * time.Millisecond)
	if _, err := p.Take(a.Token); !errors.Is(err, ErrTooSoon) {
		t.Fatalf("399 ms: %v", err)
	}
	c.t = t0.Add(450 * time.Millisecond)
	if _, err := p.Take(a.Token); err != nil {
		t.Fatalf("450 ms: %v", err)
	}
	// exactly the minimum gap runs
	b := p.Add(stopTarget("cd34"), []string{"x"})
	c.t = c.t.Add(MinConfirmGap)
	if _, err := p.Take(b.Token); err != nil {
		t.Fatalf("exactly 400 ms: %v", err)
	}
}

func TestUnknownTokenIsNoPreview(t *testing.T) {
	p := newPreviews(&fakeClock{t: t0})
	for _, tok := range []string{"", "deadbeef", "zz"} {
		if _, err := p.Take(tok); !errors.Is(err, ErrNoPreview) {
			t.Fatalf("%q: %v", tok, err)
		}
	}
}

func TestNewPreviewReplacesOld(t *testing.T) {
	c := &fakeClock{t: t0}
	p := newPreviews(c)
	a := p.Add(stopTarget("ab12"), []string{"x"})
	b := p.Add(stopTarget("ab12"), []string{"x"})
	if a.Token == b.Token {
		t.Fatal("same token")
	}
	c.t = t0.Add(time.Second)
	if _, err := p.Take(a.Token); !errors.Is(err, ErrNoPreview) {
		t.Fatalf("old token: %v", err)
	}
	d := p.Add(stopTarget("cd34"), []string{"x"})
	e := p.Add(Target{Control: Attach, AgentID: "cd34"}, []string{"x"})
	c.t = c.t.Add(time.Second)
	if _, err := p.Take(d.Token); err != nil {
		t.Fatalf("d: %v", err)
	}
	if _, err := p.Take(e.Token); err != nil {
		t.Fatalf("e: %v", err)
	}
	if _, err := p.Take(b.Token); err != nil {
		t.Fatalf("b: %v", err)
	}
}

func TestAddDropsExpired(t *testing.T) {
	c := &fakeClock{t: t0}
	p := newPreviews(c)
	a := p.Add(stopTarget("ab12"), []string{"x"})
	c.t = t0.Add(10 * time.Second)
	p.Add(stopTarget("cd34"), []string{"x"})
	if _, err := p.Take(a.Token); !errors.Is(err, ErrNoPreview) {
		t.Fatalf("expired entry should be dropped: %v", err)
	}
}

func TestTokensAreRandomHex(t *testing.T) {
	c := &fakeClock{t: t0}
	p := newPreviews(c)
	re := regexp.MustCompile(`^[0-9a-f]{32}$`)
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		tok := p.Add(Target{Control: Stop, AgentID: "id" + strconv.Itoa(i)}, []string{"x"}).Token
		if !re.MatchString(tok) {
			t.Fatalf("token %q", tok)
		}
		if seen[tok] {
			t.Fatal("duplicate token")
		}
		seen[tok] = true
	}
}

func TestPendingFields(t *testing.T) {
	c := &fakeClock{t: t0}
	p := newPreviews(c)
	pend := p.Add(stopTarget("ab12"), []string{`C:\x y\claude.exe`, "stop", "ab12"})
	if !pend.Created.Equal(t0) {
		t.Fatalf("created %v", pend.Created)
	}
	if pend.Command != CommandLine(pend.Argv) {
		t.Fatalf("command %q", pend.Command)
	}
}

func TestLabel(t *testing.T) {
	if got := (Target{Project: "other", AgentID: "ab12"}).Label(); got != "other (ab12)" {
		t.Fatal(got)
	}
	g := Target{Control: Approve, Title: "flow-review", Gate: "G2", FlowID: "example__flow__2"}
	if got := g.Label(); got != "flow-review gate G2 (example__flow__2)" {
		t.Fatal(got)
	}
}

func TestKey(t *testing.T) {
	if got := stopTarget("ab12").Key(); got != "stop|ab12" {
		t.Fatal(got)
	}
	g := Target{Control: Approve, FlowID: "f1", Gate: "G2"}
	if got := g.Key(); got != "approve|gate:f1:G2" {
		t.Fatal(got)
	}
}
