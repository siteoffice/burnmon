//go:build windows

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"burnmon/internal/live"
)

// OF28: a Cowork (Cloud) session shows how old its data is. The label text is
// built in Go (live.Session.CloudAge); the page and the Station print it
// verbatim, and only when it is there, so a local row stays as it was.

func TestPageSessionCardShowsCloudAge(t *testing.T) {
	body := idleFn(t, "renderSessionCards")
	const want = `rec.small.textContent = 'turn ' + s.turn_count + ' · ' + fmtTokens(s.tokens) + ' tok · ' + fmtCost(s.cost) + (s.cloud_age ? ' · ' + s.cloud_age : '');`
	if !strings.Contains(body, want) {
		t.Errorf("renderSessionCards lacks %q", want)
	}
}

func TestStationAdapterCopiesCloudAge(t *testing.T) {
	js := strings.ReplaceAll(stationJS, "\r\n", "\n")
	i := strings.Index(js, "function fromSnapshot(")
	if i < 0 {
		t.Fatal("station.js lacks fromSnapshot")
	}
	body := js[i:]
	if j := strings.Index(body, "\n  // ----"); j > 0 {
		body = body[:j]
	}
	if !strings.Contains(body, "cloud_age: s.cloud_age || ''") {
		t.Error("fromSnapshot does not copy cloud_age")
	}
}

func stationFnBody(t *testing.T, name string) string {
	t.Helper()
	js := strings.ReplaceAll(stationJS, "\r\n", "\n")
	i := strings.Index(js, "function "+name+"(")
	if i < 0 {
		t.Fatalf("station.js lacks %s", name)
	}
	j := strings.Index(js[i+1:], "\n    function ")
	if j < 0 {
		t.Fatalf("cannot find the end of %s", name)
	}
	return js[i : i+1+j]
}

func TestStationLabelShowsCloudAge(t *testing.T) {
	body := stationFnBody(t, "drawLabel")
	if !strings.Contains(body, "if(d.cloud_age) line2 += ' \\u00b7 ' + d.cloud_age;") {
		t.Error("drawLabel does not add cloud_age to the second line under a truthiness check")
	}
	// Added before the width is measured, so the box fits the text.
	if a, b := strings.Index(body, "d.cloud_age"), strings.Index(body, "ctx.measureText(line2)"); a < 0 || b < 0 || a > b {
		t.Error("drawLabel measures line2 before cloud_age is added")
	}
}

func TestStationTooltipShowsCloudAge(t *testing.T) {
	body := stationFnBody(t, "showTip")
	if !strings.Contains(body, "(d.cloud_age ? '<div><span>Data</span>' + esc(d.cloud_age) + '</div>' : '')") {
		t.Error("showTip does not print an escaped cloud_age row under a truthiness check")
	}
}

func TestPageSessionsTableShowsCloudAge(t *testing.T) {
	body := idleFn(t, "secBodySessions")
	const want = `esc(secWord(x.state)) + (x.cloud_age ? ' · ' + esc(x.cloud_age) : '') + '</td>`
	if !strings.Contains(body, want) {
		t.Errorf("secBodySessions lacks %q", want)
	}
}

func TestLocalSessionJSONHasNoCloudAge(t *testing.T) {
	snap := live.Snapshot{Sessions: []*live.Session{{SessionID: "s1", Agent: "claude-code", Vendor: "claude"}}}
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "cloud_age") {
		t.Errorf("Claude Code session JSON has a cloud_age key: %s", b)
	}
}
