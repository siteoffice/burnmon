package main

import (
	"context"
	"log"

	"burnmon/internal/cloudpull"
	"burnmon/internal/pricing"
	"burnmon/internal/snapshotwriter"
)

// startCloudPull runs the Cowork (Cloud) pull supervisor for the app's life.
// It re-reads cloud_pull from burnmon.json every 30 seconds, so a switch
// flipped in BurnMon Dev counts here too, and keeps a.cfg.CloudPull in step
// for the vendor strip. Off (the default) starts no CLI.
func (a *app) startCloudPull() {
	s := cloudpull.NewSupervisor(func() bool {
		on := pricing.ReadCloudPull(a.cfgPath)
		a.mu.Lock()
		a.cfg.CloudPull = on
		a.mu.Unlock()
		return on
	}, snapshotwriter.NewLock(cloudpull.LockPath()))
	go s.Run(context.Background(), func(err error) { log.Println("cloud pull:", err) })
}
