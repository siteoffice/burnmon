package lamp

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"burnmon/internal/fsutil"
)

// AllMoods lists the eight moods, the order of the sheet and the poses.
var AllMoods = []string{
	MoodOff, MoodAsleep, MoodWorking, MoodDone,
	MoodYourTurn, MoodBlocked, MoodBored, MoodSweating,
}

// poseBricks is the wall the bored pose stands beside.
const poseBricks = 12

// glimSized is Glim with its outer element given the extra attributes attrs
// (an id, or an x and y) and a height of h; the width follows the picture.
func glimSized(mood string, bricks int, theme, attrs string, h float64) string {
	svg := Glim(mood, bricks, theme)
	w := h
	if mood == MoodBored && bricks > 0 {
		w = h * 320 / 200
	}
	i := strings.Index(svg, ` width="`)
	j := strings.Index(svg, ">")
	if i < 0 || j < i {
		return svg
	}
	return svg[:i] + " " + attrs + ` width="` + ftoa(w) + `" height="` + ftoa(h) + `">` + svg[j+1:]
}

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func xmlText(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

const svgHead = `<svg xmlns="http://www.w3.org/2000/svg" `

// WritePoses writes the ten files of the poses into an existing folder: one
// glim-<mood>.svg per mood (256 by 256), glim-sheet.svg (1200 by 630) and
// glim-demo.svg. It refuses a folder that does not exist.
func WritePoses(dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("the folder %s does not exist", dir)
	}
	if !fi.IsDir() {
		return errors.New(dir + " is not a folder")
	}
	files := map[string]string{"glim-demo.svg": DemoSVG(), "glim-sheet.svg": sheetSVG()}
	for _, m := range AllMoods {
		files["glim-"+m+".svg"] = poseSVG(m)
	}
	for name, body := range files {
		if err := fsutil.WriteFileAtomic(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func poseBrickCount(mood string) int {
	if mood == MoodBored {
		return poseBricks
	}
	return 0
}

// poseSVG is one mood, centred on a 256 by 256 canvas, dark theme.
func poseSVG(mood string) string {
	const size = 256.0
	h := 180.0
	bricks := poseBrickCount(mood)
	w := h
	if bricks > 0 {
		w = h * 320 / 200
		if w > size-8 {
			w = size - 8
			h = w * 200 / 320
		}
	}
	x, y := (size-w)/2, (size-h)/2
	attrs := `x="` + ftoa(x) + `" y="` + ftoa(y) + `"`
	return svgHead + `viewBox="0 0 256 256" width="256" height="256">` +
		glimSized(mood, bricks, "dark", attrs, h) + `</svg>` + "\n"
}

// sheetSVG is all eight moods on a 1200 by 630 card, four to a row.
func sheetSVG() string {
	var b strings.Builder
	b.WriteString(svgHead + `viewBox="0 0 1200 630" width="1200" height="630">`)
	b.WriteString(`<rect width="1200" height="630" fill="#15110a"/>`)
	for i, m := range AllMoods {
		cx, top := float64(150+300*(i%4)), float64(40+300*(i/4))
		h := 190.0
		bricks := poseBrickCount(m)
		w := h
		if bricks > 0 {
			h, w = 150, 240
		}
		attrs := `x="` + ftoa(cx-w/2) + `" y="` + ftoa(top+(190-h)/2) + `"`
		b.WriteString(glimSized(m, bricks, "dark", attrs, h))
		b.WriteString(`<text x="` + ftoa(cx) + `" y="` + ftoa(top+225) + `" text-anchor="middle" fill="#ffc766" font-family="Segoe UI, sans-serif" font-size="22">` + xmlText(m) + `</text>`)
	}
	b.WriteString(`</svg>` + "\n")
	return b.String()
}

// DemoSVG is the moving picture of the README: the 24 seconds of the demo as
// one looping SVG, 24 frames of one second each. The motion is one CSS
// keyframe animation, no script and no outside resource. Glim is drawn once
// per mood in defs and placed with use.
func DemoSVG() string {
	var b strings.Builder
	b.WriteString(svgHead + `viewBox="0 0 320 40" width="640" height="80">`)
	b.WriteString(`<style>.f{opacity:0;animation:show 24s step-end infinite}` +
		`@keyframes show{0%{opacity:1}4.1%{opacity:1}4.2%{opacity:0}100%{opacity:0}}</style>`)
	b.WriteString(`<defs><clipPath id="tc"><rect x="46" y="0" width="212" height="40"/></clipPath>`)
	ids := map[string]bool{}
	frame := make([]State, DemoSeconds)
	for sec := range frame {
		frame[sec] = DemoState(sec)
		st := frame[sec]
		id := demoGlimID(st)
		if ids[id] {
			continue
		}
		ids[id] = true
		bricks := 0
		if st.Mood == MoodBored {
			bricks = st.WallBricks
		}
		b.WriteString(glimSized(st.Mood, bricks, "dark", `id="`+id+`"`, 32))
	}
	b.WriteString(`</defs>`)
	b.WriteString(`<rect width="320" height="40" fill="#000000"/>`)
	for sec, st := range frame {
		b.WriteString(`<g class="f" style="animation-delay:` + strconv.Itoa(sec) + `s">`)
		if st.Mood == MoodAsleep || st.Mood == MoodWorking {
			// asleep and working show the sliver
			b.WriteString(`<rect x="100" y="0" width="120" height="6" fill="#ffc766"/>`)
		} else {
			b.WriteString(`<rect width="320" height="40" fill="#15110a"/>`)
			b.WriteString(`<use href="#` + demoGlimID(st) + `" x="8" y="4"/>`)
			b.WriteString(`<g clip-path="url(#tc)" font-family="Segoe UI, sans-serif">` +
				`<text x="50" y="17" fill="#ffc766" font-size="13" font-weight="600">` + xmlText(st.Headline) + `</text>` +
				`<text x="50" y="32" fill="#d4bf94" font-size="11">` + xmlText(st.Line) + `</text></g>`)
		}
		b.WriteString(`<text x="290" y="` + demoTagY(st) + `" fill="#ffc766" font-family="Segoe UI, sans-serif" font-size="11" font-weight="600">demo</text>`)
		b.WriteString(`</g>`)
	}
	b.WriteString(`</svg>` + "\n")
	return b.String()
}

func demoTagY(st State) string {
	if st.Mood == MoodAsleep || st.Mood == MoodWorking {
		return "14"
	}
	return "24"
}

func demoGlimID(st State) string {
	if st.Mood == MoodBored {
		return "gl-bored-" + strconv.Itoa(st.WallBricks)
	}
	return "gl-" + st.Mood
}
