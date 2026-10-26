package controls

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAddAndTakeDoNotShareArgv(t *testing.T) {
	c := &fakeClock{t: t0}
	p := newPreviews(c)
	in := []string{"claude.exe", "stop", "ab12"}
	pend := p.Add(stopTarget("ab12"), in)
	in[1] = "evil"
	pend.Argv[2] = "evil"
	c.t = t0.Add(time.Second)
	got, err := p.Take(pend.Token)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Argv) != 3 || got.Argv[0] != "claude.exe" || got.Argv[1] != "stop" || got.Argv[2] != "ab12" {
		t.Fatalf("stored argv was mutated: %v", got.Argv)
	}
}

func TestConcurrentTakeOneWinner(t *testing.T) {
	c := &fakeClock{t: t0}
	p := newPreviews(c)
	pend := p.Add(stopTarget("ab12"), []string{"x"})
	c.t = t0.Add(time.Second)
	const n = 50
	var ok, none int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := p.Take(pend.Token)
			switch {
			case err == nil:
				atomic.AddInt32(&ok, 1)
			case errors.Is(err, ErrNoPreview):
				atomic.AddInt32(&none, 1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if ok != 1 || none != n-1 {
		t.Fatalf("ok=%d noPreview=%d", ok, none)
	}
}

func TestAuditHostileLabelStaysOneLine(t *testing.T) {
	label := "x\n" + `{"control":"approve"}`
	var sb strings.Builder
	l := AuditLine{Time: "t", Control: "stop", Target: label, Command: "c", ExitCode: 0}
	if err := WriteAudit(&sb, l); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
		t.Fatalf("want exactly one trailing newline: %q", out)
	}
	var back AuditLine
	if err := json.Unmarshal([]byte(out), &back); err != nil {
		t.Fatal(err)
	}
	if back.Target != label || back.Control != "stop" {
		t.Fatalf("round trip: %+v", back)
	}
}

type failWriter struct{ err error }

func (f failWriter) Write([]byte) (int, error) { return 0, f.err }

func TestWriteAuditReturnsWriterError(t *testing.T) {
	want := errors.New("disk full")
	if err := WriteAudit(failWriter{want}, AuditLine{Control: "stop"}); !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
}

func TestKeyGateIsUnambiguous(t *testing.T) {
	a := Target{Control: Approve, FlowID: "a:b", Gate: "c"}
	b := Target{Control: Approve, FlowID: "a", Gate: "b:c"}
	if a.Key() == b.Key() {
		t.Fatalf("colliding keys %q", a.Key())
	}
}
