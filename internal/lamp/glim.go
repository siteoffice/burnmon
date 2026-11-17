package lamp

import (
	"strconv"
	"strings"
)

// Glim draws the lamp character as an SVG string. It is a pure function of
// the mood, the wall bricks and the theme ("light"; anything else is dark):
// no image, no font file, no script, no outside resource.
//
// The picture is 200 high. It is 200 wide, or 320 wide when the wall of mood
// bored stands beside Glim (bricks above 0, at most 30). The eyes, rays, drop,
// z z and wall carry the class names glim-eye, glim-ray, glim-drop, glim-z,
// glim-wall and glim-brick so the page can animate them.
func Glim(mood string, bricks int, theme string) string {
	p := paletteFor(theme)
	if _, ok := domeShade[mood]; !ok {
		mood = MoodAsleep
	}
	wall := mood == MoodBored && bricks > 0
	width := 200
	if wall {
		width = 320
	}
	var b strings.Builder
	b.WriteString(`<svg class="glim glim-` + mood + `" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` +
		strconv.Itoa(width) + ` 200" width="` + strconv.Itoa(width) + `" height="200">`)
	// carry handle, dome, base and two feet
	b.WriteString(`<path d="M80 52 C84 18 116 18 120 52" fill="none" stroke="` + p.handle + `" stroke-width="7" stroke-linecap="round"/>`)
	b.WriteString(`<path class="glim-dome" d="M40 150 C40 74 68 48 100 48 C132 48 160 74 160 150 Z" fill="` + p.dome[domeShade[mood]] + `"/>`)
	b.WriteString(`<rect x="32" y="150" width="136" height="24" rx="6" fill="` + p.base + `"/>`)
	b.WriteString(`<rect x="48" y="174" width="26" height="10" rx="3" fill="` + p.base + `"/>`)
	b.WriteString(`<rect x="126" y="174" width="26" height="10" rx="3" fill="` + p.base + `"/>`)
	b.WriteString(eyes(mood, p))
	b.WriteString(mouth(mood, p))
	b.WriteString(extras(mood, p))
	if wall {
		b.WriteString(brickWall(min(bricks, maxBricks), p))
	}
	b.WriteString(`</svg>`)
	return b.String()
}

type palette struct {
	dome   [4]string // grey, dark amber, amber, light amber
	base   string
	handle string
	sclera string
	ink    string
	ray    string
	drop   string
	brick  string
}

// domeShade maps a mood to its dome colour, and lists the eight moods.
var domeShade = map[string]int{
	MoodOff: 0, MoodAsleep: 1, MoodWorking: 2, MoodDone: 2,
	MoodYourTurn: 3, MoodBlocked: 3, MoodBored: 2, MoodSweating: 2,
}

func paletteFor(theme string) palette {
	if theme == "light" {
		return palette{
			dome: [4]string{"#9aa0aa", "#b7791f", "#e8920c", "#f5b13a"},
			base: "#4b5058", handle: "#6b7079", sclera: "#fffaf0", ink: "#2a1a04",
			ray: "#b45309", drop: "#1d7fd1", brick: "#9a3412",
		}
	}
	return palette{
		dome: [4]string{"#7b808a", "#8a5a14", "#f5a623", "#ffc766"},
		base: "#5b6068", handle: "#8d939c", sclera: "#fff8e8", ink: "#1a1206",
		ray: "#ffc766", drop: "#4fb3ff", brick: "#c2410c",
	}
}

const eyeLX, eyeRX = 78, 122

func eye(inner string) string {
	return `<g class="glim-eye">` + inner + `</g>`
}

func ball(cx, r, pupil, dx int, p palette) string {
	return `<circle cx="` + strconv.Itoa(cx) + `" cy="104" r="` + strconv.Itoa(r) + `" fill="` + p.sclera + `"/>` +
		`<circle cx="` + strconv.Itoa(cx+dx) + `" cy="104" r="` + strconv.Itoa(pupil) + `" fill="` + p.ink + `"/>`
}

func arc(cx, y, ctrl int, p palette) string {
	d := "M" + strconv.Itoa(cx-10) + " " + strconv.Itoa(y) + " Q" + strconv.Itoa(cx) + " " + strconv.Itoa(ctrl) + " " + strconv.Itoa(cx+10) + " " + strconv.Itoa(y)
	return `<path d="` + d + `" fill="none" stroke="` + p.ink + `" stroke-width="4" stroke-linecap="round"/>`
}

func eyes(mood string, p palette) string {
	var l, r string
	switch mood {
	case MoodOff, MoodAsleep: // closed
		l, r = arc(eyeLX, 104, 112, p), arc(eyeRX, 104, 112, p)
	case MoodWorking: // open, looking aside
		l, r = ball(eyeLX, 11, 5, 4, p), ball(eyeRX, 11, 5, 4, p)
	case MoodDone: // two upward arcs
		l, r = arc(eyeLX, 108, 94, p), arc(eyeRX, 108, 94, p)
	case MoodYourTurn: // open, one raised brow
		l, r = ball(eyeLX, 11, 5, 0, p), ball(eyeRX, 11, 5, 0, p)
	case MoodBlocked: // wide open
		l, r = ball(eyeLX, 15, 4, 0, p), ball(eyeRX, 15, 4, 0, p)
	case MoodBored: // half closed
		l = `<path d="M67 104 A11 11 0 0 0 89 104 Z" fill="` + p.sclera + `"/><circle cx="78" cy="108" r="4" fill="` + p.ink + `"/><path d="M66 104 L90 104" stroke="` + p.ink + `" stroke-width="4" stroke-linecap="round"/>`
		r = `<path d="M111 104 A11 11 0 0 0 133 104 Z" fill="` + p.sclera + `"/><circle cx="122" cy="108" r="4" fill="` + p.ink + `"/><path d="M110 104 L134 104" stroke="` + p.ink + `" stroke-width="4" stroke-linecap="round"/>`
	case MoodSweating: // wide open, small pupils
		l, r = ball(eyeLX, 14, 3, 0, p), ball(eyeRX, 14, 3, 0, p)
	}
	out := eye(l) + eye(r)
	if mood == MoodYourTurn {
		out += `<path d="M110 84 L134 78" fill="none" stroke="` + p.ink + `" stroke-width="4" stroke-linecap="round"/>`
	}
	return out
}

func mouth(mood string, p palette) string {
	stroke := func(d string) string {
		return `<path d="` + d + `" fill="none" stroke="` + p.ink + `" stroke-width="4" stroke-linecap="round"/>`
	}
	switch mood {
	case MoodWorking:
		return stroke("M90 130 Q100 138 110 130")
	case MoodDone:
		return stroke("M84 128 Q100 144 116 128")
	case MoodYourTurn:
		return stroke("M92 132 L108 132")
	case MoodBlocked:
		return `<circle cx="100" cy="134" r="8" fill="` + p.ink + `"/>`
	case MoodBored:
		return stroke("M88 134 L112 134")
	case MoodSweating:
		return stroke("M86 138 Q100 124 114 138")
	case MoodAsleep:
		return stroke("M95 133 L105 133")
	}
	return stroke("M92 133 L108 133") // off
}

func extras(mood string, p palette) string {
	ray := func(x1, y1, x2, y2 int) string {
		return `<line class="glim-ray" x1="` + strconv.Itoa(x1) + `" y1="` + strconv.Itoa(y1) + `" x2="` + strconv.Itoa(x2) +
			`" y2="` + strconv.Itoa(y2) + `" stroke="` + p.ray + `" stroke-width="5" stroke-linecap="round"/>`
	}
	switch mood {
	case MoodAsleep:
		return `<text class="glim-z" x="150" y="44" font-family="sans-serif" font-size="20" font-weight="bold" fill="` + p.ray + `">z z</text>`
	case MoodYourTurn:
		return ray(30, 78, 14, 68) + ray(170, 78, 186, 68)
	case MoodBlocked:
		return ray(30, 78, 14, 68) + ray(26, 100, 8, 98) + ray(30, 122, 14, 130) +
			ray(170, 78, 186, 68) + ray(174, 100, 192, 98) + ray(170, 122, 186, 130)
	case MoodSweating:
		return `<path class="glim-drop" d="M150 62 Q160 80 150 90 Q140 80 150 62 Z" fill="` + p.drop + `"/>`
	}
	return ""
}

// brickWall stands n bricks beside Glim, six to a row, growing upward from
// the ground line of the feet.
func brickWall(n int, p palette) string {
	var b strings.Builder
	b.WriteString(`<g class="glim-wall">`)
	for i := 0; i < n; i++ {
		x := 192 + (i%6)*21
		y := 184 - (i/6+1)*11
		b.WriteString(`<rect class="glim-brick" x="` + strconv.Itoa(x) + `" y="` + strconv.Itoa(y) +
			`" width="19" height="9" rx="1" fill="` + p.brick + `"/>`)
	}
	b.WriteString(`</g>`)
	return b.String()
}
