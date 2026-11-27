Golden files for TestRecipesGolden. Name: <recipe>_<win|mac>.txt.
Each holds the block snip.py v0.7.0 prints for the fixed call in goldenRequest
(recipes_test.go): cwd C:\w\r or /w/r, engine C:\g\git_flow.py or /g/git_flow.py,
files a.txt, message msg, tag v1, subject s.

A Windows host needs Python 3 (py -3) and the folder that holds snip.py. It makes
all ten files, the Windows and the Mac ones, in one run. Run it from the repo root:
    py -3 internal\recipes\testdata\make_golden.py <folder that holds snip.py>
make_golden.py writes into its own folder, prints one line per file and a count of
refused calls, and exits 1 if any call was refused. A refused call writes no file.

No turn runs python. Without the files the test logs it and checks the inline cases only.
