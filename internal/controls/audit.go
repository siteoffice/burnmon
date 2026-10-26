package controls

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

// AuditLine is one line of controls-audit.ndjson. It holds no output, no
// session text and no preview token.
type AuditLine struct {
	Time     string `json:"time"`
	Control  string `json:"control"`
	Target   string `json:"target"`
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Refused  string `json:"refused,omitempty"`
}

// AuditPath is the audit file next to the store file.
func AuditPath(storePath string) string {
	return filepath.Join(filepath.Dir(storePath), "controls-audit.ndjson")
}

// OpenAudit opens the audit file for append, creating it.
func OpenAudit(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
}

// WriteAudit writes one JSON line with a single Write call.
func WriteAudit(w io.Writer, l AuditLine) error {
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}
