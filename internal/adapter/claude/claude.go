// Package claude parses the JSONL session transcripts Claude Code and
// Cowork write, into schema.Events. Moved out of internal/scan/parse.go
// (v0.0.1) unchanged in its per-line logic; only the output shape changed,
// from a whole-file *scan.Session to incremental schema.Events.
package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"burnmon/internal/scan"
	"burnmon/internal/schema"
)

// Adapter implements adapter.Adapter for Claude Code and Cowork transcripts.
type Adapter struct{}

func (Adapter) Name() string { return "claude" }

func (Adapter) Roots() []string { return scan.DefaultSources() }

var tagNames = []string{
	"command-message", "command-name", "command-args", "system-reminder",
	"local-command-stdout", "local-command-stderr",
}

var tagRes = func() []*regexp.Regexp {
	rs := make([]*regexp.Regexp, 0, len(tagNames))
	for _, t := range tagNames {
		rs = append(rs, regexp.MustCompile(`(?s)<`+t+`>.*?</`+t+`>`))
	}
	return rs
}()

var cmdNameRe = regexp.MustCompile(`(?s)<command-name>(.*?)</command-name>`)
var anyTagRe = regexp.MustCompile(`<[^>]{1,40}>`)

func collapseWS(s string) string { return strings.Join(strings.Fields(s), " ") }

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// cleanTitle turns a first user message into something a human recognises
// in a list. Slash commands arrive wrapped in tags; the command name is the
// useful bit.
func cleanTitle(text string) string {
	if text == "" {
		return ""
	}
	cmd := cmdNameRe.FindStringSubmatch(text)
	stripped := text
	for _, re := range tagRes {
		stripped = re.ReplaceAllString(stripped, " ")
	}
	stripped = anyTagRe.ReplaceAllString(stripped, " ")
	stripped = collapseWS(stripped)
	if cmd != nil {
		name := strings.TrimLeft(collapseWS(cmd[1]), "/")
		return truncRunes(strings.TrimSpace("/"+name+" "+stripped), 110)
	}
	return truncRunes(stripped, 110)
}

func extractText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, b := range v {
			switch bb := b.(type) {
			case map[string]any:
				if bb["type"] == "text" {
					if t, ok := bb["text"].(string); ok {
						parts = append(parts, t)
					}
				}
			case string:
				parts = append(parts, bb)
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// classifySurface maps a trail file to the report's surface vocabulary.
// cowork and chat (both Claude Desktop) merge into "desktop"; code_agent
// (worktree/sprint agent runs) keeps its own bucket since it is a run-type
// distinction, not a runtime surface. See the Step 1 plan's "Decisions
// locked in" section.
func classifySurface(path, cwd string) string {
	p := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	c := strings.ToLower(strings.ReplaceAll(cwd, "\\", "/"))
	if strings.Contains(p, "local-agent-mode-sessions") || strings.Contains(c, "local-agent-mode-sessions") {
		return "desktop"
	}
	if strings.Contains(c, ".worktrees") && strings.Contains(c, "sprint") {
		return "code_agent"
	}
	return "cli"
}

// agentFor returns the report's Agent label for a classified surface (F4):
// "cowork" for the Claude Desktop/Cowork surfaces, "claude-code" otherwise
// (cli, code_agent). Every transcript previously got Agent: "claude-code"
// regardless of surface, so the Cowork card wrongly read "CLAUDE-CODE ·
// DESKTOP" instead of "COWORK · DESKTOP".
func agentFor(surface string) string {
	if surface == "desktop" || surface == "cowork" {
		return "cowork"
	}
	return "claude-code"
}

func asInt(v any) int64 {
	if f, ok := v.(float64); ok {
		return int64(f)
	}
	return 0
}

// skillTag mirrors scan.ToolGroup's Skill re-tagging: a Skill tool call's
// own name is "Skill" for every skill, the real skill sits in the input.
// It re-tags to "skill:"+name, keeping the prefix; scan.ToolGroup strips
// it later, at report-aggregation time, not here.
func skillTag(name string, input map[string]any) string {
	if name != "Skill" {
		return name
	}
	if sk, ok := input["skill"].(string); ok && sk != "" {
		return "skill:" + sk
	}
	return name
}

// pendingToolCall is one tool_use block awaiting its tool_result, tracked by
// the block's own id (the only field a later "user" line's tool_result
// carries to match it back).
type pendingToolCall struct {
	turn        string
	tool        string
	at          string
	inputBytes  int64
	resultBytes *int64
	path        string
}

// filePathFromInput extracts I3's best-effort file path from a Claude
// tool_use block's own input: Read/Edit/Write carry "file_path",
// NotebookEdit carries "notebook_path". "" when the tool named no file (a
// shell/exec call, a search) or input carried neither key.
func filePathFromInput(input map[string]any) string {
	if p, ok := input["file_path"].(string); ok && p != "" {
		return p
	}
	if p, ok := input["notebook_path"].(string); ok && p != "" {
		return p
	}
	return ""
}

// contentByteLen approximates a tool_result block's byte size: content is
// usually a plain string (its own length), occasionally a block list (the
// sum of its "text" fields' lengths), matching extractText's own shapes.
func contentByteLen(content any) int64 {
	switch v := content.(type) {
	case string:
		return int64(len(v))
	case []any:
		var n int64
		for _, item := range v {
			if b, ok := item.(map[string]any); ok {
				if t, ok := b["text"].(string); ok {
					n += int64(len(t))
				}
			}
		}
		return n
	}
	return 0
}

// Parse reads path from byte offset from to EOF and returns one Event per
// distinct API call turn found (deduped by requestId/uuid, largest
// output_tokens wins, exactly as v0.0.1's ParseSession did), one ToolCall per
// tool_use block (S2), plus the byte offset right after the last complete
// line consumed. A trailing partial line (the file still being written) is
// left for the next call.
func (Adapter) Parse(path string, from int64) ([]schema.Event, []schema.ToolCall, int64, error) {
	if scan.IsCloudDropPath(path) {
		return parseCloudDrop(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, from, err
	}
	defer f.Close()

	if from > 0 {
		if _, err := f.Seek(from, io.SeekStart); err != nil {
			return nil, nil, from, err
		}
	}

	cwd := ""
	title := ""
	parentID := ""
	type turn struct {
		key                      string
		ts                       string
		model                    string
		fresh, cacheW, cacheR, o int64
	}
	calls := map[string]*turn{}
	var order []string
	toolCounts := map[string]map[string]int64{}
	pendingCalls := map[string]*pendingToolCall{}
	var toolCallOrder []string
	// lateResults: results whose call was read in an earlier pass (a tool
	// that outlived one incremental read), returned as result-only rows.
	var lateResults []schema.ToolCall

	offset := from
	reader := bufio.NewReader(f)
	rowIdx := 0
	for {
		rawLine, readErr := reader.ReadString('\n')
		if readErr != nil {
			if readErr == io.EOF {
				// Either nothing left, or a trailing partial line (the file
				// is still being written). Either way it was not
				// newline-terminated, so it is not a complete line: leave it
				// unprocessed and do not advance offset past it.
				break
			}
			return nil, nil, from, readErr
		}
		lineLen := int64(len(rawLine))
		line := strings.TrimSpace(rawLine)
		if line == "" {
			offset += lineLen
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			offset += lineLen
			continue
		}
		offset += lineLen
		rowIdx++

		if cwd == "" {
			if c, ok := obj["cwd"].(string); ok && c != "" {
				cwd = c
			}
		}
		// A subagent's trail (<parent>\subagents\agent-<id>.jsonl) carries
		// the parent's sessionId and isSidechain true on every line (R6).
		if parentID == "" && obj["isSidechain"] == true {
			if p, ok := obj["sessionId"].(string); ok {
				parentID = p
			}
		}
		if obj["type"] == "user" {
			if msg, ok := obj["message"].(map[string]any); ok {
				if title == "" {
					title = cleanTitle(extractText(msg["content"]))
				}
				if content, ok := msg["content"].([]any); ok {
					for _, item := range content {
						block, ok := item.(map[string]any)
						if !ok || block["type"] != "tool_result" {
							continue
						}
						tuid, _ := block["tool_use_id"].(string)
						if tuid == "" {
							continue
						}
						n := contentByteLen(block["content"])
						if pc, ok := pendingCalls[tuid]; ok {
							pc.resultBytes = &n
						} else {
							lateResults = append(lateResults, schema.ToolCall{CallID: tuid, ResultBytes: &n})
						}
					}
				}
			}
		}
		if obj["type"] != "assistant" {
			continue
		}
		msg, ok := obj["message"].(map[string]any)
		if !ok {
			continue
		}
		lineTS, _ := obj["timestamp"].(string)

		if content, ok := msg["content"].([]any); ok {
			lineTools := map[string]int64{}
			tk := ""
			if s, ok := obj["requestId"].(string); ok && s != "" {
				tk = s
			} else if s, ok := obj["uuid"].(string); ok && s != "" {
				tk = s
			} else {
				tk = fmt.Sprintf("_toolrow%d", rowIdx)
			}
			for _, item := range content {
				block, ok := item.(map[string]any)
				if !ok || block["type"] != "tool_use" {
					continue
				}
				name, ok := block["name"].(string)
				if !ok || name == "" {
					continue
				}
				input, _ := block["input"].(map[string]any)
				lineTools[skillTag(name, input)]++

				if id, _ := block["id"].(string); id != "" {
					inputBytes := int64(0)
					if b, err := json.Marshal(input); err == nil {
						inputBytes = int64(len(b))
					}
					pendingCalls[id] = &pendingToolCall{turn: tk, tool: skillTag(name, input), at: lineTS, inputBytes: inputBytes, path: filePathFromInput(input)}
					toolCallOrder = append(toolCallOrder, id)
				}
			}
			if len(lineTools) > 0 {
				seen := toolCounts[tk]
				if seen == nil {
					seen = map[string]int64{}
					toolCounts[tk] = seen
				}
				for name, c := range lineTools {
					if c > seen[name] {
						seen[name] = c
					}
				}
			}
		}

		usage, ok := msg["usage"].(map[string]any)
		if !ok || len(usage) == 0 {
			continue
		}
		key := ""
		if s, ok := obj["requestId"].(string); ok && s != "" {
			key = s
		} else if s, ok := obj["uuid"].(string); ok && s != "" {
			key = s
		} else {
			key = fmt.Sprintf("_row%d", len(order))
		}
		model, _ := msg["model"].(string)
		ts, _ := obj["timestamp"].(string)
		if model == "" || model == "<synthetic>" {
			continue
		}
		t := &turn{
			key: key, ts: ts, model: model,
			fresh:  asInt(usage["input_tokens"]),
			cacheW: asInt(usage["cache_creation_input_tokens"]),
			cacheR: asInt(usage["cache_read_input_tokens"]),
			o:      asInt(usage["output_tokens"]),
		}
		prev := calls[key]
		if prev == nil {
			calls[key] = t
			order = append(order, key)
		} else if t.o >= prev.o {
			calls[key] = t
		}
	}

	surface := classifySurface(path, cwd)
	base := filepath.Base(path)
	sessionID := strings.TrimSuffix(base, filepath.Ext(base))
	if parentID == sessionID {
		parentID = ""
	}

	var events []schema.Event
	consumedToolKeys := map[string]bool{}
	for _, k := range order {
		t := calls[k]
		e := schema.Event{
			Vendor: "anthropic", Agent: agentFor(surface), Surface: surface,
			SessionID: sessionID, ParentID: parentID, RequestID: k, Project: cwd, Title: title,
			Model: t.model, Input: t.fresh, Output: t.o,
		}
		if t.cacheW > 0 {
			v := t.cacheW
			e.CacheWrite = &v
		}
		if t.cacheR > 0 {
			v := t.cacheR
			e.CacheRead = &v
		}
		if t.ts != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, t.ts); err == nil {
				e.At = parsed.UTC()
			}
		}
		if tools := toolCounts[k]; len(tools) > 0 {
			e.Tools = tools
			consumedToolKeys[k] = true
		}
		events = append(events, e)
	}

	// Orphan tool-only lines: a requestId/uuid whose tool_use blocks never
	// had a matching usage line anywhere in this read. v0.0.1 still summed
	// these into the session's Tools total (toolCounts was summed
	// unconditionally), so they become their own zero-token Events here,
	// keyed by their own (unique, never a calls[] key) request id.
	for k, tools := range toolCounts {
		if consumedToolKeys[k] {
			continue
		}
		events = append(events, schema.Event{
			Vendor: "anthropic", Agent: agentFor(surface), Surface: surface,
			SessionID: sessionID, ParentID: parentID, RequestID: k, Project: cwd, Title: title,
			Tools: tools,
		})
	}

	var toolCalls []schema.ToolCall
	for _, id := range toolCallOrder {
		pc := pendingCalls[id]
		var at time.Time
		if pc.at != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, pc.at); err == nil {
				at = parsed.UTC()
			}
		}
		toolCalls = append(toolCalls, schema.ToolCall{
			Vendor: "anthropic", Agent: agentFor(surface), SessionID: sessionID,
			CallID: id, Turn: pc.turn, Tool: pc.tool, At: at,
			InputBytes: pc.inputBytes, ResultBytes: pc.resultBytes, Path: pc.path,
		})
	}
	for _, r := range lateResults {
		r.Vendor, r.SessionID = "anthropic", sessionID
		toolCalls = append(toolCalls, r)
	}

	return events, toolCalls, offset, nil
}
