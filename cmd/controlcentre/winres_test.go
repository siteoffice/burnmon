package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
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
	// Its own icon (option A, the tree, Wilco 2026-11-15), not BurnMon's bars
	// with the euro: the same seven sizes, from cc-icon-<size>.png files.
	sizes := []int{256, 128, 64, 48, 32, 24, 16}
	var want []string
	for _, n := range sizes {
		want = append(want, fmt.Sprintf("cc-icon-%d.png", n))
	}
	if got := cc.Icon["#1"]["0000"]; !reflect.DeepEqual(got, want) {
		t.Errorf("RT_GROUP_ICON %v, want %v", got, want)
	}
	if reflect.DeepEqual(cc.Icon, dev.Icon) {
		t.Error("controlcentre.exe still has the BurnMon Dev icon")
	}
	for i, name := range want {
		f, err := os.Open(filepath.Join("..", "..", "winres", name))
		if err != nil {
			t.Fatal(err)
		}
		c, err := png.DecodeConfig(f)
		f.Close()
		if err != nil || c.Width != sizes[i] || c.Height != sizes[i] {
			t.Errorf("%s: %dx%d (%v), want %d px square", name, c.Width, c.Height, err, sizes[i])
		}
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

// TestControlCentreIcns: the macOS icon (same art as the exe icon) is a valid
// icns whose entries fill the file and include 512 px (ic09) and 1024 px
// (ic10). No bundle uses it yet; the future .app step picks it up from here.
func TestControlCentreIcns(t *testing.T) {
	d, err := os.ReadFile(filepath.Join("..", "..", "winres", "controlcentre.icns"))
	if err != nil {
		t.Fatal(err)
	}
	if len(d) < 8 || string(d[:4]) != "icns" || int(binary.BigEndian.Uint32(d[4:8])) != len(d) {
		t.Fatalf("not an icns file of its own length (%d bytes)", len(d))
	}
	types := map[string]bool{}
	for i := 8; i < len(d); {
		if i+8 > len(d) {
			t.Fatalf("entry header cut at byte %d", i)
		}
		n := int(binary.BigEndian.Uint32(d[i+4 : i+8]))
		if n < 8 || i+n > len(d) {
			t.Fatalf("entry %q at %d has bad length %d", d[i:i+4], i, n)
		}
		types[string(d[i:i+4])] = true
		i += n
	}
	for _, want := range []string{"ic09", "ic10"} {
		if !types[want] {
			t.Errorf("icns lacks %s; has %v", want, types)
		}
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
