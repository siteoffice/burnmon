package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"

	"burnmon/internal/schema"
)

// cloudRow is one line of a Cowork (Cloud) drop file: the usage of one model
// request in a cloud Cowork session, written by the BurnMon cloud plugin from
// the session's own transcript. Whitelisted numbers and ids only; the drop
// never carries message text, tool input or paths
// (02_roadmap\2026-10-12_cowork_cloud_bucket_spec.md).
type cloudRow struct {
	Source        string `json:"source"`
	TS            string `json:"ts"`
	SessionID     string `json:"session_id"`
	RequestID     string `json:"request_id"`
	MessageID     string `json:"message_id"`
	Model         string `json:"model"`
	Agent         string `json:"agent"` // "main", or "agent-<id>" for a subagent's request
	InputTokens   int64  `json:"input_tokens"`
	OutputTokens  int64  `json:"output_tokens"`
	CacheCreation int64  `json:"cache_creation_input_tokens"`
	CacheRead     int64  `json:"cache_read_input_tokens"`
}

// parseCloudDrop reads a whole drop file. The plugin rewrites the file every
// turn, and latest.jsonl may switch to another session's rows, so a stored
// offset means nothing here: the file is always read from the top and the
// returned offset is its size. Upserts are keyed on (vendor, session_id,
// request_id), so re-reading rows already stored changes nothing.
func parseCloudDrop(path string) ([]schema.Event, []schema.ToolCall, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, 0, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, nil, 0, err
	}
	var events []schema.Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var r cloudRow
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		if !strings.HasPrefix(r.Source, "burnmon-cloud") || r.SessionID == "" {
			continue
		}
		key := r.RequestID
		if key == "" {
			key = r.MessageID
		}
		if key == "" {
			continue
		}
		e := schema.Event{
			Vendor: "anthropic", Agent: "cowork-cloud", Surface: "cloud",
			SessionID: r.SessionID, RequestID: key, Model: r.Model,
			Input: r.InputTokens, Output: r.OutputTokens,
		}
		// A subagent's request becomes its own session under its parent,
		// named as a local subagent trail is (agent-<id>), so the Station
		// draws it as a subagent. A row without an agent (older plugin) or
		// with "main" stays on its session.
		if r.Agent != "" && r.Agent != "main" && r.Agent != r.SessionID {
			e.SessionID, e.ParentID = r.Agent, r.SessionID
		}
		if r.CacheCreation > 0 {
			v := r.CacheCreation
			e.CacheWrite = &v
		}
		if r.CacheRead > 0 {
			v := r.CacheRead
			e.CacheRead = &v
		}
		if t, err := time.Parse(time.RFC3339Nano, r.TS); err == nil {
			e.At = t.UTC()
		}
		events = append(events, e)
	}
	if err := sc.Err(); err != nil {
		return nil, nil, 0, err
	}
	return events, nil, fi.Size(), nil
}
