package main

import (
	"strings"
	"testing"
)

// ---- B6 fix pass E: deleted branch and the Close chip -----------------------

func TestPageBranchGoneRow(t *testing.T) {
	if !strings.Contains(pageHTML, "branch_gone:['branch gone, check by hand'") {
		t.Error("SEC_KIND has no branch_gone word")
	}
	if got := pageFuncBody(t, "secTier"); !strings.Contains(got, "branch_gone") {
		t.Error("secTier does not put branch_gone in the done tier")
	}
	row := pageFuncBody(t, "secItemRow")
	if !strings.Contains(row, "secgrey") || !strings.Contains(row, "'branch_gone'") {
		t.Error("secItemRow does not draw a branch_gone row grey")
	}
	if !strings.Contains(pageHTML, ".secgrey") {
		t.Error("page has no .secgrey rule")
	}
	// The only chips are on gate, close_out and agent rows: never branch_gone.
	if strings.Contains(row, "kind === 'branch_gone'){ tg") {
		t.Error("a branch_gone row gets a control")
	}
}

func TestPageCloseChipOnlyWhenForemanWouldAccept(t *testing.T) {
	row := pageFuncBody(t, "secItemRow")
	for _, want := range []string{"report_path", "parked_lines", "parked line", "no report yet", "close_reason"} {
		if !strings.Contains(row, want) {
			t.Errorf("secItemRow lacks %q", want)
		}
	}
	// The close_out row keeps its reason note; the Close chip itself moved to the
	// Control Centre (2026-11-15), so no closeout chip call is left here.
	if !strings.Contains(row, "it.kind === 'close_out'") {
		t.Fatal("secItemRow has no close_out branch")
	}
	if strings.Contains(row, "ctlChips('closeout'") {
		t.Error("secItemRow still draws a closeout chip")
	}
	if !strings.Contains(row, "var closeable = ") || !strings.Contains(row, "!!it.report_path") || !strings.Contains(row, "it.close_ok === true") || !strings.Contains(row, "!(it.parked_lines > 0)") {
		t.Error("closeable is not 'a report exists, no parked lines and Foreman's gate would accept (close_ok)'")
	}
}

// The Close control's switch moved to the Control Centre (2026-11-15, its
// label "Close sprint" is checked there); BurnMon Dev has no Close switch.
func TestPageGearLabelIsCloseSprint(t *testing.T) {
	if strings.Contains(pageHTML, `data-ctl-sw="close_sprint"`) || strings.Contains(pageHTML, "Close a merged sprint") {
		t.Error("BurnMon Dev still has a Close switch")
	}
}