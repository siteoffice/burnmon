package main

// devBindingNames is every function bindDev (main.go) registers on the window,
// in the order it registers them. TestDevBindingNamesCoverPage checks the page
// calls nothing outside it and TestBindDevRegistersSharedList checks bindDev
// registers exactly this list.
var devBindingNames = []string{
	"bdevToggleFullscreen",
	"bdevExitFullscreen",
	"bdevMarkFirstRender",
	"bdevSetHidden",
	"bdevSetUITheme",
	"bdevSetStationTheme",
	"bdevSetToast",
	"bdevSections",
	"bdevSnapshotStamp",
	"bdevSetSections",
	"bdevControlPreview",
	"bdevControlRun",
	"bdevOpenItemFile",
	"bdevAgentMapStart",
	"bdevSnapshotNow",
	"bdevPerfReport",
	"bdevTurnDetail",
	"bdevCacheBreakdown",
	"bdevExport",
	"bdevDeleteHistory",
	"bdevExportHistory",
	"bdevBurnReport",
	"bdevHistoryInfo",
	"bdevTodoLogin",
}
