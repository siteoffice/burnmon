package main

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"burnmon/internal/historyadmin"
	"burnmon/internal/store"
)

// History delete and export for the settings dialog (B1). Both front ends
// (the WebView2 bindings in main.go, the browser bridge in browser.go) call
// these two methods; the rules themselves live in internal/historyadmin.

// rebuildAfterDelete refreshes the Month column and the History tab once a
// delete removed rows. A variable so tests do not scan the real machine.
var rebuildAfterDelete = func(a *app) {
	if _, err := a.rebuild(false, nil); err != nil {
		log.Println("rebuild after history delete failed:", err)
	}
}

// deleteHistory removes history by scope. The Go side refuses without
// confirm: true (historyadmin.ErrNotConfirmed), whatever the page sent.
func (a *app) deleteHistory(st *store.Store, req historyadmin.DeleteRequest) (historyadmin.DeleteResult, error) {
	res, err := historyadmin.Delete(st, req, time.Now())
	if err != nil {
		return res, err
	}
	if res.Events > 0 || res.ToolCalls > 0 {
		go rebuildAfterDelete(a)
	}
	return res, nil
}

// exportHistory writes the same file burnmon-cli export writes into
// req.Dir. Only an absolute folder is accepted from the app: a relative one
// would resolve against whatever the process working directory is.
func (a *app) exportHistory(st *store.Store, req historyadmin.ExportRequest, now time.Time) (historyadmin.ExportResult, error) {
	if !filepath.IsAbs(req.Dir) {
		return historyadmin.ExportResult{}, fmt.Errorf("export folder must be an absolute path (for example %s), got %q",
			historyadmin.DefaultExportDir(), req.Dir)
	}
	cfg := a.cfgSnapshot()
	return historyadmin.Export(st, &cfg, req, now)
}

// exportOutcome is what bmExportHistory resolves the page with in the
// WebView2 window: a result or an error text, since the binding itself
// returns before the export runs.
type exportOutcome struct {
	Path  string `json:"path"`
	Rows  int    `json:"rows"`
	Error string `json:"error,omitempty"`
}

func (a *app) exportOutcome(st *store.Store, req historyadmin.ExportRequest) exportOutcome {
	res, err := a.exportHistory(st, req, time.Now())
	if err != nil {
		return exportOutcome{Error: err.Error()}
	}
	return exportOutcome{Path: res.Path, Rows: res.Rows}
}

// historyOwners is cfg.Owners' owner names, first appearance order, each
// once: the export form's checkboxes.
func historyOwners(a *app) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range a.cfg.Owners {
		if r.Owner != "" && !seen[r.Owner] {
			seen[r.Owner] = true
			out = append(out, r.Owner)
		}
	}
	return out
}

// historyBlockHTML is the "History" block of the settings dialog: delete
// (older than N days, one session, everything) and export to a folder. Each
// delete button is two-step: the first click arms it for 5 seconds and
// relabels it "Confirm: ...", the second click sends confirm: true. The Go
// side refuses a delete without it either way. Called with a.mu held, like
// the rest of settingsModalHTML.
func (a *app) historyBlockHTML() string {
	keep := "off"
	if a.cfg.KeepHistory {
		keep = "on"
	}
	var owners strings.Builder
	for _, o := range historyOwners(a) {
		owners.WriteString(`<label style="flex-direction:row;align-items:center;gap:6px"><input type="checkbox" class="cc_h_owner" value="` +
			escapeAttr(o) + `" style="padding:0"> ` + escapeAttr(o) + `</label>`)
	}
	if owners.Len() == 0 {
		owners.WriteString(`<span style="font-size:12px;color:#9fb4bd">No owner rules in burnmon.json, so no owner is needed.</span>`)
	}
	return `
  <div id="cc_h_block" style="margin-top:18px;padding-top:14px;border-top:1px solid var(--pine-40)">
   <h4 style="margin:0 0 6px;font-size:14px">History</h4>
   <p style="margin:0 0 10px;color:#9fb4bd;font-size:12px">Keep history: ` + keep + ` (keep_history in burnmon.json)</p>
   <style>
    #cc_h_block button{padding:6px 12px;border:1px solid var(--pine-40);border-radius:6px;background:transparent;color:#fff;font:13px 'Inter','Segoe UI',sans-serif;cursor:pointer}
    #cc_h_block .row{display:flex;gap:10px;align-items:flex-end;margin-bottom:10px;flex-wrap:wrap}
    #cc_h_block .del{border-color:#ff8a65;color:#ff8a65}
   </style>
   <div class="row">
    <label>Delete older than (days)<input id="cc_h_days" type="number" step="1" min="1" value="90" style="width:90px"></label>
    <button id="cc_h_delete_older" class="del" type="button">Delete older than N days</button>
   </div>
   <div class="row">
    <label>Delete one session<input id="cc_h_session" type="text" placeholder="session id" style="width:220px"></label>
    <button id="cc_h_delete_session" class="del" type="button">Delete one session</button>
   </div>
   <div class="row">
    <button id="cc_h_delete_all" class="del" type="button">Delete all history</button>
   </div>
   <div id="cc_h_delete_result" style="font-size:12px;color:#9fb4bd;margin-bottom:12px;min-height:14px"></div>
   <h4 style="margin:0 0 6px;font-size:14px">Export history</h4>
   <div class="grid">
    <label style="grid-column:1/3">Folder (absolute path)<input id="cc_h_dir" type="text" value="` + escapeAttr(historyadmin.DefaultExportDir()) + `"></label>
    <label>Since (YYYY-MM-DD)<input id="cc_h_since" type="text" placeholder="first day"></label>
    <label>Until (YYYY-MM-DD)<input id="cc_h_until" type="text" placeholder="last day"></label>
    <label>Label<input id="cc_h_label" type="text" value="dev-1"></label>
    <div style="display:flex;flex-direction:column;gap:4px;font-size:12px;color:#c8d4d9">Owners` + owners.String() + `</div>
   </div>
   <div class="row" style="margin-top:10px">
    <button id="cc_h_export" type="button">Export</button>
   </div>
   <div id="cc_h_export_result" style="font-size:12px;color:#9fb4bd;min-height:14px;word-break:break-all"></div>
  </div>
  <script>
  (function(){
    var $ = function(id){ return document.getElementById(id); };
    if(!$('cc_h_block')) return;
    // Two-step confirmation: the first press validates the field, locks the
    // value into the request and arms the button for 5 seconds with a label
    // naming what will go; the second press sends that locked request. If the
    // field changed in between, it re-arms with the new value instead of
    // sending. Invalid input disarms.
    function twoStep(btn, build){
      var timer = null, armed = null, text = btn.textContent;
      function disarm(){ if(timer){ clearTimeout(timer); timer = null; } armed = null; btn.textContent = text; }
      btn.onclick = function(){
        var cur = build();
        if(!cur){ disarm(); return; }
        if(armed && armed.key === cur.key){ var req = armed.req; disarm(); sendDelete(req); return; }
        disarm();
        armed = cur;
        btn.textContent = 'Confirm: ' + cur.label;
        timer = setTimeout(disarm, 5000);
      };
    }
    function sendDelete(req){
      var out = $('cc_h_delete_result');
      out.textContent = 'Deleting...';
      window.bmDeleteHistory(req).then(function(r){
        out.textContent = 'Deleted ' + r.events + ' events and ' + r.tool_calls + ' tool calls. The dashboard refreshes at the next rebuild.';
      }).catch(function(e){ out.textContent = 'Delete failed: ' + String((e && e.message) || e); });
    }
    twoStep($('cc_h_delete_older'), function(){
      var raw = $('cc_h_days').value.trim();
      var days = Number(raw);
      if(raw === '' || !Number.isInteger(days) || days < 1){
        $('cc_h_delete_result').textContent = 'Enter a whole number of days, 1 or more.';
        return null;
      }
      return {key: 'older:' + days, label: 'delete older than ' + days + ' days',
        req: {scope: 'older_than', days: days, confirm: true}};
    });
    twoStep($('cc_h_delete_session'), function(){
      var id = $('cc_h_session').value.trim();
      if(!id){ $('cc_h_delete_result').textContent = 'Enter a session id.'; return null; }
      return {key: 'session:' + id, label: 'delete session ' + id,
        req: {scope: 'session', session_id: id, confirm: true}};
    });
    twoStep($('cc_h_delete_all'), function(){
      return {key: 'all', label: 'delete all history', req: {scope: 'all', confirm: true}};
    });
    var expBtn = $('cc_h_export'), expOut = $('cc_h_export_result');
    window.__bmExportHistoryResolve = function(key, d){
      expBtn.disabled = false;
      if(!d || d.error){ expOut.textContent = 'Export failed: ' + ((d && d.error) || 'no answer'); return; }
      expOut.textContent = 'Wrote ' + d.rows + ' rows to ' + d.path;
    };
    expBtn.onclick = function(){
      var owners = [];
      var boxes = document.querySelectorAll('#cc_h_block .cc_h_owner');
      for(var i = 0; i < boxes.length; i++){ if(boxes[i].checked) owners.push(boxes[i].value); }
      expBtn.disabled = true;
      expOut.textContent = 'Exporting...';
      window.bmExportHistory({dir: $('cc_h_dir').value.trim(), since: $('cc_h_since').value.trim(),
        until: $('cc_h_until').value.trim(), label: $('cc_h_label').value.trim(), owners: owners});
    };
  })();
  </script>`
}
