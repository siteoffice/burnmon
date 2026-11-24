package vendorstrip

import (
	"path/filepath"
	"testing"
	"time"

	"burnmon/internal/store"
)

func cloudModFixture(t *testing.T, newest time.Time, have bool) Payload {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "burnmon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	old := cloudModNewest
	cloudModNewest = func() (time.Time, bool) { return newest, have }
	defer func() { cloudModNewest = old }()
	p, err := Build(st, nil, time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Decision 3 of the cloud mod route: the payload says whether the
// burnmon-cloud mod has delivered a file, so the page can show the day mods
// start to run in cloud sessions.
func TestCloudModNoDeliveryYet(t *testing.T) {
	p := cloudModFixture(t, time.Time{}, false)
	if p.CloudMod != "cloud mod: no delivery yet" || p.CloudModDelivered {
		t.Fatalf("CloudMod = %q delivered=%v, want no delivery yet", p.CloudMod, p.CloudModDelivered)
	}
}

func TestCloudModNewestDelivery(t *testing.T) {
	at := time.Date(2026, 11, 24, 9, 30, 0, 0, time.UTC)
	p := cloudModFixture(t, at, true)
	want := "cloud mod: newest delivery " + at.In(time.Local).Format("2006-01-02 15:04")
	if p.CloudMod != want || !p.CloudModDelivered {
		t.Fatalf("CloudMod = %q delivered=%v, want %q", p.CloudMod, p.CloudModDelivered, want)
	}
}
