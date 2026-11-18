package lamp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"burnmon/internal/fsutil"
	"burnmon/internal/timefmt"
)

// The week card (lane scope X3): two SVG pictures of the running ISO week, one
// for a story (1080 by 1920) and one wide (1200 by 630). Each shows Glim, a
// wall of one brick per 10 minutes the agents waited on the person (at most
// 60) and three numbers. It holds no project, title or path: its only input
// is the three totals of a Week.

const (
	cardBrickSeconds = 600
	cardMaxBricks    = 60
	cardFooter       = "BurnMon Dev, measured while the lamp was on"
	cardRepo         = "github.com/siteoffice/burnmon"
)

// Week is the three totals of one ISO week, local time, Monday to Sunday.
type Week struct {
	Year, Number   int
	WaitSeconds    int64 // agent_wait_seconds: how long the agents waited on the person
	LongestSeconds int64 // the largest longest_wait_seconds of a day
	OnSeconds      int64 // lamp_on_seconds
	Demo           bool  // made-up numbers, marked "demo" on the card
}

// WeekOf sums the ledger days of the ISO week that holds now.
func WeekOf(days map[string]DayRec, now time.Time) Week {
	y, n := now.ISOWeek()
	w := Week{Year: y, Number: n}
	monday := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location()).
		AddDate(0, 0, -((int(now.Weekday()) + 6) % 7))
	for i := 0; i < 7; i++ {
		d, ok := days[monday.AddDate(0, 0, i).Format(ledgerDayLayout)]
		if !ok {
			continue
		}
		w.WaitSeconds += d.AgentWaitSeconds
		w.OnSeconds += d.LampOnSeconds
		w.LongestSeconds = max(w.LongestSeconds, d.LongestWaitSeconds)
	}
	return w
}

// DemoWeek is the week of now with fixed made-up numbers, for --demo.
func DemoWeek(now time.Time) Week {
	y, n := now.ISOWeek()
	return Week{Year: y, Number: n, WaitSeconds: 15120, LongestSeconds: 2280, OnSeconds: 111600, Demo: true}
}

// CardFileNames are the two file names of a week.
func CardFileNames(w Week) (story, wide string) {
	base := fmt.Sprintf("glim-week-%d-W%02d-", w.Year, w.Number)
	return base + "story.svg", base + "wide.svg"
}

// WriteCards writes both cards into an existing folder and returns the two
// file names, story first.
func WriteCards(dir string, w Week, lang string) ([]string, error) {
	fi, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("the folder %s does not exist", dir)
	}
	if !fi.IsDir() {
		return nil, errors.New(dir + " is not a folder")
	}
	story, wide := CardFileNames(w)
	for name, body := range map[string]string{story: CardStory(w, lang), wide: CardWide(w, lang)} {
		if err := fsutil.WriteFileAtomic(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			return nil, err
		}
	}
	return []string{story, wide}, nil
}

// CardStory is the 1080 by 1920 card.
func CardStory(w Week, lang string) string {
	return card(w, lang, cardLayout{
		w: 1080, h: 1920, glimX: 340, glimY: 330, glimH: 400, anchor: "middle",
		titleX: 540, titleY: 230, titleSize: 62, weekY: 150,
		numX: [3]int{540, 540, 540}, numY: [3]int{930, 1140, 1350}, numSize: 120, labelSize: 38,
		wallX: 320, wallY: 1790, perRow: 10, bw: 44, bh: 20, gap: 4,
		footY: 1850, footSize: 28,
	})
}

// CardWide is the 1200 by 630 card.
func CardWide(w Week, lang string) string {
	return card(w, lang, cardLayout{
		w: 1200, h: 630, glimX: 50, glimY: 150, glimH: 300, anchor: "start",
		titleX: 400, titleY: 120, titleSize: 54, weekY: 60,
		numX: [3]int{400, 690, 940}, numY: [3]int{300, 300, 300}, numSize: 76, labelSize: 26,
		wallX: 400, wallY: 560, perRow: 30, bw: 22, bh: 12, gap: 3,
		footY: 590, footSize: 20,
	})
}

type cardLayout struct {
	w, h                int
	glimX, glimY, glimH int
	anchor              string
	titleX, titleY      int
	titleSize, weekY    int
	numX, numY          [3]int
	numSize, labelSize  int
	wallX, wallY        int
	perRow, bw, bh, gap int
	footY, footSize     int
}

type cardWords struct{ sentence, week, wait, longest, on, brick string }

func wordsFor(lang string) cardWords {
	if lang == "nl" {
		return cardWords{"Mijn agents wachtten op mij.", "Week", "wachttijd", "langste wacht", "lamp aan", "1 steen is 10 min wachten"}
	}
	return cardWords{"My agents waited on me.", "Week", "waiting time", "longest wait", "lamp on", "1 brick is 10 min of waiting"}
}

const (
	cardBG    = "#14110a"
	cardAmber = "#f5a623"
	cardText  = "#fff8e8"
	cardMuted = "#c9b995"
	cardBrick = "#c2410c"
)

func card(w Week, lang string, l cardLayout) string {
	t := wordsFor(lang)
	var b strings.Builder
	num := func(i int, sec int64, label string) {
		x := strconv.Itoa(l.numX[i])
		b.WriteString(`<text class="card-number" x="` + x + `" y="` + strconv.Itoa(l.numY[i]) + `" text-anchor="` + l.anchor +
			`" font-family="sans-serif" font-size="` + strconv.Itoa(l.numSize) + `" font-weight="bold" fill="` + cardAmber + `">` +
			timefmt.Short(sec) + `<title>` + timefmt.Exact(sec) + `</title></text>`)
		b.WriteString(`<text class="card-label" x="` + x + `" y="` + strconv.Itoa(l.numY[i]+l.labelSize+12) + `" text-anchor="` + l.anchor +
			`" font-family="sans-serif" font-size="` + strconv.Itoa(l.labelSize) + `" fill="` + cardMuted + `">` + xmlText(label) + `</text>`)
	}
	text := func(cls string, x, y, size int, anchor, fill, s string) {
		b.WriteString(`<text class="` + cls + `" x="` + strconv.Itoa(x) + `" y="` + strconv.Itoa(y) + `" text-anchor="` + anchor +
			`" font-family="sans-serif" font-size="` + strconv.Itoa(size) + `" fill="` + fill + `">` + xmlText(s) + `</text>`)
	}
	b.WriteString(svgHead + `width="` + strconv.Itoa(l.w) + `" height="` + strconv.Itoa(l.h) + `" viewBox="0 0 ` +
		strconv.Itoa(l.w) + ` ` + strconv.Itoa(l.h) + `">`)
	b.WriteString(`<rect width="` + strconv.Itoa(l.w) + `" height="` + strconv.Itoa(l.h) + `" fill="` + cardBG + `"/>`)
	b.WriteString(glimSized(MoodBored, 0, "dark", `x="`+strconv.Itoa(l.glimX)+`" y="`+strconv.Itoa(l.glimY)+`"`, float64(l.glimH)))
	text("card-week", l.titleX, l.weekY, l.labelSize, l.anchor, cardMuted, fmt.Sprintf("%s %d, %d", t.week, w.Number, w.Year))
	text("card-title", l.titleX, l.titleY, l.titleSize, l.anchor, cardText, t.sentence)
	num(0, w.WaitSeconds, t.wait)
	num(1, w.LongestSeconds, t.longest)
	num(2, w.OnSeconds, t.on)
	bricks := min(int(w.WaitSeconds/cardBrickSeconds), cardMaxBricks)
	b.WriteString(`<g class="card-wall">`)
	for i := 0; i < bricks; i++ {
		x := l.wallX + (i%l.perRow)*(l.bw+l.gap)
		y := l.wallY - (i/l.perRow+1)*(l.bh+l.gap)
		b.WriteString(`<rect class="card-brick" x="` + strconv.Itoa(x) + `" y="` + strconv.Itoa(y) + `" width="` + strconv.Itoa(l.bw) +
			`" height="` + strconv.Itoa(l.bh) + `" rx="2" fill="` + cardBrick + `"/>`)
	}
	b.WriteString(`</g>`)
	foot := l.titleX
	text("card-footer", foot, l.footY, l.footSize, l.anchor, cardMuted, cardFooter)
	text("card-repo", foot, l.footY+l.footSize+6, l.footSize, l.anchor, cardMuted, cardRepo)
	if l.anchor == "middle" {
		text("card-brick-key", foot, l.footY-l.footSize-14, l.footSize, l.anchor, cardMuted, t.brick)
	}
	if w.Demo {
		b.WriteString(`<text class="card-demo" x="` + strconv.Itoa(l.w-30) + `" y="` + strconv.Itoa(l.weekY) +
			`" text-anchor="end" font-family="sans-serif" font-size="` + strconv.Itoa(l.titleSize) + `" font-weight="bold" fill="` + cardAmber + `">demo</text>`)
	}
	b.WriteString("</svg>\n")
	return b.String()
}
