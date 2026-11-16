package gitrefs

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// Remote is one [remote "name"] section of a git config file.
type Remote struct {
	Name string
	URL  string // the first url line of the section, "" when it has none
}

var remoteHead = regexp.MustCompile(`^\[remote\s+"([^"]+)"\]$`)

// Remotes reads the [remote "name"] sections of a git config file, in file
// order. An unreadable file gives an empty, non-nil list.
func Remotes(configPath string) []Remote {
	out := []Remote{}
	f, err := os.Open(configPath)
	if err != nil {
		return out
	}
	defer f.Close()
	cur := -1
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			cur = -1
			if m := remoteHead.FindStringSubmatch(line); m != nil {
				out = append(out, Remote{Name: m[1]})
				cur = len(out) - 1
			}
			continue
		}
		if cur < 0 {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.EqualFold(strings.TrimSpace(k), "url") && out[cur].URL == "" {
			out[cur].URL = strings.TrimSpace(v)
		}
	}
	return out
}
