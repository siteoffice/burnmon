package centre

// History rules of the Control Centre's Back button (spec item 5). Pure
// functions: no clock, no I/O. cmd\controlcentre\page.html carries a
// line-for-line port (viewAdds, histAdd, histBack) that the page tests pin
// against these.

// View is what the main pane shows: page, Overview switch, unit, unit tab,
// list page number, file, board and the selected thing.
type View struct {
	Page     string `json:"page"`
	Overview string `json:"overview"`
	Unit     string `json:"unit"`
	Tab      string `json:"tab"`
	ListPage int    `json:"list_page"`
	File     string `json:"file"`
	Board    string `json:"board"`
	Sel      string `json:"sel"`
}

// HistoryMax is the most views the history holds.
const HistoryMax = 50

// ViewAdds reports whether next is a new view after last: the page, unit,
// tab, file or board differ. A change of selection, list page number or
// Overview switch alone adds nothing.
func ViewAdds(last, next View) bool {
	return last.Page != next.Page || last.Unit != next.Unit || last.Tab != next.Tab ||
		last.File != next.File || last.Board != next.Board
}

// HistoryAdd records v: appended when it is a new view, else it replaces
// the last entry, so the last entry always holds the current selection,
// page number and Overview switch. The oldest entries go beyond
// HistoryMax. h is not modified.
func HistoryAdd(h []View, v View) []View {
	out := append([]View(nil), h...)
	if len(out) == 0 || ViewAdds(out[len(out)-1], v) {
		out = append(out, v)
	} else {
		out[len(out)-1] = v
	}
	if len(out) > HistoryMax {
		out = out[len(out)-HistoryMax:]
	}
	return out
}

// BackResult is what one Back does: the new history, the view to show
// when Moved, or only closing the comment popup.
type BackResult struct {
	History    []View
	Show       View
	Moved      bool
	ClosePopup bool
}

// HistoryBack is one Back. With the popup open it only closes the popup;
// with nothing before the current view it does nothing; else it drops the
// current view and shows the one before it.
func HistoryBack(h []View, popupOpen bool) BackResult {
	if popupOpen {
		return BackResult{History: h, ClosePopup: true}
	}
	if len(h) < 2 {
		return BackResult{History: h}
	}
	return BackResult{History: h[:len(h)-1], Show: h[len(h)-2], Moved: true}
}

// CanGoBack reports whether a view lies before the current one (the last
// entry); "empty history" in the spec means this is false.
func CanGoBack(h []View) bool { return len(h) > 1 }
