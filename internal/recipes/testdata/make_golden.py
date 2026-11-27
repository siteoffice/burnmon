"""Make the golden files of TestRecipesGolden from snip.py. Run on Windows or a Mac:
    py -3 internal\recipes\testdata\make_golden.py <folder that holds snip.py>
Two bypasses, both needed because the golden call uses made-up paths:
 1. the "folder exists" check for the made-up cwd of the host system;
 2. the engine path, which snip.py takes from its own folder on the host system.
Neither changes how a line is built."""
import contextlib, io, os, pathlib, sys

def main(snip_dir):
    sys.path.insert(0, snip_dir)
    import snip
    out = pathlib.Path(__file__).resolve().parent
    cwd = {"win": "C:\\w\\r", "mac": "/w/r"}
    eng = {"win": "C:\\g\\git_flow.py", "mac": "/g/git_flow.py"}
    real_isdir = os.path.isdir
    snip.os.path.isdir = lambda p: True if p == cwd[snip.HOST_OS] else real_isdir(p)
    snip.ENGINE_PATH = eng[snip.HOST_OS]
    bad = 0
    for osn in ("win", "mac"):
        shared = ["--os", osn, "--cwd", cwd[osn]]
        calls = {
            "save": ["git", "save"] + shared + ["--files", "a.txt", "--message", "msg"],
            "share": ["git", "share"] + shared,
            "land": ["git", "land"] + shared + ["--subject", "s", "--engine", eng[osn]],
            "tag": ["git", "tag"] + shared + ["--tag", "v1", "--message", "msg"],
            "sync": ["git", "sync"] + shared,
        }
        for name, argv in calls.items():
            buf = io.StringIO()
            with contextlib.redirect_stdout(buf):
                code = snip.main(argv)
            data = buf.getvalue().encode("utf-8")
            if code != 0 or not data:
                bad += 1
                print(name, osn, "REFUSED, exit", code, "- file not written")
                continue
            (out / (name + "_" + osn + ".txt")).write_bytes(data)
            print(name, osn, "exit", code, len(data), "bytes")
    print("refused calls:", bad)
    return 1 if bad else 0

if __name__ == "__main__":
    sys.exit(main(sys.argv[1]))
