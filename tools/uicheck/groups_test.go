package main

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// The seven groups, in the order a full run takes (B7.2 spec item 6).
func TestDevGroupOrder(t *testing.T) {
	want := []string{"fit", "charts", "station", "layout", "map", "waits", "shell"}
	var got []string
	for _, g := range devGroups {
		got = append(got, g.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("group order = %v, want %v", got, want)
	}
}

func TestDevGroupsNotEmpty(t *testing.T) {
	for _, g := range devGroups {
		if len(g.Cases) == 0 {
			t.Errorf("group %s has no cases", g.Name)
		}
	}
}

func TestDevCaseInOneGroup(t *testing.T) {
	seen := map[string]string{}
	for _, g := range devGroups {
		for _, c := range g.Cases {
			if c.Run == nil {
				t.Errorf("%s/%s has no Run", g.Name, c.Name)
			}
			if prev, ok := seen[c.Name]; ok {
				t.Errorf("case %s is in %s and %s", c.Name, prev, g.Name)
			}
			seen[c.Name] = g.Name
		}
	}
	for alias, target := range devAliases {
		if _, ok := seen[alias]; ok {
			t.Errorf("alias %s is also a case", alias)
		}
		if _, ok := seen[target]; !ok {
			t.Errorf("alias %s points at %s, which is no case", alias, target)
		}
	}
}

// Every old name d0 to d43 still runs, as a case or an alias, and is a dev
// check; so are the group and group/case names.
func TestOldNamesResolve(t *testing.T) {
	for i := 0; i <= 43; i++ {
		name := fmt.Sprintf("d%d", i)
		if checks[name] == nil {
			t.Errorf("old check %s does not resolve", name)
		}
		if !isDevCheck(name) {
			t.Errorf("%s is not a dev check", name)
		}
	}
	for _, g := range devGroups {
		if checks[g.Name] == nil || !isDevCheck(g.Name) {
			t.Errorf("group %s is not a registered dev check", g.Name)
		}
		for _, c := range g.Cases {
			full := g.Name + "/" + c.Name
			if checks[full] == nil || !isDevCheck(full) {
				t.Errorf("%s is not a registered dev check", full)
			}
		}
	}
}

// The w checks stay burnmon.exe checks, "waits" and "shell" do not.
func TestWChecksAreNotDev(t *testing.T) {
	for i := 0; i <= 8; i++ {
		name := fmt.Sprintf("w%d", i)
		if checks[name] == nil {
			t.Errorf("%s not registered", name)
		}
		if isDevCheck(name) {
			t.Errorf("%s counted as a dev check", name)
		}
	}
}

// A group runs its cases in order, backwards on "reverse", passes other args
// through, and stops at the first failure naming group/case.
func TestRunDevGroup(t *testing.T) {
	stubApplyStart(t, func(uintptr, startState) error { return nil })
	var ran []string
	var gotArgs [][]string
	mk := func(name string, fail bool) devCase {
		return devCase{name, func(hwnd uintptr, args []string) error {
			ran = append(ran, name)
			gotArgs = append(gotArgs, args)
			if fail {
				return errors.New("boom")
			}
			return nil
		}}
	}
	g := devGroup{"g", []devCase{mk("a", false), mk("b", false), mk("c", false)}, appStart()}

	if err := runDevGroup(0, g, []string{"x"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Join(ran, " ") != "a b c" {
		t.Fatalf("order = %v", ran)
	}
	if !reflect.DeepEqual(gotArgs[0], []string{"x"}) {
		t.Fatalf("args = %v, want [x]", gotArgs[0])
	}

	ran, gotArgs = nil, nil
	if err := runDevGroup(0, g, []string{"reverse"}); err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if strings.Join(ran, " ") != "c b a" {
		t.Fatalf("reverse order = %v", ran)
	}
	if len(gotArgs[0]) != 0 {
		t.Fatalf("reverse leaked into case args: %v", gotArgs[0])
	}
	if strings.Join(caseNames(g), " ") != "a b c" {
		t.Fatalf("reverse changed the group itself: %v", caseNames(g))
	}

	ran = nil
	g2 := devGroup{"g", []devCase{mk("a", false), mk("b", true), mk("c", false)}, appStart()}
	err := runDevGroup(0, g2, nil)
	if err == nil || !strings.HasPrefix(err.Error(), "g/b: ") {
		t.Fatalf("err = %v, want it to start with g/b: ", err)
	}
	if strings.Join(ran, " ") != "a b" {
		t.Fatalf("ran after the failure: %v", ran)
	}
}

func caseNames(g devGroup) []string {
	var n []string
	for _, c := range g.Cases {
		n = append(n, c.Name)
	}
	return n
}
