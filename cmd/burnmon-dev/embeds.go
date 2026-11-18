package main

import _ "embed"

//go:embed page.html
var pageHTML string

// The lamp page, shown by the lamp subcommand (plan 2026-11-17_lamp_plan.md).
//
//go:embed lamp.html
var lampHTML string

// The week card window of `lamp --card <folder> --card-window`: the story card
// and one Save PNG button.
//
//go:embed lamp_card.html
var lampCardHTML string

// The Station (02_roadmap\2026-10-01_station_secret_screen.md): the two
// atlases (space and construction site), the startup theme and the screen,
// spliced into page.html at stationMarker by assemblePage before SetHtml.
//
//go:embed station/station_atlas.js
var stationAtlasJS string

//go:embed station/station_atlas_site.js
var stationAtlasSiteJS string

//go:embed station/station.js
var stationJS string
