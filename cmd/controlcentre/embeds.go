//go:build windows || darwin

package main

import (
	_ "embed"
	"strings"

	"burnmon/internal/centre"
)

//go:embed page.html
var pageHTML string

// assemblePage returns page.html with the theme set in the markup when it is
// light or dark, so it is there before the first paint (the technique of
// assemblePage in cmd\burnmon-dev\main.go). System leaves the page as is.
func assemblePage(s centre.Settings) string {
	switch t := s.Normalized().Theme; t {
	case "light", "dark":
		return strings.Replace(pageHTML, `<html lang="en">`, `<html lang="en" data-theme="`+t+`">`, 1)
	}
	return pageHTML
}
