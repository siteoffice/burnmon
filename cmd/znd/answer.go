package main

import (
	"fmt"
	"io"
	"time"

	"burnmon/internal/controls"
)

// runAnswer answers a spec gate through internal\controls. It offers Approve
// and SendBack only. --yes calls Preview then Run in this one process, so the
// preview token never leaves it. Run keeps its fresh gate check and writes its
// start line before it runs anything.
func runAnswer(args []string, stdout, stderr io.Writer, d deps) int {
	pos, dry, yes, bad := splitFlags(args)
	if bad != "" {
		fmt.Fprintf(stderr, "unknown flag %s\n%s\n", bad, usage)
		return exitUsage
	}
	if len(pos) != 2 || (pos[1] != "approve" && pos[1] != "send-back") {
		fmt.Fprintln(stderr, usage)
		return exitUsage
	}
	if dry == yes {
		fmt.Fprintln(stderr, "give exactly one of --dry-run and --yes")
		return exitUsage
	}
	z, err := d.zndConfig()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	control := controlsApprove
	if pos[1] == "send-back" {
		control = controlsSendBack
	}
	svc := newService(d, z)
	pv, err := svc.Preview(previewRequest(control, pos[0]))
	if err != nil {
		fmt.Fprintln(stderr, "refused: "+err.Error())
		return exitRefused
	}
	fmt.Fprintln(stdout, pv.Label)
	fmt.Fprintln(stdout, pv.Display)
	if dry {
		return exitOK
	}
	// Run refuses a press inside MinConfirmGap of the preview, so a double
	// click never runs. One process waits it out; a person is not in the loop.
	d.wait(controls.MinConfirmGap + 50*time.Millisecond)
	res, err := svc.Run(pv.Token)
	if err != nil {
		fmt.Fprintln(stderr, "refused: "+err.Error())
		return exitRefused
	}
	switch {
	case res.ExitCode == gateExitChanged:
		fmt.Fprintln(stderr, res.Refused)
		return exitChanged
	case res.ExitCode == gateExitVerdict:
		fmt.Fprintln(stderr, res.Refused)
		return exitVerdict
	case res.Failed:
		fmt.Fprintf(stderr, "failed with exit %d: %s\n", res.ExitCode, res.Line)
		return exitFailed
	}
	fmt.Fprintln(stdout, "decided")
	return exitOK
}
