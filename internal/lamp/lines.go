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
		vals["{count}"] = strconv.Itoa(st.Agents)
		if st.Agents < 1 {
			delete(vals, "{count}")
		}
	case MoodBlocked:
		if st.Counts.Blocked >= 1 {
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
	if len(fit) == 0 {
		return ""
	}
	h := fnv.New32a()
	h.Write([]byte(topID + st.Mood))
	return fit[int((uint64(h.Sum32())+uint64(now.Unix()/60/5))%uint64(len(fit)))]
}

var placeholders = []string{"{who}", "{minutes}", "{count}", "{rate}"}
