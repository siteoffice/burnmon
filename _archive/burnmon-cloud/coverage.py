#!/usr/bin/env python3
"""BurnMon cloud, watcher coverage check.

Runs anywhere, reads one diag.log written by the plugin. For every write of
latest.jsonl (an extract that wrote, or a settle rewrite) it asks: did a
FileChanged "fired" line follow within WINDOW seconds? It prints the share
covered, and the longest stretch of writes with no event. Use it on the diag.log
of a long cloud session (several turns, an idle gap, a resume) to decide
whether the Stop, UserPromptSubmit and SessionEnd copy hooks can go.

  python3 coverage.py <path to diag.log>
  python3 coverage.py                     (default: the cloud outputs folder)

Exit code 0 when every write was covered, 1 when not, 2 when there is no data.
"""
import json
import sys
from datetime import datetime

DEFAULT = "/mnt/user-data/outputs/.burnmon/cloud/diag.log"
WINDOW = 3.0
WRITE_RESULTS = {("PostToolUse", "wrote"), ("Stop", "wrote"), ("SubagentStop", "wrote"),
                 ("settle", "rewrote")}


def ts(e):
    return datetime.strptime(e["at"], "%Y-%m-%dT%H:%M:%S.%fZ").timestamp()


def main(path):
    try:
        with open(path, encoding="utf-8") as f:
            ev = [json.loads(l) for l in f if l.strip()]
    except OSError as e:
        print("cannot read", path, e)
        return 2
    writes = [e for e in ev if (e["hook"], e["result"]) in WRITE_RESULTS]
    fired = [ts(e) for e in ev if e["hook"] == "FileChanged"]
    if not writes:
        print("no writes in", path)
        return 2
    covered = [any(0 <= f - ts(w) <= WINDOW for f in fired) for w in writes]
    miss = covered.count(False)
    print("writes %d, covered by FileChanged within %.0f s: %d, missed: %d (%.0f%% covered)"
          % (len(writes), WINDOW, len(writes) - miss, miss, 100.0 * (len(writes) - miss) / len(writes)))
    print("FileChanged fired: %d, SessionStart registrations: %d"
          % (len(fired), sum(1 for e in ev if e["hook"] == "SessionStart")))
    run = best = 0
    first = None
    best_first = None
    for w, c in zip(writes, covered):
        if c:
            run = 0
            first = None
        else:
            run += 1
            first = first or w["at"]
            if run > best:
                best, best_first = run, first
    if best:
        print("longest run of uncovered writes: %d, starting %s" % (best, best_first))
    return 0 if miss == 0 else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else DEFAULT))
