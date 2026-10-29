package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// devCase is one named burnmon-dev.exe check inside a group. Name is the old
// d number (d34), so "layout/d34" and plain "d34" both run it.
type devCase struct {
	Name string
	Run  checkFunc
}

type devGroup struct {
	Name  string
	Cases []devCase
}

// devGroups is the fixed order a full run takes (B7.2 spec item 6). Each old
// d check is a case in exactly one group; groups_test.go holds that line.
var devGroups = []devGroup{
	{"fit", []devCase{
		{"d2", caseD2}, {"d3", caseD3}, {"d4", caseD4}, {"d5", caseD5},
		{"d6", caseD6}, {"d7", caseD7}, {"d8", caseD8}, {"d9", caseD9},
	}},
	{"charts", []devCase{
		{"d10", caseD10}, {"d11", caseD11}, {"d12", caseD12}, {"d13", caseD13},
		{"d14", caseD14}, {"d15", caseD15}, {"d16", caseD16}, {"d17", caseD17},
		{"d18", caseD18}, {"d20", caseD20}, {"d21", caseD21}, {"d22", caseD22},
	}},
	{"station", []devCase{
		{"d23", caseD23}, {"d25", caseD25}, {"d26", caseD26}, {"d27", caseD27},
		{"d28", caseD28}, {"d41", caseD41},
	}},
	// d19 sits here, not in charts: it asserts the fit pass shares the free
	// height and To Do reaches the window bottom, a layout rule.
	{"layout", []devCase{
		{"d19", caseD19}, {"d29", caseD29}, {"d33", caseD33}, {"d34", caseD34},
		{"d42", caseD42}, {"d43", caseD43},
	}},
	{"map", []devCase{
		{"d30", caseD30}, {"d31", caseD31}, {"d35", caseD35}, {"d38", caseD38},
		{"d40", caseD40},
	}},
	{"waits", []devCase{
		{"d36", caseD36}, {"d37", caseD37}, {"d39", caseD39},
	}},
	{"shell", []devCase{
		{"d0", caseD0}, {"d1", caseD1}, {"d24", caseD24}, {"d32", caseD32},
	}},
}

// devAliases maps an old name that was merged into another case to the case
// that now covers its assert. Empty: no case was merged in B7.2 Task 3.
var devAliases = map[string]string{}

// devNames holds every name that targets burnmon-dev.exe: group, group/case,
// case and alias. main.go's isDevCheck reads it, since "waits" and "shell" do
// not start with "d" and "waits" starts with the w checks' own prefix.
var devNames = map[string]bool{}

func init() {
	for _, g := range devGroups {
		g := g
		checks[g.Name] = func(hwnd uintptr, args []string) error { return runDevGroup(hwnd, g, args) }
		devNames[g.Name] = true
		for _, c := range g.Cases {
			checks[g.Name+"/"+c.Name] = c.Run
			checks[c.Name] = c.Run
			devNames[g.Name+"/"+c.Name] = true
			devNames[c.Name] = true
		}
	}
	for alias, target := range devAliases {
		checks[alias] = checks[target]
		devNames[alias] = true
	}
}

// runDevGroup runs a group's cases in order (backwards when args holds
// "reverse") and stops at the first failure, naming it group/case. Any other
// args go to every case unchanged.
func runDevGroup(hwnd uintptr, g devGroup, args []string) error {
	cases := append([]devCase(nil), g.Cases...)
	var rest []string
	for _, a := range args {
		if a == "reverse" {
			for i, j := 0, len(cases)-1; i < j; i, j = i+1, j-1 {
				cases[i], cases[j] = cases[j], cases[i]
			}
			continue
		}
		rest = append(rest, a)
	}
	names := make([]string, len(cases))
	for i, c := range cases {
		names[i] = c.Name
	}
	fmt.Printf("uicheck: group %s: %s\n", g.Name, strings.Join(names, " "))
	for _, c := range cases {
		full := g.Name + "/" + c.Name
		fmt.Printf("--- case %s ---\n", full)
		if err := c.Run(hwnd, rest); err != nil {
			return fmt.Errorf("%s: %w", full, err)
		}
		fmt.Printf("uicheck: %s OK\n", full)
	}
	return nil
}

// readFixture reads a frozen example from the repo's testdata folder (uicheck
// runs from tools\uicheck), the setup line most d cases shared word for word.
func readFixture(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join("..", "..", "testdata", name))
}
