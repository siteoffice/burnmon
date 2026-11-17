#!/usr/bin/env python3
"""BurnMon cloud, extract hook (1.1.4).

Runs inside a cloud Cowork session. Reads the hook input on stdin, then the
session transcript and its subagent files, and writes a usage-only extract:
one JSON line per model request, the last row per (requestId, message.id),
whitelisted fields only. No message text, tool input, paths, cwd or branch is
ever written.

Modes:
  (no flag)    extract now (Stop, SubagentStop)
  --throttle   extract, unless latest.jsonl was written less than 20 s ago
               (PostToolUse, which fires after every tool call)
  --watch      SessionStart: print the watchPaths output that makes the
               plugin's FileChanged hook fire on every write of latest.jsonl,
               then exit; no extract. Prints nothing outside a cloud session.
               1.1.4 watches the drop FOLDER, not the file: the outputs folder
               is an rclone mount whose file stat flickers after a write, and
               a watcher bound to the file itself went deaf after 2 to 4
               events (measured 2026-10-13). The hook matcher still limits
               the copy to latest.jsonl.
  --fired      FileChanged (1.1.1 diagnostic): log when the hook fired and
               what latest.jsonl held at that moment; no extract.
  --settle H   started by an extract, detached: after 10 s, rewrite
               latest.jsonl with the same bytes if its SHA-256 is still H.
               A copy within about 5 s of a write delivers the previous
               upload of the outputs folder; this rewrite's copy finds the
               new content uploaded (measured 2026-10-12).

Outputs, under /mnt/user-data/outputs/.burnmon/cloud/:
  <session_id>.jsonl   the extract for this session
  latest.jsonl         the same rows under a fixed name; the plugin's copy
                       hooks send this file to BurnMon's drop folder on the
                       user's computer
  diag.log             1.1.1 diagnostic: one line per hook run (time, hook,
                       result, row count, short session id); no content
  errors.log           only when something failed

Never fails the turn: every error is logged and the script exits 0.
"""
import glob
import hashlib
import json
import os
import subprocess
import sys
import time

__version__ = "1.1.4"

OUT = "/mnt/user-data/outputs/.burnmon/cloud"
CLOUD_ENTRYPOINT = "remote_cowork"
THROTTLE_SECONDS = 20
DIAG_MAX_BYTES = 256 * 1024
SETTLE_SECONDS = 10
USAGE_KEYS = (
    "input_tokens",
    "output_tokens",
    "cache_creation_input_tokens",
    "cache_read_input_tokens",
)


def usage_row(d, agent):
    m = d.get("message") or {}
    if not isinstance(m, dict):
        return None
    u = m.get("usage")
    if not isinstance(u, dict):
        return None
    cc = u.get("cache_creation") or {}
    otd = u.get("output_tokens_details") or {}
    row = {
        "v": 1,
        "source": "burnmon-cloud",
        "ts": d.get("timestamp"),
        "session_id": d.get("sessionId"),
        "request_id": d.get("requestId"),
        "message_id": m.get("id"),
        "model": m.get("model"),
        "agent": agent,
        "is_sidechain": bool(d.get("isSidechain")),
        "entrypoint": d.get("entrypoint"),
        "service_tier": u.get("service_tier"),
        "speed": u.get("speed"),
        "cache_5m": cc.get("ephemeral_5m_input_tokens"),
        "cache_1h": cc.get("ephemeral_1h_input_tokens"),
        "thinking_tokens": otd.get("thinking_tokens"),
    }
    for k in USAGE_KEYS:
        v = u.get(k)
        row[k] = v if isinstance(v, (int, float)) else 0
    return row


def read_rows(path, agent):
    rows = {}
    with open(path, encoding="utf-8", errors="replace") as f:
        for line in f:
            try:
                d = json.loads(line)
            except ValueError:
                continue
            r = usage_row(d, agent)
            if r is None:
                continue
            # Last row wins: early rows of a streamed request carry partial
            # output counts (measured 2026-10-12: output 8, then 148).
            rows[(r["request_id"], r["message_id"])] = r
    return rows


def main_transcript(tpath):
    # The docs give the main transcript as transcript_path, also on
    # SubagentStop and on tool calls inside a subagent. If a subagent file
    # (<session>/subagents/<agent>.jsonl) ever arrives instead, go up to the
    # session's own file, so latest.jsonl never shrinks to one subagent.
    sub_dir = os.path.dirname(tpath)
    if os.path.basename(sub_dir) == "subagents":
        main = os.path.dirname(sub_dir) + ".jsonl"
        if os.path.isfile(main):
            return main
    return tpath


def in_cloud():
    return os.environ.get("CLAUDE_CODE_ENTRYPOINT") == CLOUD_ENTRYPOINT


def now_iso(t=None):
    t = time.time() if t is None else t
    return time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(t)) + ".%03dZ" % int((t % 1) * 1000)


def diag(hook, mode, result, **extra):
    # 1.1.1 diagnostic: one line per hook run, so a live test can tell when
    # each hook fired inside the session. Timestamps, counts and the short
    # session id only; never content or paths. Never raises.
    try:
        os.makedirs(OUT, exist_ok=True)
        p = os.path.join(OUT, "diag.log")
        if os.path.exists(p) and os.path.getsize(p) > DIAG_MAX_BYTES:
            os.replace(p, p + ".1")
        line = {"at": now_iso(), "v": __version__, "hook": hook, "mode": mode, "result": result}
        line.update(extra)
        with open(p, "a", encoding="utf-8") as f:
            f.write(json.dumps(line, separators=(",", ":")) + "\n")
    except Exception:
        pass


def latest_age():
    try:
        return time.time() - os.path.getmtime(os.path.join(OUT, "latest.jsonl"))
    except OSError:
        return None


def watch():
    if not in_cloud():
        return
    out = {"hookSpecificOutput": {
        "hookEventName": "SessionStart",
        "watchPaths": [OUT],
    }}
    sys.stdout.write(json.dumps(out, separators=(",", ":")) + "\n")
    diag("SessionStart", "watch", "watch registered")


def fired():
    # FileChanged command hook, run in parallel with the copy hooks of the
    # same event: its timestamp is the moment the copy started.
    if not in_cloud():
        return
    hook = json.load(sys.stdin)
    p = os.path.join(OUT, "latest.jsonl")
    try:
        mtime = now_iso(os.path.getmtime(p))
        with open(p, encoding="utf-8") as f:
            nrows = sum(1 for l in f if l.strip())
    except OSError:
        mtime, nrows = None, None
    diag("FileChanged", "fired", "logged", sid=(hook.get("session_id") or "")[:8],
         event=hook.get("event"), latest_mtime=mtime, latest_rows=nrows)


def extract(throttle):
    if os.environ.get("CLAUDE_CODE_ENTRYPOINT", CLOUD_ENTRYPOINT) != CLOUD_ENTRYPOINT:
        return
    hook = json.load(sys.stdin)
    event = hook.get("hook_event_name") or "unknown"
    sid = hook.get("session_id") or "unknown"
    mode = "throttle" if throttle else "extract"
    if throttle:
        age = latest_age()
        if age is not None and age < THROTTLE_SECONDS:
            diag(event, mode, "throttled", sid=sid[:8], age_s=round(age, 1))
            return
    tpath = hook.get("transcript_path") or ""
    if not os.path.isfile(tpath):
        diag(event, mode, "no transcript", sid=sid[:8])
        return
    tpath = main_transcript(tpath)
    rows = read_rows(tpath, "main")
    for p in glob.glob(os.path.join(os.path.splitext(tpath)[0], "subagents", "*.jsonl")):
        rows.update(read_rows(p, os.path.splitext(os.path.basename(p))[0]))
    # Cloud sessions only. A local Cowork or Claude Code session already
    # leaves its own transcript on the user's computer, and BurnMon reads
    # that; sending it again would count it twice under Cowork (Cloud).
    out = sorted((r for r in rows.values() if r["entrypoint"] == CLOUD_ENTRYPOINT),
                 key=lambda r: (r["ts"] or "", r["request_id"] or ""))
    if not out:
        if in_cloud():
            diag(event, mode, "no cloud rows", sid=sid[:8])
        return
    body = "".join(json.dumps(r, separators=(",", ":")) + "\n" for r in out)
    os.makedirs(OUT, exist_ok=True)
    # The session file: temp file plus rename, never half written.
    tmp = os.path.join(OUT, sid + ".jsonl." + str(os.getpid()) + ".tmp")
    with open(tmp, "w", encoding="utf-8") as f:
        f.write(body)
    os.replace(tmp, os.path.join(OUT, sid + ".jsonl"))
    # latest.jsonl last, since its write fires the FileChanged copy, and in
    # place: a rename gives the file a new inode, and the watcher of a file
    # that already existed then never fires again (measured 2026-10-12, 1.1.1).
    # Write first, cut the tail after, so the file is never empty; rows only
    # grow, so a reader at worst sees the old tail for a moment.
    write_in_place(os.path.join(OUT, "latest.jsonl"), body)
    diag(event, mode, "wrote", sid=sid[:8], rows=len(out), newest=out[-1]["ts"])
    spawn_settle(hashlib.sha256(body.encode("utf-8")).hexdigest())


def spawn_settle(digest):
    # Detached and silent, so the hook returns at once and the runner never
    # waits on this process's output.
    subprocess.Popen([sys.executable, os.path.abspath(__file__), "--settle", digest],
                     stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                     stderr=subprocess.DEVNULL, start_new_session=True, close_fds=True)


def settle(digest):
    if not in_cloud():
        return
    time.sleep(SETTLE_SECONDS)
    p = os.path.join(OUT, "latest.jsonl")
    try:
        with open(p, "rb") as f:
            data = f.read()
    except OSError:
        return
    if hashlib.sha256(data).hexdigest() != digest:
        diag("settle", "settle", "changed, skipped")
        return
    write_in_place(p, data.decode("utf-8"))
    diag("settle", "settle", "rewrote", latest_rows=data.count(b"\n"))


def write_in_place(path, body):
    data = body.encode("utf-8")
    fd = os.open(path, os.O_RDWR | os.O_CREAT, 0o644)
    try:
        view = memoryview(data)
        while view:
            view = view[os.write(fd, view):]
        os.ftruncate(fd, len(data))
    finally:
        os.close(fd)


def main(argv):
    flags = set(argv)
    if argv[:1] == ["--settle"] and len(argv) == 2:
        settle(argv[1])
        return
    unknown = flags - {"--watch", "--throttle", "--fired"}
    if unknown:
        raise ValueError("unknown flag " + " ".join(sorted(unknown)))
    if "--watch" in flags:
        watch()
    elif "--fired" in flags:
        fired()
    else:
        extract("--throttle" in flags)


def run(argv):
    try:
        main(argv)
    except Exception as e:  # never break the user's turn
        try:
            os.makedirs(OUT, exist_ok=True)
            with open(os.path.join(OUT, "errors.log"), "a", encoding="utf-8") as f:
                f.write(json.dumps({"at": time.time(), "v": __version__,
                                    "error": repr(e)[:300]}) + "\n")
        except Exception:
            pass


if __name__ == "__main__":
    run(sys.argv[1:])
    sys.exit(0)
