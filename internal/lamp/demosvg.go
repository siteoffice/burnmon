package lamp

import (
	"strconv"
	"strings"
)

// demoKF is one stop of an act's motion: how far into the act (0 to 1) and
// the CSS transform Glim has there.
type demoKF struct {
	f float64
	t string
}

// demoMoves is the motion of each act on Glim's wrapper, from the Definitions
// of the lane. The wrapper turns about the point where the cord meets the
// ticker. Coffee and peek start and end above the picture.
var demoMoves = map[string][]demoKF{
	ActAlarm:   {{0, "none"}, {.1, "rotate(18deg)"}, {.25, "rotate(-16deg)"}, {.4, "rotate(12deg)"}, {.55, "rotate(-8deg)"}, {.75, "rotate(3deg)"}, {1, "none"}},
	ActKnock:   {{0, "none"}, {.15, "rotate(-4deg)"}, {.3, "none"}, {.6, "rotate(-4deg)"}, {.75, "none"}, {1, "none"}},
	ActStamp:   {{0, "none"}, {.2, "translateY(-10px)"}, {.4, "none"}, {.6, "translateY(-10px)"}, {.8, "none"}, {1, "none"}},
	ActWelcome: {{0, "none"}, {.15, "translateY(-8px)"}, {.3, "none"}, {.5, "rotate(6deg)"}, {.7, "rotate(-6deg)"}, {.9, "none"}, {1, "none"}},
	ActWake:    {{0, "translateY(-90px)"}, {.5, "none"}, {.7, "translateY(-8px)"}, {1, "none"}},
	ActSlip: {{0, "none"}, {.3, "translateX(110px)"}, {.4, "translateX(110px)"}, {.55, "translate(110px,50px) rotate(25deg)"},
		{.7, "translateX(110px)"}, {1, "none"}},
	ActCoffee: {{0, "translateY(-100px)"}, {.2, "none"}, {.4, "rotate(-6deg)"}, {.55, "none"}, {.7, "rotate(-6deg)"}, {.8, "none"}, {1, "translateY(-100px)"}},
	ActPeek: {{0, "translateY(-100px)"}, {.25, "translateY(-45px)"}, {.4, "translate(-14px,-45px)"},
		{.6, "translate(14px,-45px)"}, {.75, "translateY(-45px)"}, {1, "translateY(-100px)"}},
}

func demoPct(sec float64) string {
	return strconv.FormatFloat(sec*100/DemoSeconds, 'f', 3, 64) + "%"
}

// demoKeyframes writes one @keyframes per act over the whole loop: Glim stands
// still until the act starts, moves for its length and stands still again.
func demoKeyframes() string {
	var b strings.Builder
	for _, d := range demoActs {
		n := float64(d.start)
		l := ActSeconds(d.act)
		b.WriteString("@keyframes act-" + d.act + "{0%{transform:none}")
		for i, k := range demoMoves[d.act] {
			if i == 0 {
				b.WriteString(demoPct(n) + "{transform:none}")
			}
			b.WriteString(demoPct(n+k.f*l) + "{transform:" + k.t + "}")
		}
		b.WriteString("100%{transform:none}}")
		b.WriteString(".a-" + d.act + "{animation:act-" + d.act + " 48s linear infinite}")
	}
	return b.String()
}

const (
	demoW, demoH         = 320, 112
	demoGlimSize         = 80.0
	demoGlimX, demoGlimY = 120, 24
)

// demoActStart is the second an act starts at in the loop; 0 for no act.
func demoActStart(act string) int {
	for _, d := range demoActs {
		if d.act == act {
			return d.start
		}
	}
	return 0
}

// demoLine is the sentence on the ticker at a second: the line of the act that
// plays, or the line of the state.
func demoLine(sec int, st State) string {
	act, _ := DemoAct(sec)
	off := demoWrap(sec) - demoActStart(act)
	part := ""
	switch act {
	case ActSlip:
		part = "back"
		if off < 2 {
			part = "start"
		} else if off < 4 {
			part = "air"
		}
	case ActStamp:
		part = "a"
	}
	hour := 9
	if act == ActCoffee {
		hour = 10
	}
	if l := ActLine(act, part, "en", st.Counted, hour); l != "" {
		return l
	}
	return st.Line
}

// demoStyle is the style of the demo picture: the frame switch, the acts and
// the short loops of the states that last.
func demoStyle() string {
	return `<style>.f{opacity:0;animation:show 48s step-end infinite}` +
		`@keyframes show{0%{opacity:1}2.05%{opacity:1}2.1%{opacity:0}100%{opacity:0}}` +
		`.mv{transform-box:view-box;transform-origin:160px 24px}` + demoKeyframes() +
		`@keyframes hold-sweat{0%{transform:translateX(-1.5px)}50%{transform:translateX(1.5px)}100%{transform:translateX(-1.5px)}}.hold-sweat{animation:hold-sweat .3s linear infinite}` +
		`@keyframes hold-sleep{0%{transform:rotate(-1.5deg)}50%{transform:rotate(1.5deg)}100%{transform:rotate(-1.5deg)}}.hold-sleep{animation:hold-sleep 4s ease-in-out infinite}` +
		`@keyframes hold-dots{0%{transform:translateX(-30px);opacity:0}20%{opacity:1}100%{transform:translateX(0);opacity:0}}.dotp{fill:#ffc766;animation:hold-dots 1.2s linear infinite}` +
		`.rayp{stroke:#ffc766;fill:none;animation:rayblink .4s step-end infinite}@keyframes rayblink{50%{opacity:.3}}</style>`
}

// demoFrame is the picture of one second: the sliver or the ticker with its
// line, Glim in a wrapper that carries the act and the state that lasts, and
// what an act adds (rays, mug, stamp mark, token dots).
func demoFrame(sec int, st State) string {
	var b strings.Builder
	act, _ := DemoAct(sec)
	set := DemoSettings(sec)
	quiet := Quiet(st, set)
	holds := Holds(st, set)
	b.WriteString(`<g class="f" style="animation-delay:` + strconv.Itoa(sec) + `s">`)
	if quiet && act != ActCoffee {
		b.WriteString(`<rect x="100" y="0" width="120" height="6" fill="#ffc766"/>`)
	} else {
		b.WriteString(`<path d="M10 0 H310 V20 a6 6 0 0 1 -6 6 H16 a6 6 0 0 1 -6 -6 Z" fill="#15110a"/>`)
		b.WriteString(`<g clip-path="url(#tc)" font-family="Segoe UI, sans-serif" font-size="10"><text x="20" y="17"><tspan fill="#ffc766" font-weight="600">` +
			xmlText(st.Headline) + `</tspan><tspan fill="#d4bf94"> ` + xmlText(demoLine(sec, st)) + `</tspan></text></g>`)
	}
	if !quiet || act == ActCoffee || act == ActPeek {
		cls := "mv"
		if act != "" {
			cls += " a-" + act
		}
		for _, h := range holds {
			if h == HoldSweat || h == HoldSleep {
				cls += " hold-" + h
			}
		}
		b.WriteString(`<g class="` + cls + `"><use href="#` + demoGlimID(st) + `" x="` + strconv.Itoa(demoGlimX) + `" y="` + strconv.Itoa(demoGlimY) + `"/>`)
		if act == ActAlarm {
			b.WriteString(`<path class="rayp" d="M120 60 l-10 -4 M120 72 l-12 0 M200 60 l10 -4 M200 72 l12 0 M130 36 l-6 -8 M190 36 l6 -8"/>`)
		}
		if act == ActCoffee {
			b.WriteString(`<rect x="188" y="82" width="9" height="8" rx="1.5" fill="#d3d1c7"/>`)
		}
		b.WriteString(`</g>`)
	}
	if act == ActStamp {
		b.WriteString(`<g font-family="Segoe UI, sans-serif" font-size="9" font-weight="600"><rect x="218" y="4" width="64" height="18" rx="3" fill="#15110a" stroke="#5dcaa5" stroke-width="1.5"/>` +
			`<text x="250" y="16" text-anchor="middle" fill="#5dcaa5">answered</text></g>`)
	}
	for _, h := range holds {
		if h == HoldDots {
			b.WriteString(`<circle class="dotp" cx="112" cy="58" r="2"/><circle class="dotp" cx="112" cy="68" r="2" style="animation-delay:.4s"/><circle class="dotp" cx="112" cy="78" r="2" style="animation-delay:.8s"/>`)
		}
	}
	b.WriteString(`<text x="290" y="108" fill="#ffc766" font-family="Segoe UI, sans-serif" font-size="9" font-weight="600">demo</text>`)
	b.WriteString(`</g>`)
	return b.String()
}

// DemoSVG is the moving picture of the README: the 48 seconds of the demo as
// one looping SVG, one frame of a second each, with the ticker, Glim and the
// acts. The acts are CSS keyframes over the whole loop on a wrapper around
// Glim, the states that last are short loops. No script and no outside
// resource. Glim is drawn once per mood in defs and placed with use.
func DemoSVG() string {
	var b strings.Builder
	b.WriteString(svgHead + `viewBox="0 0 ` + strconv.Itoa(demoW) + ` ` + strconv.Itoa(demoH) + `" width="640" height="224">`)
	b.WriteString(demoStyle())
	b.WriteString(`<defs><clipPath id="tc"><rect x="16" y="0" width="288" height="26"/></clipPath>`)
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
		b.WriteString(glimSized(st.Mood, bricks, "dark", `id="`+id+`"`, demoGlimSize))
	}
	b.WriteString(`</defs>`)
	b.WriteString(`<rect width="320" height="112" fill="#2b2823"/>`)
	for sec, st := range frame {
		b.WriteString(demoFrame(sec, st))
	}
	b.WriteString(`</svg>` + "\n")
	return b.String()
}

func demoGlimID(st State) string {
	if st.Mood == MoodBored {
		return "gl-bored-" + strconv.Itoa(st.WallBricks)
	}
	return "gl-" + st.Mood
}
