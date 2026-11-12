//go:build windows || darwin

package main

import (
	"regexp"
	"strings"
	"testing"
)

// pageFunc returns the body of the page function declared at two-space
// indent as "  function name(", up to the next such declaration.
func pageFunc(t *testing.T, name string) string {
	t.Helper()
	start := strings.Index(pageHTML, "\n  function "+name+"(")
	if start < 0 {
		t.Fatalf("page.html has no function %s at two-space indent", name)
	}
	start++
	rest := pageHTML[start:]
	end := strings.Index(rest[1:], "\n  function ")
	if end < 0 {
		return rest
	}
	return rest[:end+1]
}

// enclosingFunc names the two-space page function that holds offset i, or
// "" when i lies before the first one.
func enclosingFunc(i int) string {
	j := strings.LastIndex(pageHTML[:i], "\n  function ")
	if j < 0 {
		return ""
	}
	m := regexp.MustCompile(`^\n  function (\w+)\(`).FindStringSubmatch(pageHTML[j:])
	if m == nil {
		return ""
	}
	return m[1]
}

// apiTable returns the offsets of the "var api = {" table, start and end.
func apiTable(t *testing.T) (int, int) {
	t.Helper()
	s := strings.Index(pageHTML, "var api = {")
	if s < 0 {
		t.Fatal("page.html has no api table")
	}
	e := strings.Index(pageHTML[s:], "};")
	if e < 0 {
		t.Fatal("api table is not closed")
	}
	return s, s + e + 2
}

type cssRule struct {
	Selectors []string
	Body      string
}

// cssRules parses the rules of the page's <style> element, entering
// @media blocks. Comments are dropped.
func cssRules(t *testing.T) []cssRule {
	t.Helper()
	s := strings.Index(pageHTML, "<style>")
	e := strings.Index(pageHTML, "</style>")
	if s < 0 || e < s {
		t.Fatal("page.html has no <style> element")
	}
	css := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(pageHTML[s+len("<style>"):e], "")
	return parseCSS(t, css)
}

func parseCSS(t *testing.T, css string) []cssRule {
	t.Helper()
	var out []cssRule
	for {
		open := strings.Index(css, "{")
		if open < 0 {
			if strings.TrimSpace(css) != "" {
				t.Fatalf("css text outside a rule: %q", css)
			}
			return out
		}
		sel := strings.TrimSpace(css[:open])
		depth, close := 0, -1
		for i := open; i < len(css); i++ {
			switch css[i] {
			case '{':
				depth++
			case '}':
				depth--
			}
			if depth == 0 {
				close = i
				break
			}
		}
		if close < 0 {
			t.Fatalf("css rule %q is not closed", sel)
		}
		body := css[open+1 : close]
		if strings.HasPrefix(sel, "@media") {
			out = append(out, parseCSS(t, body)...)
		} else {
			var sels []string
			for _, p := range strings.Split(sel, ",") {
				sels = append(sels, strings.Join(strings.Fields(p), " "))
			}
			out = append(out, cssRule{Selectors: sels, Body: body})
		}
		css = css[close+1:]
	}
}

// ruleFor returns the body of the first rule whose selector list is
// exactly sel.
func ruleFor(t *testing.T, sel string) string {
	t.Helper()
	for _, r := range cssRules(t) {
		if len(r.Selectors) == 1 && r.Selectors[0] == sel {
			return r.Body
		}
	}
	t.Fatalf("page.html has no css rule %q", sel)
	return ""
}

// compact drops all white space, so declarations compare by value.
func compact(s string) string {
	return strings.Join(strings.Fields(s), "")
}
