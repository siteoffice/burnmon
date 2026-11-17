"""Tests for the BurnMon cloud extract hook (plugin/scripts/extract.py).

Runs on any computer with Python 3, no cloud session needed:

    cd tools/burnmon-cloud
    python3 -m unittest discover -s tests -v

Each test points the script's output folder at a temp folder and sets
CLAUDE_CODE_ENTRYPOINT itself, so it covers both the cloud environment
(remote_cowork) and a local one.
"""
import importlib.util
import io
import json
import os
import sys
import tempfile
import time
import unittest
from contextlib import redirect_stdout
from unittest import mock

HERE = os.path.dirname(os.path.abspath(__file__))
PLUGIN = os.path.join(os.path.dirname(HERE), "plugin")
SCRIPT = os.path.join(PLUGIN, "scripts", "extract.py")

WHITELIST = {
    "v", "source", "ts", "session_id", "request_id", "message_id", "model",
    "agent", "is_sidechain", "entrypoint", "service_tier", "speed",
    "cache_5m", "cache_1h", "thinking_tokens", "input_tokens",
    "output_tokens", "cache_creation_input_tokens", "cache_read_input_tokens",
}
SECRETS = ("SECRET_TEXT", "SECRET_TOOL", "/secret/cwd", "secret-branch", "SECRET_PROMPT")


def load():
    sys.dont_write_bytecode = True  # keep plugin/scripts free of __pycache__
    spec = importlib.util.spec_from_file_location("extract_under_test", SCRIPT)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def assistant(req, msg, out_tokens, ts, entrypoint="remote_cowork", sidechain=False):
    return {
        "type": "assistant", "timestamp": ts, "sessionId": "sess-1",
        "requestId": req, "isSidechain": sidechain, "entrypoint": entrypoint,
        "cwd": "/secret/cwd", "gitBranch": "secret-branch",
        "message": {
            "id": msg, "model": "claude-opus-5-5", "role": "assistant",
            "content": [{"type": "text", "text": "SECRET_TEXT"},
                        {"type": "tool_use", "name": "Bash", "input": {"command": "SECRET_TOOL"}}],
            "usage": {
                "input_tokens": 10, "output_tokens": out_tokens,
                "cache_creation_input_tokens": 100, "cache_read_input_tokens": 1000,
                "cache_creation": {"ephemeral_5m_input_tokens": 40, "ephemeral_1h_input_tokens": 60},
                "output_tokens_details": {"thinking_tokens": 3},
                "service_tier": "standard", "speed": "standard",
            },
        },
    }


def user(text, entrypoint="remote_cowork"):
    return {"type": "user", "timestamp": "2026-10-12T10:00:00Z", "sessionId": "sess-1",
            "entrypoint": entrypoint, "cwd": "/secret/cwd",
            "message": {"role": "user", "content": text}}


def write_jsonl(path, rows):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        for r in rows:
            f.write(json.dumps(r) + "\n")


class Base(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = self.tmp.name
        self.out = os.path.join(self.root, "outputs", ".burnmon", "cloud")
        self.mod = load()
        self.mod.OUT = self.out
        # 1.1.3: extract starts a detached settle process; record it instead.
        self.spawned = []
        self.mod.spawn_settle = lambda digest: self.spawned.append(digest)
        self.transcript = os.path.join(self.root, "projects", "p", "sess-1.jsonl")
        self.subdir = os.path.join(self.root, "projects", "p", "sess-1", "subagents")

    def tearDown(self):
        self.tmp.cleanup()

    def run_hook(self, argv, entrypoint="remote_cowork", stdin=None, event="Stop"):
        if stdin is None:
            stdin = json.dumps({"session_id": "sess-1", "transcript_path": self.transcript,
                                "hook_event_name": event})
        env = dict(os.environ)
        env.pop("CLAUDE_CODE_ENTRYPOINT", None)
        if entrypoint is not None:
            env["CLAUDE_CODE_ENTRYPOINT"] = entrypoint
        buf = io.StringIO()
        with mock.patch.dict(os.environ, env, clear=True), \
                mock.patch.object(sys, "stdin", io.StringIO(stdin)), redirect_stdout(buf):
            self.mod.run(argv)
        return buf.getvalue()

    def latest(self):
        return os.path.join(self.out, "latest.jsonl")

    def read_latest(self):
        with open(self.latest(), encoding="utf-8") as f:
            return [json.loads(l) for l in f if l.strip()]

    def errors(self):
        p = os.path.join(self.out, "errors.log")
        return open(p, encoding="utf-8").read() if os.path.exists(p) else ""

    def basic_transcript(self, entrypoint="remote_cowork"):
        write_jsonl(self.transcript, [
            user("SECRET_PROMPT", entrypoint),
            assistant("req_1", "msg_1", 8, "2026-10-12T10:00:01Z", entrypoint),
            assistant("req_1", "msg_1", 148, "2026-10-12T10:00:02Z", entrypoint),
            assistant("req_2", "msg_2", 20, "2026-10-12T10:00:05Z", entrypoint),
        ])


class ExtractTest(Base):
    # version

    def test_version_matches_manifest(self):
        with open(os.path.join(PLUGIN, ".claude-plugin", "plugin.json"), encoding="utf-8") as f:
            manifest = json.load(f)
        self.assertEqual(manifest["version"], "1.1.4")
        self.assertEqual(self.mod.__version__, manifest["version"])

    def test_manifest_stays_enabled_by_default(self):
        # defaultEnabled false also switched the hooks off in Cowork (tested 3.4.2),
        # so no copy ran and no usage reached the laptop. Never set it to false.
        with open(os.path.join(PLUGIN, ".claude-plugin", "plugin.json"), encoding="utf-8") as f:
            manifest = json.load(f)
        self.assertNotIn("defaultEnabled", manifest)

    # --watch

    def test_watch_in_cloud_prints_watch_paths(self):
        out = self.run_hook(["--watch"], stdin="")
        self.assertEqual(json.loads(out), {"hookSpecificOutput": {
            "hookEventName": "SessionStart",
            "watchPaths": [self.out]}})
        self.assertFalse(os.path.exists(self.latest()), "--watch must not create latest.jsonl")
        self.assertEqual(self.errors(), "")

    def test_watch_path_is_the_staged_folder_in_production(self):
        # 1.1.4: the folder, not the file (the file watch went deaf after a few events).
        self.assertEqual(load().OUT, "/mnt/user-data/outputs/.burnmon/cloud")

    def test_watch_outside_cloud_prints_nothing(self):
        self.assertEqual(self.run_hook(["--watch"], entrypoint="cli", stdin=""), "")
        self.assertEqual(self.run_hook(["--watch"], entrypoint="local-agent", stdin=""), "")
        self.assertEqual(self.run_hook(["--watch"], entrypoint=None, stdin=""), "")

    def test_watch_does_not_extract(self):
        self.basic_transcript()
        self.run_hook(["--watch"])
        self.assertFalse(os.path.exists(self.latest()))

    # plain extract

    def test_extract_writes_whitelisted_last_rows(self):
        self.basic_transcript()
        self.assertEqual(self.run_hook([]), "")
        rows = self.read_latest()
        self.assertEqual([r["request_id"] for r in rows], ["req_1", "req_2"])
        self.assertEqual(rows[0]["output_tokens"], 148, "last row of a streamed request wins")
        for r in rows:
            self.assertEqual(set(r), WHITELIST)
            self.assertEqual(r["agent"], "main")
            self.assertEqual(r["cache_5m"], 40)
            self.assertEqual(r["thinking_tokens"], 3)
        with open(self.latest(), encoding="utf-8") as f:
            body = f.read()
        for s in SECRETS:
            self.assertNotIn(s, body)
        with open(os.path.join(self.out, "sess-1.jsonl"), encoding="utf-8") as f:
            self.assertEqual(f.read(), body)
        self.assertEqual([n for n in os.listdir(self.out) if n.endswith(".tmp")], [])

    def test_latest_is_rewritten_in_place(self):
        # 1.1.2: a rename gives latest.jsonl a new inode, and the FileChanged
        # watcher of an existing file then never fires again (measured
        # 2026-10-12). The same inode must survive every rewrite.
        self.basic_transcript()
        self.run_hook([])
        ino = os.stat(self.latest()).st_ino
        write_jsonl(self.transcript, [assistant("req_1", "msg_1", 8, "2026-10-12T10:00:01Z")])
        self.run_hook([])
        self.assertEqual(os.stat(self.latest()).st_ino, ino)
        self.assertEqual([r["request_id"] for r in self.read_latest()], ["req_1"],
                         "a shorter extract leaves no stale tail")

    def test_extract_skips_local_rows(self):
        self.basic_transcript(entrypoint="cli")
        self.run_hook([], entrypoint="cli")
        self.assertFalse(os.path.exists(self.latest()))

    def test_extract_includes_subagents(self):
        self.basic_transcript()
        write_jsonl(os.path.join(self.subdir, "agent-abc.jsonl"), [
            assistant("req_9", "msg_9", 30, "2026-10-12T10:00:03Z", sidechain=True)])
        self.run_hook([], event="SubagentStop")
        rows = {r["request_id"]: r for r in self.read_latest()}
        self.assertEqual(rows["req_9"]["agent"], "agent-abc")
        self.assertTrue(rows["req_9"]["is_sidechain"])
        self.assertIn("req_1", rows)

    def test_extract_from_subagent_transcript_path_still_covers_the_session(self):
        # Docs: transcript_path is the main transcript on SubagentStop. Guard
        # anyway: if a hook ever hands the subagent file, the extract must
        # not shrink latest.jsonl to that one subagent.
        self.basic_transcript()
        sub = os.path.join(self.subdir, "agent-abc.jsonl")
        write_jsonl(sub, [assistant("req_9", "msg_9", 30, "2026-10-12T10:00:03Z", sidechain=True)])
        stdin = json.dumps({"session_id": "sess-1", "transcript_path": sub})
        self.run_hook([], stdin=stdin)
        ids = sorted(r["request_id"] for r in self.read_latest())
        self.assertEqual(ids, ["req_1", "req_2", "req_9"])

    def test_bad_input_never_raises(self):
        self.run_hook([], stdin="not json")
        self.assertIn("error", self.errors())
        self.assertFalse(os.path.exists(self.latest()))

    def test_unknown_flag_never_raises(self):
        self.basic_transcript()
        self.run_hook(["--bogus"])
        self.assertIn("error", self.errors())

    # --throttle

    def test_throttle_skips_when_written_recently(self):
        self.basic_transcript()
        os.makedirs(self.out)
        with open(self.latest(), "w", encoding="utf-8") as f:
            f.write("OLD\n")
        self.run_hook(["--throttle"], event="PostToolUse")
        with open(self.latest(), encoding="utf-8") as f:
            self.assertEqual(f.read(), "OLD\n")

    def test_throttle_extracts_when_older_than_20s(self):
        self.basic_transcript()
        os.makedirs(self.out)
        with open(self.latest(), "w", encoding="utf-8") as f:
            f.write("OLD\n")
        old = time.time() - 21
        os.utime(self.latest(), (old, old))
        self.run_hook(["--throttle"], event="PostToolUse")
        self.assertEqual(len(self.read_latest()), 2)

    def test_throttle_extracts_when_no_file_yet(self):
        self.basic_transcript()
        self.run_hook(["--throttle"], event="PostToolUse")
        self.assertEqual(len(self.read_latest()), 2)

    def test_throttle_does_not_apply_without_flag(self):
        self.basic_transcript()
        os.makedirs(self.out)
        with open(self.latest(), "w", encoding="utf-8") as f:
            f.write("OLD\n")
        self.run_hook([])
        self.assertEqual(len(self.read_latest()), 2)


DIAG_KEYS = {"at", "v", "sid", "hook", "mode", "result", "rows", "newest", "event",
             "latest_mtime", "latest_rows", "age_s"}


class DiagTest(Base):
    """1.1.1: diag.log records when each hook ran and what it did, timestamps only."""

    def diag(self):
        p = os.path.join(self.out, "diag.log")
        if not os.path.exists(p):
            return []
        with open(p, encoding="utf-8") as f:
            return [json.loads(l) for l in f if l.strip()]

    def fired_input(self, event="change"):
        return json.dumps({"session_id": "sess-1", "transcript_path": self.transcript,
                           "hook_event_name": "FileChanged", "event": event,
                           "file_path": os.path.join(self.out, "latest.jsonl")})

    def test_watch_logs_session_start(self):
        self.run_hook(["--watch"], stdin="")
        d = self.diag()
        self.assertEqual(len(d), 1)
        self.assertEqual((d[0]["hook"], d[0]["mode"], d[0]["result"]),
                         ("SessionStart", "watch", "watch registered"))

    def test_extract_logs_write(self):
        self.basic_transcript()
        self.run_hook([], event="Stop")
        d = self.diag()
        self.assertEqual(len(d), 1)
        self.assertEqual((d[0]["hook"], d[0]["mode"], d[0]["result"], d[0]["rows"]),
                         ("Stop", "extract", "wrote", 2))
        self.assertEqual(d[0]["newest"], "2026-10-12T10:00:05Z")
        self.assertEqual(d[0]["sid"], "sess-1")

    def test_throttle_logs_skip_with_age(self):
        self.basic_transcript()
        os.makedirs(self.out)
        with open(self.latest(), "w", encoding="utf-8") as f:
            f.write("OLD\n")
        self.run_hook(["--throttle"], event="PostToolUse")
        d = self.diag()
        self.assertEqual((d[0]["hook"], d[0]["mode"], d[0]["result"]),
                         ("PostToolUse", "throttle", "throttled"))
        self.assertLess(d[0]["age_s"], 20)

    def test_fired_logs_file_changed(self):
        self.basic_transcript()
        self.run_hook([], event="Stop")
        out = self.run_hook(["--fired"], stdin=self.fired_input())
        self.assertEqual(out, "", "--fired prints nothing")
        d = self.diag()
        self.assertEqual(len(d), 2)
        f = d[1]
        self.assertEqual((f["hook"], f["mode"], f["event"], f["latest_rows"]),
                         ("FileChanged", "fired", "change", 2))
        self.assertIn("latest_mtime", f)
        self.assertEqual(len(self.read_latest()), 2, "--fired never writes latest.jsonl")

    def test_fired_when_file_is_gone(self):
        self.run_hook(["--fired"], stdin=self.fired_input("unlink"))
        d = self.diag()
        self.assertEqual((d[0]["event"], d[0]["latest_rows"]), ("unlink", None))

    def test_nothing_logged_outside_cloud(self):
        self.basic_transcript(entrypoint="cli")
        self.run_hook([], entrypoint="cli")
        self.run_hook(["--throttle"], entrypoint="cli")
        self.run_hook(["--fired"], entrypoint="cli", stdin=self.fired_input())
        self.run_hook(["--watch"], entrypoint="cli", stdin="")
        self.assertFalse(os.path.exists(self.out), "no files at all outside a cloud session")

    def test_diag_holds_no_content(self):
        self.basic_transcript()
        self.run_hook(["--watch"], stdin="")
        self.run_hook([], event="SubagentStop")
        self.run_hook(["--throttle"], event="PostToolUse")
        self.run_hook(["--fired"], stdin=self.fired_input())
        with open(os.path.join(self.out, "diag.log"), encoding="utf-8") as f:
            body = f.read()
        for s in SECRETS + (self.root,):
            self.assertNotIn(s, body)
        for line in self.diag():
            self.assertLessEqual(set(line), DIAG_KEYS)
            self.assertEqual(line["v"], self.mod.__version__)

    def test_diag_rotates_past_256_kb(self):
        os.makedirs(self.out)
        p = os.path.join(self.out, "diag.log")
        with open(p, "w", encoding="utf-8") as f:
            f.write("x" * (256 * 1024 + 1))
        self.run_hook(["--watch"], stdin="")
        self.assertTrue(os.path.exists(p + ".1"))
        self.assertEqual(len(self.diag()), 1)



class SettleTest(Base):
    """1.1.3: a copy that runs within about 5 s of a write delivers the
    previous content (measured 2026-10-12: the outputs folder uploads in the
    background, and the copy reads the uploaded file). So every write is
    followed, about 10 s later, by an identical in-place rewrite whose copy
    finds the new content uploaded."""

    def test_extract_schedules_one_settle_with_the_body_digest(self):
        import hashlib
        self.basic_transcript()
        self.run_hook([])
        with open(self.latest(), "rb") as f:
            digest = hashlib.sha256(f.read()).hexdigest()
        self.assertEqual(self.spawned, [digest])

    def test_no_settle_without_a_write(self):
        self.run_hook([])  # no transcript
        self.basic_transcript()
        os.makedirs(self.out, exist_ok=True)
        with open(self.latest(), "w", encoding="utf-8") as f:
            f.write("OLD\n")
        self.run_hook(["--throttle"], event="PostToolUse")
        self.assertEqual(self.spawned, [])

    def settle(self, digest):
        self.mod.SETTLE_SECONDS = 0
        self.run_hook(["--settle", digest], stdin="")

    def test_settle_rewrites_unchanged_file_in_place(self):
        import hashlib
        self.basic_transcript()
        self.run_hook([])
        ino = os.stat(self.latest()).st_ino
        old = time.time() - 30
        os.utime(self.latest(), (old, old))
        with open(self.latest(), "rb") as f:
            body = f.read()
        self.settle(hashlib.sha256(body).hexdigest())
        st = os.stat(self.latest())
        self.assertEqual(st.st_ino, ino)
        self.assertGreater(st.st_mtime, old + 20, "the rewrite must touch the file")
        with open(self.latest(), "rb") as f:
            self.assertEqual(f.read(), body)
        diag = [json.loads(l) for l in open(os.path.join(self.out, "diag.log"))]
        self.assertEqual((diag[-1]["hook"], diag[-1]["result"]), ("settle", "rewrote"))

    def test_settle_skips_when_a_newer_write_landed(self):
        self.basic_transcript()
        self.run_hook([])
        old = time.time() - 30
        os.utime(self.latest(), (old, old))
        self.settle("0" * 64)
        self.assertLess(os.stat(self.latest()).st_mtime, old + 1, "no rewrite")
        diag = [json.loads(l) for l in open(os.path.join(self.out, "diag.log"))]
        self.assertEqual((diag[-1]["hook"], diag[-1]["result"]), ("settle", "changed, skipped"))

    def test_settle_does_nothing_outside_cloud(self):
        self.mod.SETTLE_SECONDS = 0
        self.run_hook(["--settle", "0" * 64], entrypoint="cli", stdin="")
        self.assertFalse(os.path.exists(self.out))


if __name__ == "__main__":
    unittest.main()
