package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"burnmon/internal/controls"
	"burnmon/internal/recipes"
)

const gitUsage = "usage: znd git save|share|land|tag|sync --cwd <repo> [recipe flags] (--dry-run | --yes)"

// gitStepTimeout bounds one recipe line. Land waits on a pull request merge.
const gitStepTimeout = 10 * time.Minute

// gitEnv is what the git command needs from the machine.
type gitEnv struct {
	Git, Python, Engine string
	AuditPath           string
	Runner              controls.Runner
	Now                 func() time.Time
}

// recipeFlag returns where a flag's value goes, or nil for an unknown flag.
func recipeFlag(r *recipes.Request, name string) *string {
	switch name {
	case "--cwd":
		return &r.Cwd
	case "--message":
		return &r.Message
	case "--body":
		return &r.Body
	case "--co-author":
		return &r.CoAuthor
	case "--tag":
		return &r.Tag
	case "--subject":
		return &r.Subject
	case "--expect":
		return &r.Expect
	case "--runs-in":
		return &r.RunsIn
	case "--needs":
		return &r.Needs
	}
	return nil
}

// runGit runs one snip.py recipe. --dry-run prints the block and nothing else.
// --yes prints the block, writes the start audit line, runs the argv lines in
// order without a shell and stops at the first one that fails. If the start
// line cannot be written, nothing runs.
func runGit(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, gitUsage)
		return exitUsage
	}
	req := recipes.Request{Recipe: args[0]}
	var dry, yes bool
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		switch a := rest[i]; a {
		case "--dry-run":
			dry = true
		case "--yes":
			yes = true
		case "--files":
			for i+1 < len(rest) && !strings.HasPrefix(rest[i+1], "--") {
				i++
				req.Files = append(req.Files, rest[i])
			}
		default:
			dst := recipeFlag(&req, a)
			if dst == nil || i+1 >= len(rest) {
				fmt.Fprintf(stderr, "unknown flag or missing value: %s\n%s\n", a, gitUsage)
				return exitUsage
			}
			i++
			*dst = rest[i]
		}
	}
	if dry == yes {
		fmt.Fprintln(stderr, "give exactly one of --dry-run and --yes")
		return exitUsage
	}
	env := d.git()
	req.Engine = env.Engine
	plan, err := recipes.Build(req, recipes.Tools{Git: env.Git, Python: env.Python, DirExists: dirExists})
	if err != nil {
		fmt.Fprintln(stderr, "refused: "+err.Error())
		return exitUsage
	}
	fmt.Fprint(stdout, plan.Block)
	if dry {
		// The block copies snip.py; --yes runs the resolved paths below, so show them.
		for i, a := range plan.Argv {
			fmt.Fprintf(stdout, "# runs %d of %d: %s\n", i+1, len(plan.Argv), strings.Join(a, " "))
		}
		return exitOK
	}

	control := "git_" + req.Recipe
	af, err := controls.OpenAudit(env.AuditPath)
	if err != nil {
		fmt.Fprintln(stderr, "refused: the audit line cannot be written, nothing runs: "+err.Error())
		return exitRefused
	}
	defer af.Close()
	shown := make([]string, len(plan.Argv))
	for i, a := range plan.Argv {
		shown[i] = strings.Join(a, " ")
	}
	line := controls.AuditLine{Time: env.Now().UTC().Format(time.RFC3339), Control: control, Target: plan.Dir,
		Command: strings.Join(shown, " ; "), Phase: "start"}
	if err := controls.WriteAudit(af, line); err != nil {
		fmt.Fprintln(stderr, "refused: the audit line cannot be written, nothing runs: "+err.Error())
		return exitRefused
	}

	result := controls.AuditLine{Time: line.Time, Control: control, Target: plan.Dir, Command: line.Command}
	code := exitOK
	for i, argv := range plan.Argv {
		fmt.Fprintf(stdout, "step %d of %d: %s\n", i+1, len(plan.Argv), shown[i])
		ctx, cancel := context.WithTimeout(context.Background(), gitStepTimeout)
		exit, first, rerr := env.Runner.Run(ctx, argv, plan.Dir)
		cancel()
		if rerr != nil || exit != 0 {
			result.ExitCode = exit
			result.Refused = fmt.Sprintf("step %d failed with exit %d: %s", i+1, exit, first)
			if rerr != nil {
				result.Refused = fmt.Sprintf("step %d did not run: %v", i+1, rerr)
			}
			fmt.Fprintln(stderr, result.Refused)
			code = exitFailed
			break
		}
	}
	result.Time = env.Now().UTC().Format(time.RFC3339)
	if err := controls.WriteAudit(af, result); err != nil {
		fmt.Fprintln(stderr, "the result line could not be written: "+err.Error())
		return exitFailed
	}
	return code
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
