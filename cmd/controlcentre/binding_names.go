package main

// ccBindingNames is every function bindAll (bindings.go) registers on the
// window, in the order it registers them. TestCCBindingNamesMatchPage checks
// the page calls exactly these and TestBindAllRegistersSharedList checks
// bindAll registers exactly this list.
var ccBindingNames = []string{
	"ccState",
	"ccFile",
	"ccBoard",
	"ccSettingsGet",
	"ccSettingsSet",
	"ccComment",
	"ccPreview",
	"ccRun",
	"ccAgents",
}
