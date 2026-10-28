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
	for _, want := range []string{"report_path", "parked_lines", "parked line", "no report yet"} {
		if !strings.Contains(row, want) {
			t.Errorf("secItemRow lacks %q", want)
		}
	}
	// The chip call for close_out sits behind the report and parked-lines test.
	i := strings.Index(row, "it.kind === 'close_out'")
	if i < 0 {
		t.Fatal("secItemRow has no close_out branch")
	}
	branch := row[i:]
	if j := strings.Index(branch, "else {"); j > 0 {
		branch = branch[:j]
	}
	if !strings.Contains(branch, "closeable") || !strings.Contains(branch, "ctlChips('closeout'") {
		t.Errorf("the close_out chip is not gated on closeable: %q", branch)
	}
	if !strings.Contains(row, "var closeable = ") || !strings.Contains(row, "!!it.report_path") || !strings.Contains(row, "!(it.parked_lines > 0)") {
		t.Error("closeable is not 'a report exists and no parked lines'")
	}
}
