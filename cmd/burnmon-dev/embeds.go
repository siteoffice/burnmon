package main

import _ "embed"

//go:embed page.html
var pageHTML string

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
