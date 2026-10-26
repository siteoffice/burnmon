package controls

import (
	"reflect"
	"strings"
	"testing"
)

func TestValidAgentID(t *testing.T) {
	good := []string{"ab12", "7c5dcf5d", "a_b-c", strings.Repeat("a", 64)}
	for _, id := range good {
		if !ValidAgentID(id) {
			t.Errorf("ValidAgentID(%q) = false, want true", id)
		}
	}
	bad := []string{"", "-x", "--all", "a b", `a"b`, "a&b", "%PATH%", `a\b`,
		strings.Repeat("a", 65), "ab12\n"}
	for _, id := range bad {
		if ValidAgentID(id) {
			t.Errorf("ValidAgentID(%q) = true, want false", id)
		}
	}
}

func TestStopAndAttachArgv(t *testing.T) {
	exe := `C:\x\claude.exe`
	got, err := StopArgv(exe, "ab12")
	if err != nil || !reflect.DeepEqual(got, []string{exe, "stop", "ab12"}) {
		t.Fatalf("StopArgv = %v, %v", got, err)
	}
	got, err = AttachArgv(exe, "ab12")
	if err != nil || !reflect.DeepEqual(got, []string{exe, "attach", "ab12"}) {
		t.Fatalf("AttachArgv = %v, %v", got, err)
	}
	if a, err := StopArgv(exe, "-x"); err == nil || a != nil {
		t.Errorf("StopArgv with a bad id = %v, %v", a, err)
	}
	if a, err := AttachArgv(exe, "a b"); err == nil || a != nil {
		t.Errorf("AttachArgv with a bad id = %v, %v", a, err)
	}
	if a, err := StopArgv("", "ab12"); err == nil || a != nil {
		t.Errorf("StopArgv with no binary = %v, %v", a, err)
	}
}

func TestControlIDs(t *testing.T) {
	want := []string{"stop", "attach", "approve", "send_back"}
	if !reflect.DeepEqual(All, want) {
		t.Fatalf("All = %v", All)
	}
}
