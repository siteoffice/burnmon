# Cowork cloud pull, step 6: report on retiring the burnmon-cloud plugin

Flow `burnmon/cowork-cloud-pull`, step 6 of 7. Parent spec: section 13 ("Out of scope here, but report on
it") of `C:\ZND\10_holding\04_assets\2026-11-17_burnmon_cowork_cloud_gap_research_and_proposal.md`.
Date: 2026-11-17. Status: spec, awaiting the spec gate.

## What changes

One report, no code: `C:\ZND\50_projects\burnmon\04_assets\2026-11-17_burnmon_cloud_plugin_retirement.md`.
It proposes; Wilco decides. Uninstalling the plugin from an organisation or a session is a human step
on the Plugins page and stays out of this run.

The report answers, each with its source (read live) or marked inferred:
1. What the plugin is and does today: `tools\burnmon-cloud` (plugin, extract script, build, tests, `dist`
   packages), which hooks it runs in a cloud session, and the version that is installed (the hub document
   names 4.2.2 with extract script 1.1.4; the repo's `plugin.json` says 1.1.4, and `dist` stops at 3.4.2;
   the report says which is which or that they disagree).
2. Evidence that it no longer delivers: the last drop row (2026-11-12 16:43 UTC), the cloud container
   without transcripts, and what the cloud pull now measures instead.
3. What it still costs: a hook on every tool call in every cloud session where it is installed (its run
   time per call if the repo or a log shows it; otherwise inferred), and the drop folders BurnMon still
   watches.
4. What retiring means in BurnMon, as a proposal with the files named: the drop parser
   (`internal\adapter\claude\cloud.go`) and the old 8,911 drop rows stay readable for history; the
   `not measured` marker driven by the drop rows; `scan.CloudDropDirs` and `EnsureCloudDropDir`; the README
   section "Cowork in the cloud"; `tools\burnmon-cloud` itself (archive, or delete after a release).
5. The human steps, in order, with who does them: uninstall from the Plugins page, the README change, and
   a later BurnMon change (a separate flow, not this one).
6. A recommendation with pros and cons, and the open questions for Wilco.

## What stays

All code, the plugin source and its packages, the drop rows in the store. Nothing is uninstalled or deleted.

## How to check

- Every fact in the report carries its source path or the word inferred.
- No em dash; paths in full `C:\...` form; ends with a Close-out block.
- No code changed, so vet, test and build are not rerun.
