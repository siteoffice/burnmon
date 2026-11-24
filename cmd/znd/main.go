// znd is one program for Windows and Mac that answers a Foreman spec gate,
// later runs git recipes, updates mods and writes audit lines. It makes no
// network call and starts no shell. Every acting command prints what it would
// do with --dry-run and acts only with --yes.
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"burnmon/internal/controls"
	"burnmon/internal/controlsenv"
	"burnmon/internal/snapshot"
	"burnmon/internal/store"
)

// Exit codes. 3 and 4 are the gate exit codes of Foreman's own script.
const (
	exitOK      = 0
	exitFailed  = 1
	exitUsage   = 2
	exitChanged = 3
	exitVerdict = 4
	exitRefused = 5
)

// deps is what the commands need from the machine, so tests inject fakes.
type deps struct {
	zndConfig func() (controlsenv.ZndConfig, error)
	env       func(z controlsenv.ZndConfig) controls.Env
	wait      func(time.Duration)
	git       func() gitEnv
}

func realDeps() deps {
	dataDir := store.DataDirFrom(os.Getenv("LOCALAPPDATA"), os.UserConfigDir)
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	return deps{
		wait: time.Sleep,
		git: func() gitEnv {
			storePath, _ := store.DefaultPath()
			git, _ := exec.LookPath("git")
			if abs, err := filepath.Abs(git); err == nil && git != "" {
				git = abs
			}
			python, _ := controls.ResolvePython(exec.LookPath, runtime.GOOS)
			return gitEnv{Git: git, Python: python, Engine: controlsenv.LoadBurnmonConfig(exeDir, dataDir).GroundworkEngine,
				AuditPath: controls.AuditPath(storePath), Runner: controls.ExecRunner{}, Now: time.Now}
		},
		zndConfig: func() (controlsenv.ZndConfig, error) { return controlsenv.LoadZndConfig(exeDir, dataDir) },
		env: func(z controlsenv.ZndConfig) controls.Env {
			snapPath, _ := snapshot.DefaultPath()
			storePath, _ := store.DefaultPath()
			e := controlsenv.NewDecisionEnv(controlsenv.LoadBurnmonConfig(exeDir, dataDir), dataDir, snapPath, controls.AuditPath(storePath), exec.LookPath)
			e.GateWithoutSnapshot = z.GateWithoutSnapshot
			return e
		},
	}
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, realDeps())) }

const usage = "usage: znd answer <gate-wait-id> approve|send-back (--dry-run | --yes)\n" +
	"       znd git save|share|land|tag|sync --cwd <repo> [recipe flags] (--dry-run | --yes)"

func run(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return exitUsage
	}
	switch args[0] {
	case "answer":
		return runAnswer(args[1:], stdout, stderr, d)
	case "git":
		return runGit(args[1:], stdout, stderr, d)
	}
	fmt.Fprintf(stderr, "unknown command %q\n%s\n", args[0], usage)
	return exitUsage
}

// splitFlags separates the two mode flags from the positional words.
func splitFlags(args []string) (pos []string, dry, yes bool, bad string) {
	for _, a := range args {
		switch a {
		case "--dry-run":
			dry = true
		case "--yes":
			yes = true
		default:
			if len(a) > 1 && a[0] == '-' {
				bad = a
			}
			pos = append(pos, a)
		}
	}
	return
}
