package main

import (
	"os"
	"strings"
	"testing"
)

func TestMacDOMOnlyCases(t *testing.T) {
	if len(macDOMOnlyCases) == 0 {
		t.Fatal("macDOMOnlyCases is empty")
	}
	for _, name := range macDOMOnlyCases {
		if checks[name] == nil {
			t.Errorf("%s is not a registered check", name)
		}
		if !isDevCheck(name) {
			t.Errorf("%s is not a burnmon-dev check", name)
		}
	}
	b, err := os.ReadFile("../../scripts/maccheck.sh")
	if err != nil {
		t.Fatal(err)
	}
	want := `DOM_CASES="` + strings.Join(macDOMOnlyCases, " ") + `"`
	if !strings.Contains(strings.ReplaceAll(string(b), "\r\n", "\n"), want) {
		t.Errorf("scripts/maccheck.sh lacks %s", want)
	}
}
