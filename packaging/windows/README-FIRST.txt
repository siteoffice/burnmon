BurnMon on Windows: read this first
===================================

This folder holds three apps and one command line program:

  burnmon.exe          BurnMon: history, tokens and cost per day, vendor, project and client
  burnmon-dev.exe      BurnMon Dev: the last 30 minutes, who waits on you, the agent map
  controlcentre.exe    Control Centre: units, files, plan, boards and a Git overview
  burnmon-cli.exe      scripts, exports and merges (run it in PowerShell)
  burnmon.example.json an example of the settings file

To start
--------

1. Unzip the download to a folder of its own, for example C:\Tools\burnmon.
2. Double-click burnmon.exe, burnmon-dev.exe or controlcentre.exe.

The programs are not signed in this release. Windows may show a warning for
an unsigned program before it starts one; what it shows on a fresh download is
not tested yet. Signing follows in a later release.

The windows use Microsoft Edge WebView2, which Windows 10 and 11 normally have.

Settings
--------

The settings file is %LOCALAPPDATA%\burnmon\burnmon.json; burnmon.exe reads a
burnmon.json next to it first. Copy burnmon.example.json there as
burnmon.json to change a setting. Without the file every setting keeps its
default.

BurnMon reads the transcripts your coding tools already write on this
computer. It needs no account and makes no network call of its own. With
"Read Cowork (Cloud) sessions" on, it starts your own Claude Code CLI, which
goes online.

More: https://github.com/siteoffice/burnmon
