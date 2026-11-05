package main

// appBindingNames lists every function the app window binds for the page, in
// bind order. bindApp (main.go) registers exactly these; a test checks that
// the page calls nothing outside this list.
var appBindingNames = []string{
	"ccRefresh", "bmLive", "bmTurn", "bmSessionInsight", "bmHistory",
	"bmVendorStrip", "bmForecast", "ccSaveSettings", "bmDeleteHistory", "bmExportHistory",
}
