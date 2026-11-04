package toast

// osascriptPath is the macOS script runner. No undocumented API is involved.
const osascriptPath = "/usr/bin/osascript"

// osascriptLines are the constant script lines. User text never enters the
// script source: it travels as argv elements read by the script.
var osascriptLines = []string{
	"on run argv",
	"display notification (item 2 of argv) with title (item 1 of argv)",
	"end run",
}

// osascriptArgs returns the full argv after the binary: each script line
// preceded by -e, then "--", then title (item 1 of argv), then text (item 2).
// The "--" ends option parsing, so a text such as "-e" or "-l" cannot be read
// as a flag. Empty text returns nil (no call).
func osascriptArgs(text, title string) []string {
	if text == "" {
		return nil
	}
	args := make([]string, 0, 2*len(osascriptLines)+3)
	for _, l := range osascriptLines {
		args = append(args, "-e", l)
	}
	return append(args, "--", title, text)
}
