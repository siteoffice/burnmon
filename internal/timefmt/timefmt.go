// Package timefmt writes a length of time the one way every ZND screen does
// (UI standard 17): "45 s", "12 min", "5 h", "19 d", with the exact value
// for the hover text.
package timefmt

import (
	"strconv"
	"strings"
)

// Short is the largest whole unit, rounded down: seconds under a minute,
// minutes under an hour, hours under a day, then days. A negative length is 0 s.
func Short(sec int64) string {
	switch {
	case sec < 60:
		if sec < 0 {
			sec = 0
		}
		return strconv.FormatInt(sec, 10) + " s"
	case sec < 3600:
		return strconv.FormatInt(sec/60, 10) + " min"
	case sec < 86400:
		return strconv.FormatInt(sec/3600, 10) + " h"
	}
	return strconv.FormatInt(sec/86400, 10) + " d"
}

// Exact lists every unit that is not zero, for example "1 h 4 min 12 s".
// Zero and a negative length are "0 s".
func Exact(sec int64) string {
	if sec <= 0 {
		return "0 s"
	}
	var parts []string
	for _, u := range []struct {
		size int64
		name string
	}{{86400, "d"}, {3600, "h"}, {60, "min"}, {1, "s"}} {
		if n := sec / u.size; n > 0 {
			parts = append(parts, strconv.FormatInt(n, 10)+" "+u.name)
			sec -= n * u.size
		}
	}
	return strings.Join(parts, " ")
}
