package winbind

import (
	"reflect"
	"testing"
)

func TestPageCallsCalledMinusDefined(t *testing.T) {
	src := `window.bmLive(1); bmTurn ('a'); function bmLocal(){} bmLocal(); window.ccReload = function(){}; window.ccReload && ccReload(); __bmHidden(); x.bmNot; if(typeof window.bmX === 'function'){}`
	got := PageCalls(src, "bm", "cc")
	want := []string{"bmLive", "bmTurn"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PageCalls = %v, want %v", got, want)
	}
}

func TestPageCallsEmpty(t *testing.T) {
	if got := PageCalls("", "bdev"); len(got) != 0 {
		t.Fatalf("PageCalls empty = %v, want none", got)
	}
}
