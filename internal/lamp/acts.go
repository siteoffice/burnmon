package lamp

import "time"

// The acts of Glim, in the order the first that is due plays.
const (
	ActAlarm   = "alarm"
	ActKnock   = "knock"
	ActStamp   = "stamp"
	ActWelcome = "welcome"
	ActWake    = "wake"
	ActSlip    = "slip"
	ActCoffee  = "coffee"
	ActPeek    = "peek"
)

// The states that last while their condition holds.
const (
	HoldWall  = "wall"
	HoldSweat = "sweat"
	HoldSleep = "sleep"
	HoldDots  = "dots"
)

const (
	slipMinutes   = 15
	welcomeIdle   = 300 // seconds without input before "welcome back"
	welcomeBack   = 5   // seconds since the last input that count as back
	peekEveryMins = 4
)

// actSeconds is the length of each act, from the Definitions of the lane.
var actSeconds = map[string]float64{
	ActAlarm: 2.6, ActKnock: 1.6, ActStamp: 2.4, ActWelcome: 2.4,
	ActWake: 1.6, ActSlip: 6.4, ActCoffee: 4, ActPeek: 4.4,
}

// ActSeconds is how long an act plays; 0 for a name that is no act.
func ActSeconds(act string) float64 { return actSeconds[act] }

// Played is what the engine remembers between two ticks so an act plays once
// where it must. The zero value is a lamp that has played nothing.
type Played struct {
	Seq         int64 // goes up by one each time an act starts
	SlipStretch bool  // slip already played in the current stretch of mood bored
	CoffeeDay   int   // yyyymmdd of the last coffee slot seen
	CoffeeSlot  int   // 10 or 15: the last slot seen on CoffeeDay
	PeekMinute  int64 // the last unix minute a tick was seen in
}

// ActInput is everything NextAct joins. Idle and PrevIdle are the seconds
// since the last keyboard or mouse input on this tick and on the one before.
type ActInput struct {
	Prev, Cur      State
	Now            time.Time
	Idle, PrevIdle int
	Settings       Settings
	Played         Played
}

// Quiet reports whether the lamp shows only the sliver: Glim is up, out of
// sight. That is a quiet mood (asleep, working) or no data, with lamp_quiet
// not "stay". With "stay" Glim always hangs.
func Quiet(st State, s Settings) bool {
	if s.Quiet == "stay" {
		return false
	}
	return st.Mood == MoodAsleep || st.Mood == MoodWorking || st.Mood == MoodOff
}

// quietMood is a mood the lamp has nothing to say in, whatever the setting.
func quietMood(mood string) bool { return mood == MoodAsleep || mood == MoodWorking }

// NextAct works out the one act that is due this tick, if any, the states that
// last, and what to remember. It is a pure function: no clock, no I/O.
//
// A tick with no earlier state (the lamp just started, or there was no data
// before) plays no act that compares two states: what is already there is not
// news. Mood off never plays an act. lamp_motion calm and off play none; the
// bookkeeping of peek, coffee and slip still runs.
func NextAct(in ActInput) (act string, holds []string, played Played) {
	p := in.Played
	cur, prev := in.Cur, in.Prev
	holds = Holds(cur, in.Settings)

	// Bookkeeping first: it must run on a tick that plays nothing.
	if cur.Mood != MoodBored {
		p.SlipStretch = false
	}
	minute := in.Now.Unix() / 60
	peekFirst := minute != p.PeekMinute
	p.PeekMinute = minute
	coffeeSlot := coffeeSlotAt(in.Now)
	coffeeFirst := false
	if coffeeSlot != 0 {
		day := in.Now.Year()*10000 + int(in.Now.Month())*100 + in.Now.Day()
		coffeeFirst = p.CoffeeDay != day || p.CoffeeSlot < coffeeSlot
		p.CoffeeDay, p.CoffeeSlot = day, coffeeSlot
	}

	if cur.Mood == MoodOff || in.Settings.Motion == "calm" || in.Settings.Motion == "off" {
		return "", holds, p
	}

	known := prev.Mood != "" && prev.Mood != MoodOff
	switch {
	case known && cur.Counts.Blocked > prev.Counts.Blocked:
		act = ActAlarm
	case known && cur.Counts.YourTurn > prev.Counts.YourTurn:
		act = ActKnock
	case known && stampDue(prev, cur):
		act = ActStamp
	case known && in.PrevIdle >= welcomeIdle && in.Idle < welcomeBack && cur.Counted >= 1:
		act = ActWelcome
	case known && prev.Agents == 0 && cur.Agents >= 1:
		act = ActWake
	case cur.Mood == MoodBored && cur.LongestWaitMin >= slipMinutes && !p.SlipStretch:
		act = ActSlip
		p.SlipStretch = true
	case coffeeFirst && cur.Counted == 0:
		act = ActCoffee
	case peekFirst && minute%peekEveryMins == 0 && quietMood(cur.Mood) && in.Settings.Quiet != "stay":
		act = ActPeek
	}
	if act != "" {
		p.Seq++
	}
	return act, holds, p
}

// stampDue: a counted item of the previous state is gone and the count went down.
func stampDue(prev, cur State) bool {
	if cur.Counted >= prev.Counted {
		return false
	}
	have := make(map[string]bool, len(cur.CountedIDs))
	for _, id := range cur.CountedIDs {
		have[id] = true
	}
	for _, id := range prev.CountedIDs {
		if !have[id] {
			return true
		}
	}
	return false
}

// coffeeSlotAt is 10 or 15 in the minute that starts at 10:00 or 15:00 local
// time, else 0.
func coffeeSlotAt(now time.Time) int {
	if now.Minute() == 0 && (now.Hour() == 10 || now.Hour() == 15) {
		return now.Hour()
	}
	return 0
}

// Holds lists the states that last while their condition holds. Mood off has
// none. The page decides how a state moves under lamp_motion.
func Holds(st State, s Settings) []string {
	var h []string
	if st.Mood == MoodOff {
		return h
	}
	switch st.Mood {
	case MoodBored:
		h = append(h, HoldWall)
	case MoodSweating:
		h = append(h, HoldSweat)
	case MoodAsleep:
		if s.Quiet == "stay" {
			h = append(h, HoldSleep)
		}
	}
	if st.Agents >= 1 && !Quiet(st, s) {
		h = append(h, HoldDots)
	}
	return h
}

// DotSeconds is the seconds between two token dots: clamp(30000 / tokens a
// minute, 0.3, 4). A rate of 0 or less gives the slowest, 4.
func DotSeconds(tokensPerMinute float64) float64 {
	if tokensPerMinute <= 0 {
		return 4
	}
	return min(max(30000/tokensPerMinute, 0.3), 4)
}
