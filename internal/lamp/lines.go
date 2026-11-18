package lamp

import (
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"burnmon/internal/waits"
)

// LineDef is one sentence of Glim in both languages. The set is the table
// "The lines" of the lamp spec; the code adds no line and changes none.
type LineDef struct {
	Mood string
	EN   string
	NL   string
}

var lineTable = []LineDef{
	{MoodOff, "BurnMon is not running.", "BurnMon draait niet."},
	{MoodAsleep, "No agents running. I am off.", "Geen agents actief. Ik ben uit."},
	{MoodAsleep, "Quiet site. Lamp off.", "Stille bouwplaats. Lamp uit."},
	{MoodAsleep, "Nothing runs. Nothing burns.", "Niets draait. Niets brandt."},
	{MoodAsleep, "Wake me when you start something.", "Maak me wakker als je iets start."},
	{MoodWorking, "{count} agents at work. Nothing for you.", "{count} agents aan het werk. Niets voor jou."},
	{MoodWorking, "All quiet. Go get coffee.", "Alles rustig. Haal koffie."},
	{MoodWorking, "They are busy. You are free.", "Zij zijn bezig. Jij bent vrij."},
	{MoodWorking, "Nothing waits on you. Enjoy it.", "Niets wacht op jou. Geniet ervan."},
	{MoodDone, "Something is ready to close. Nice.", "Er kan iets dicht. Netjes."},
	{MoodDone, "Finished. Somebody should look at it.", "Klaar. Iemand zou ernaar moeten kijken."},
	{MoodDone, "Done. I did nothing and I am still proud.", "Klaar. Ik deed niets en ben toch trots."},
	{MoodYourTurn, "{who} waits on you. No rush.", "{who} wacht op je. Geen haast."},
	{MoodYourTurn, "{who} is done thinking. Your move.", "{who} is klaar met denken. Jij bent."},
	{MoodYourTurn, "Your turn. {who} cannot press Enter for you.", "Jouw beurt. {who} kan niet voor jou op Enter drukken."},
	{MoodYourTurn, "A question waits. It is probably yes.", "Er wacht een vraag. Het is vast ja."},
	{MoodBlocked, "{who} is stuck. It needs a human. That is you.", "{who} zit vast. Er is een mens nodig. Dat ben jij."},
	{MoodBlocked, "Blocked. Nobody moves until you do.", "Geblokkeerd. Niemand beweegt tot jij beweegt."},
	{MoodBlocked, "{count} things are blocked. Pick one.", "{count} dingen zitten vast. Kies er een."},
	{MoodBlocked, "A gate waits. Gates do not open themselves.", "Er wacht een gate. Gates gaan niet vanzelf open."},
	{MoodBored, "{who} waits {minutes} minutes. It is very patient.", "{who} wacht {minutes} minuten. Heel geduldig."},
	{MoodBored, "{minutes} minutes. I started a wall.", "{minutes} minuten. Ik ben maar gaan metselen."},
	{MoodBored, "Still your turn. The wall has {minutes} bricks.", "Nog steeds jouw beurt. De muur telt {minutes} stenen."},
	{MoodBored, "{who} could have shipped twice by now.", "{who} had al twee keer kunnen opleveren."},
	{MoodSweating, "{rate} tokens a minute. Bold.", "{rate} tokens per minuut. Gedurfd."},
	{MoodSweating, "The meter is running. Fast.", "De meter loopt. Hard."},
	{MoodSweating, "Three times the usual burn. I hope it is worth it.", "Drie keer het normale verbruik. Hoop dat het wat oplevert."},
	{MoodSweating, "Somebody is reading the whole repo again.", "Iemand leest weer de hele repo."},
}

// gateLine is used only when the top row is a Foreman gate.
const gateLine = "A gate waits. Gates do not open themselves."

// wallLine is used only up to 30 minutes: past that the plank is full.
const wallLine = "Still your turn. The wall has {minutes} bricks."

// singularTable holds the two lines used only when the count is exactly 1.
// A line with {count} in lineTable is used only from 2. These join the pick
// at count 1; the pick formula and the per-mood counts of lineTable stay.
var singularTable = []LineDef{
	{MoodWorking, "1 agent at work. Nothing for you.", "1 agent aan het werk. Niets voor jou."},
	{MoodBlocked, "One thing is blocked. Guess whose turn.", "Een ding zit vast. Raad eens wie er aan zet is."},
}

// ActLine is the sentence of an act, or "" when the act has none. The parts:
// slip has start, air and back; stamp has a and b (the page alternates them
// by act_seq); welcome takes the count of waiting items; coffee takes the
// local hour, 10 or 15.
func ActLine(act, part, lang string, count, hour int) string {
	en, nl := "", ""
	switch {
	case act == "slip" && part == "start":
		en, nl = "I am not bored. I am testing the rail.", "Ik verveel me niet. Ik test de rail."
	case act == "slip" && part == "air":
		en, nl = "Whoa.", "Ho."
	case act == "slip" && part == "back":
		en, nl = "You saw nothing.", "Je hebt niets gezien."
	case act == "stamp" && part == "a":
		en, nl = "Stamped. Back to work.", "Gestempeld. Weer aan het werk."
	case act == "stamp" && part == "b":
		en, nl = "Answered. That was all it took.", "Beantwoord. Meer was het niet."
	case act == "wake":
		en, nl = "Somebody started something.", "Iemand is iets begonnen."
	case act == "welcome" && count == 1:
		en, nl = "Welcome back. One thing waited.", "Welkom terug. Er wachtte een ding."
	case act == "welcome" && count >= 2:
		n := strconv.Itoa(count)
		en, nl = "Welcome back. "+n+" things waited.", "Welkom terug. Er wachtten "+n+" dingen."
	case act == "coffee" && hour == 10:
		en, nl = "Ten o'clock. Coffee.", "Tien uur. Koffie."
	case act == "coffee" && hour == 15:
		en, nl = "Three o'clock. Coffee. The agents get none.", "Drie uur. Koffie. De agents krijgen niets."
	}
	if lang == "nl" {
		return nl
	}
	return en
}

// Lines returns a copy of the whole table.
func Lines() []LineDef { return append([]LineDef(nil), lineTable...) }

// whoFor is the name a line uses for an item id. Vendor names stay as they
// are; the unnamed agent is "An agent" in English and "Een agent" in Dutch.
func whoFor(id, lang string) string {
	w := Who(id)
	if w == anAgent && lang == "nl" {
		return "Een agent"
	}
	return w
}

// Line picks the sentence for the state in the language of the settings, or
// "" when lamp_lines is off or no line fits. The pick is
// (fnv32a(top row id + mood) + unix minutes / 5) mod count over the lines
// whose placeholders can all be filled, so it changes every 5 minutes.
func Line(st State, s Settings, now time.Time) string {
	if !s.Lines {
		return ""
	}
	topID, topKind := "", ""
	if len(st.Rows) > 0 && st.Rows[0].Kind != RowKindMore {
		topID, topKind = st.Rows[0].ID, st.Rows[0].Kind
	}
	vals := map[string]string{}
	if topID != "" {
		vals["{who}"] = whoFor(topID, s.Language)
	}
	switch st.Mood {
	case MoodWorking:
		if st.Agents >= 2 { // the singular rule: {count} only from 2
			vals["{count}"] = strconv.Itoa(st.Agents)
		}
	case MoodBlocked:
		if st.Counts.Blocked >= 2 {
			vals["{count}"] = strconv.Itoa(st.Counts.Blocked)
		}
	}
	if st.LongestWaitMin >= 1 {
		vals["{minutes}"] = strconv.Itoa(st.LongestWaitMin)
	}
	if st.BurnRate > 0 {
		vals["{rate}"] = rate(st.BurnRate)
	}

	var fit []string
	for _, l := range lineTable {
		if l.Mood != st.Mood {
			continue
		}
		text := l.EN
		if s.Language == "nl" {
			text = l.NL
		}
		// a gate top row gets the gate line and no other; any other row never does
		if st.Mood == MoodBlocked && (l.EN == gateLine) != (topKind == waits.KindGate) {
			continue
		}
		if l.EN == wallLine && st.LongestWaitMin > maxBricks {
			continue
		}
		ok := true
		for _, p := range placeholders {
			if strings.Contains(text, p) {
				v, have := vals[p]
				if !have {
					ok = false
					break
				}
				text = strings.ReplaceAll(text, p, v)
			}
		}
		if ok {
			fit = append(fit, text)
		}
	}
	if (st.Mood == MoodWorking && st.Agents == 1) || (st.Mood == MoodBlocked && st.Counts.Blocked == 1 && topKind != waits.KindGate) {
		for _, l := range singularTable {
			if l.Mood == st.Mood {
				if s.Language == "nl" {
					fit = append(fit, l.NL)
				} else {
					fit = append(fit, l.EN)
				}
			}
		}
	}
	if len(fit) == 0 {
		return ""
	}
	h := fnv.New32a()
	h.Write([]byte(topID + st.Mood))
	return fit[int((uint64(h.Sum32())+uint64(now.Unix()/60/5))%uint64(len(fit)))]
}

var placeholders = []string{"{who}", "{minutes}", "{count}", "{rate}"}
