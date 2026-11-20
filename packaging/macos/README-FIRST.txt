BurnMon on a Mac: read this first
=================================

This folder holds three apps and one command line program:

  BurnMon.app          history: tokens and cost per day, vendor, project and client
  BurnMon Dev.app      the last 30 minutes: who waits on you, the agent map
  Control Centre.app   steering: units, files, plan, boards and a Git overview
  burnmon-cli          scripts, exports and merges (run it in Terminal)
  burnmon.example.json an example of the settings file

The apps are not signed in this release. Signing follows in a later release.
Until then macOS blocks a downloaded app it cannot check.

To start
--------

1. Unzip the download. You get this folder.
2. Open Terminal and run this one line, with the path of this folder
   (type "xattr -dr com.apple.quarantine " and drag the folder onto Terminal):

     xattr -dr com.apple.quarantine ~/Downloads/<this folder>

   It removes the download mark that makes macOS block the apps.
3. Double-click BurnMon.app, BurnMon Dev.app or Control Centre.app.

You may move the apps to Applications first; run the xattr line on them there.

Settings
--------

The apps read burnmon.json from ~/Library/Application Support/burnmon/.
Copy burnmon.example.json there as burnmon.json to change a setting. Without
the file every setting keeps its default.

BurnMon reads the transcripts your coding tools already write on this Mac. It
needs no account and makes no network call of its own. With "Read Cowork
(Cloud) sessions" on, it starts your own Claude Code CLI, which goes online.

More: https://github.com/siteoffice/burnmon
