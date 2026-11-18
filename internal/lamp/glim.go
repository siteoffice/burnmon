package lamp

import (
	"strconv"
	"strings"
)

// Glim draws the lamp character as a rig, an SVG string. It is a pure function
// of the mood, the wall bricks and the theme ("light"; anything else is dark):
// no image, no font file, no script, no outside resource.
//
// The picture is 200 high. It is 200 wide, or 320 wide when the wall of mood
// bored stands beside Glim (bricks above 0). Every mood draws every part (cord,
// handle, body, eyes with pupils and lids, mouth, two arms, rays, drop, z z,
// mug, dots, stamp mark, ring); one style element inside the picture keys the
// colours to the theme class on the root and shows the parts a mood holds, so a
// pose file stands alone. The page animates the parts by their class names and
// may override the colour variables. The shapes and coordinates are those of
// the glim alive prototype, moved by the cord length. The wall is a plank on two
// cords with the bricks on it, and shows 30+ from 30 bricks.
func Glim(mood string, bricks int, theme string) string {
	if _, ok := domeShade[mood]; !ok {
		mood = MoodAsleep
	}
	if theme != "light" {
		theme = "dark"
	}
	wall := mood == MoodBored && bricks > 0
	width, vbw := 200, 132
	if wall {
		width, vbw = 320, 211
	}
	var b strings.Builder
	b.WriteString(`<svg class="glim glim-` + mood + ` mood-` + mood + ` theme-` + theme + `" xmlns="http://www.w3.org/2000/svg" viewBox="-66 -6 ` +
		strconv.Itoa(vbw) + ` 132" width="` + strconv.Itoa(width) + `" height="200">`)
	b.WriteString(rigStyle)
	b.WriteString(`<g class="rig"><g class="swing"><g transform="translate(0,4)">`)
	b.WriteString(`<path class="cord" d="M0 -4 V23" stroke-width="2"/>`)
	b.WriteString(`<path class="cord handle" d="M-11 38 q11 -30 22 0" stroke-width="3" stroke-linecap="round"/>`)
	b.WriteString(rays())
	b.WriteString(`<g class="arm armL"><path class="cord" d="M-26 82 l-18 -7" stroke-width="3" stroke-linecap="round"/><circle class="mitt" cx="-46" cy="74" r="4.5"/></g>`)
	b.WriteString(`<g class="arm armR"><path class="cord" d="M26 82 l20 -6" stroke-width="3" stroke-linecap="round"/><circle class="mitt" cx="48" cy="75" r="4.5"/></g>`)
	b.WriteString(`<g class="body"><path class="dome glim-dome" d="M-26 90 v-26 a26 26 0 0 1 52 0 v26 z"/>`)
	b.WriteString(`<rect class="basep" x="-32" y="90" width="64" height="13" rx="4"/>`)
	b.WriteString(`<rect class="feet" x="-22" y="103" width="12" height="5" rx="2"/><rect class="feet" x="10" y="103" width="12" height="5" rx="2"/>`)
	b.WriteString(eyes(mood))
	b.WriteString(`<path class="mouth" d="` + mouthPath[mood] + `" fill="none" stroke="#412402" stroke-width="2" stroke-linecap="round"/></g>`)
	b.WriteString(`<path class="drop glim-drop" d="M34 48 q-5 8 0 11 q5 -3 0 -11 z"/>`)
	b.WriteString(`<text class="zz glim-z" x="34" y="46" font-size="13">z z</text>`)
	b.WriteString(`<g class="mug"><rect x="28" y="92" width="13" height="11" rx="2"/><path d="M41 94 q6 1 0 7" fill="none" stroke-width="2"/><path class="steam" d="M32 88 q-2 -3 0 -6 M37 88 q-2 -3 0 -6" fill="none" stroke-width="1.5"/></g>`)
	b.WriteString(`<g class="dots"><circle class="dot" cx="-60" cy="52" r="2"/><circle class="dot" cx="-60" cy="66" r="2"/><circle class="dot" cx="-60" cy="80" r="2"/></g>`)
	b.WriteString(`<g transform="translate(0,56) rotate(-8)"><g class="stampi"><rect x="-46" y="-14" width="92" height="28" rx="4" stroke-width="2.5"/><text x="0" y="5" text-anchor="middle" font-size="13">answered</text></g></g>`)
	b.WriteString(`<circle class="rip" cx="56" cy="74" r="9" stroke-width="2"/>`)
	b.WriteString(`</g></g></g>`)
	if wall {
		b.WriteString(brickWall(min(bricks, maxBricks), bricks >= maxBricks))
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// rigStyle keys the colours to the theme class and shows the parts a mood
// holds. Colours are variables, so the page can override them. The dome keeps
// its ambers in both themes.
const rigStyle = `<style>` +
	`.theme-light{--cord:#888780;--base:#5F5E5A;--feet:#888780;--ray:#EF9F27;--rip:#BA7517;--mitt:#B4B2A9;--zz:#854F0B;--stamp:#0F6E56;--plank:#8A6A3C;--brick:#9a3412;--panel:#FFFFFF}` +
	`.theme-dark{--cord:#888780;--base:#5F5E5A;--feet:#888780;--ray:#FAC775;--rip:#FAC775;--mitt:#D3D1C7;--zz:#FAC775;--stamp:#5DCAA5;--plank:#A98554;--brick:#c2410c;--panel:#14110D}` +
	`.cord{stroke:var(--cord);fill:none}.basep{fill:var(--base)}.feet{fill:var(--feet)}.mitt{fill:var(--mitt)}` +
	`.glim-ray{stroke:var(--ray);fill:none}.rip{stroke:var(--rip);fill:none}.zz{fill:var(--zz);font-weight:600}` +
	`.drop{fill:#378ADD}.mug rect{fill:var(--mitt)}.mug path{stroke:var(--mitt)}.dot{fill:var(--ray)}` +
	`.stampi rect{stroke:var(--stamp);fill:var(--panel)}.stampi text{fill:var(--stamp);font-weight:600}` +
	`.plankp{fill:var(--plank)}.glim-brick{fill:var(--brick)}.wallnote{fill:var(--zz);font-weight:600}` +
	`.dome{fill:#EF9F27}.mood-off .dome{fill:#888780}.mood-asleep .dome{fill:#854F0B}` +
	`.mood-your_turn .dome,.mood-blocked .dome{fill:#FAC775}.mood-sweating .dome{fill:#D85A30}` +
	`.lid{fill:#EF9F27}.mood-off .lid{fill:#888780}.mood-asleep .lid{fill:#854F0B}` +
	`.mood-your_turn .lid,.mood-blocked .lid{fill:#FAC775}.mood-sweating .lid{fill:#D85A30}` +
	`.lid{transform-box:fill-box;transform-origin:50% 0;transform:scaleY(0)}` +
	`.mood-off .lid,.mood-asleep .lid{transform:scaleY(1)}.mood-bored .lid{transform:scaleY(.5)}` +
	`.arm,.rays,.drop,.zz,.mug,.dots,.stampi,.rip,.brow{opacity:0}` +
	`.mood-your_turn .arm,.mood-bored .arm{opacity:1}` +
	`.mood-blocked .rays,.mood-your_turn .rays{opacity:1}.mood-your_turn .glim-ray:not(.r2){opacity:0}` +
	`.mood-sweating .drop,.mood-asleep .zz,.mood-your_turn .brow{opacity:1}` +
	`</style>`

// domeShade lists the eight moods.
var domeShade = map[string]int{
	MoodOff: 0, MoodAsleep: 1, MoodWorking: 2, MoodDone: 2,
	MoodYourTurn: 3, MoodBlocked: 3, MoodBored: 2, MoodSweating: 2,
}

// mouthPath is the mouth of each mood: smile, flat, sad or oh of the prototype
// (done is a wider smile).
var mouthPath = map[string]string{
	MoodOff:      "M-6 83 h12",
	MoodAsleep:   "M-5 83 h10",
	MoodWorking:  "M-7 82 q7 5 14 0",
	MoodDone:     "M-9 81 q9 9 18 0",
	MoodYourTurn: "M-6 83 h12",
	MoodBlocked:  "M-4 82 q4 5 8 0 q-4 -5 -8 0",
	MoodBored:    "M-6 83 h12",
	MoodSweating: "M-7 85 q7 -5 14 0",
}

// pupil is the pupil of each mood: the x offset and the radius.
type pupil struct{ dx, r int }

var pupilOf = map[string]pupil{
	MoodOff: {0, 3}, MoodAsleep: {0, 3}, MoodWorking: {3, 3}, MoodDone: {0, 3},
	MoodYourTurn: {0, 3}, MoodBlocked: {0, 2}, MoodBored: {0, 3}, MoodSweating: {0, 2},
}

func eyes(mood string) string {
	pu := pupilOf[mood]
	var b strings.Builder
	b.WriteString(`<g class="eyes">`)
	for _, cx := range []int{-11, 11} {
		x := strconv.Itoa(cx)
		b.WriteString(`<g class="glim-eye"><circle cx="` + x + `" cy="70" r="7" fill="#FFFFFF"/>` +
			`<circle class="pup" cx="` + strconv.Itoa(cx+pu.dx) + `" cy="70" r="` + strconv.Itoa(pu.r) + `" fill="#2C2C2A"/>` +
			`<rect class="lid" x="` + strconv.Itoa(cx-8) + `" y="62" width="16" height="16"/></g>`)
	}
	b.WriteString(`</g>`)
	// a raised brow of mood your_turn
	b.WriteString(`<path class="brow" d="M3 60 l16 -5" fill="none" stroke="#412402" stroke-width="2" stroke-linecap="round"/>`)
	return b.String()
}

// rays is the rig's six rays; the two on top are the short set (r2) that
// mood your_turn shows, mood blocked shows all six.
func rays() string {
	rs := []struct{ d, extra string }{
		{"M-38 56 l-13 -6", " r2"}, {"M-40 74 h-14", ""}, {"M-26 38 l-7 -11", ""},
		{"M38 56 l13 -6", " r2"}, {"M40 74 h14", ""}, {"M26 38 l7 -11", ""},
	}
	var b strings.Builder
	b.WriteString(`<g class="rays">`)
	for _, r := range rs {
		b.WriteString(`<path class="glim-ray` + r.extra + `" d="` + r.d + `" stroke-width="2.5" stroke-linecap="round"/>`)
	}
	b.WriteString(`</g>`)
	return b.String()
}

// brickWall hangs a plank on two cords beside Glim and stacks n bricks on it,
// six to a row. over adds the 30+ note above the full plank.
func brickWall(n int, over bool) string {
	var b strings.Builder
	b.WriteString(`<g class="plank glim-wall"><path class="cord" d="M76 -6 V104 M138 -6 V104" stroke-width="1.5"/>`)
	b.WriteString(`<rect class="plankp" x="72" y="104" width="70" height="5" rx="1"/><g class="bricks">`)
	for i := 0; i < n; i++ {
		x := 74 + (i%6)*11
		y := 104 - (i/6+1)*6
		b.WriteString(`<rect class="brick glim-brick" x="` + strconv.Itoa(x) + `" y="` + strconv.Itoa(y) + `" width="10" height="5.5" rx="1"/>`)
	}
	b.WriteString(`</g>`)
	if over {
		b.WriteString(`<text class="wallnote" x="107" y="68" text-anchor="middle" font-size="10">30+</text>`)
	}
	b.WriteString(`</g>`)
	return b.String()
}
