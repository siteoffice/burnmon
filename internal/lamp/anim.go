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
	"burnmon/internal/waits"
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
	// every pose is 155 high; the bored one is wider (1.6 times) for the wall
	// and still fits the canvas (248 of 256)
	h := 155.0
	bricks := poseBrickCount(mood)
	w := h
	if bricks > 0 {
		w = h * 320 / 200
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
		// every pose is 170 high; the bored one is wider (272) for the wall
		h := 170.0
		bricks := poseBrickCount(m)
		w := h
		if bricks > 0 {
			w = h * 320 / 200
		}
		attrs := `x="` + ftoa(cx-w/2) + `" y="` + ftoa(top+(190-h)/2) + `"`
		b.WriteString(glimSized(m, bricks, "dark", attrs, h))
		b.WriteString(`<text x="` + ftoa(cx) + `" y="` + ftoa(top+225) + `" text-anchor="middle" fill="#ffc766" font-family="Segoe UI, sans-serif" font-size="22">` + xmlText(waits.MoodWord(m)) + `</text>`)
	}
	b.WriteString(`</svg>` + "\n")
	return b.String()
}
