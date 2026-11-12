package main

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type winresFile struct {
	Icon     map[string]map[string][]string `json:"RT_GROUP_ICON"`
	Manifest map[string]map[string]struct {
		Identity struct {
			Version string `json:"version"`
		} `json:"identity"`
	} `json:"RT_MANIFEST"`
	Version map[string]map[string]struct {
		Fixed struct {
			FileVersion    string `json:"file_version"`
			ProductVersion string `json:"product_version"`
		} `json:"fixed"`
		Info map[string]map[string]string `json:"info"`
	} `json:"RT_VERSION"`
}

func loadWinres(t *testing.T, name string) winresFile {
	t.Helper()
	var w winresFile
	if err := json.Unmarshal([]byte(readText(t, filepath.Join("..", "..", "winres", name))), &w); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return w
}

func TestWinresControlCentre(t *testing.T) {
	cc, dev := loadWinres(t, "controlcentre.json"), loadWinres(t, "burnmon-dev.json")
	if len(cc.Icon) == 0 || !reflect.DeepEqual(cc.Icon, dev.Icon) {
		t.Errorf("RT_GROUP_ICON %v, want %v", cc.Icon, dev.Icon)
	}
	ccv, devv := cc.Version["#1"]["0000"], dev.Version["#1"]["0000"]
	if ccv.Fixed != devv.Fixed || devv.Fixed.FileVersion == "" {
		t.Errorf("fixed %+v, want %+v", ccv.Fixed, devv.Fixed)
	}
	if got, want := cc.Manifest["#1"]["0409"].Identity.Version, dev.Manifest["#1"]["0409"].Identity.Version; got != want || want == "" {
		t.Errorf("manifest version %q, want %q", got, want)
	}
	ci, di := ccv.Info["0409"], devv.Info["0409"]
	for _, k := range []string{"ProductVersion", "FileVersion"} {
		if ci[k] != di[k] || di[k] == "" {
			t.Errorf("%s %q, want %q", k, ci[k], di[k])
		}
	}
	if ci["OriginalFilename"] != "controlcentre.exe" {
		t.Errorf("OriginalFilename %q, want controlcentre.exe", ci["OriginalFilename"])
	}
	if ci["ProductName"] != "Control Centre" {
		t.Errorf("ProductName %q, want Control Centre", ci["ProductName"])
	}
}

func TestBuildScriptBuildsControlCentre(t *testing.T) {
	s := readText(t, filepath.Join("..", "..", "build.ps1"))
	for _, want := range []string{
		`winres\controlcentre.json --out cmd\controlcentre\rsrc`,
		`-o controlcentre.exe .\cmd\controlcentre`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("build.ps1 lacks %s", want)
		}
	}
	for i, r := range s {
		if r > 127 {
			t.Fatalf("build.ps1 has a non-ASCII character at byte %d", i)
		}
	}
}
