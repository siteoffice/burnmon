package cloudpull

// SwitchLabel and SwitchLines are the Settings text for the switch, the one
// source both apps show (burnmon.exe builds its overlay from them; BurnMon
// Dev's page.html carries the same strings, checked by a test).
const SwitchLabel = "Read Cowork (Cloud) sessions"

var SwitchLines = []string{
	"Starts your Claude Code CLI to copy your cloud sessions to this computer.",
	"Needs Claude Code installed and signed in to the same account.",
	"Reads cloud sessions you start in the Claude app on this computer. Sessions started from your phone, the browser or another computer are not read, and neither is subagent usage inside a cloud session.",
}
