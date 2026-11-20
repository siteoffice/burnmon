package winbind

import "testing"

// The first window is the wanted size, limited to the visible area of the
// screen minus FirstWindowMargin on each axis.
func TestFitFirstWindow(t *testing.T) {
	cases := []struct {
		name         string
		wantW, wantH int
		visW, visH   int
		gotW, gotH   int
	}{
		{"1024x768 dev", 1280, 860, 1024, 768, 1024 - FirstWindowMargin, 768 - FirstWindowMargin},
		{"1024x768 centre", 1536, 864, 1024, 768, 1024 - FirstWindowMargin, 768 - FirstWindowMargin},
		{"1470x956 dev", 1280, 860, 1470, 956, 1280, 860},
		{"1470x956 centre", 1536, 864, 1470, 956, 1470 - FirstWindowMargin, 864},
		{"1920x1080 dev", 1280, 860, 1920, 1080, 1280, 860},
		{"1920x1080 centre", 1536, 864, 1920, 1080, 1536, 864},
		{"no screen read", 1280, 860, 0, 0, 1280, 860},
		{"screen smaller than margin", 1280, 860, 50, 50, 1280, 860},
	}
	for _, c := range cases {
		w, h := FitFirstWindow(c.wantW, c.wantH, c.visW, c.visH)
		if w != c.gotW || h != c.gotH {
			t.Errorf("%s: FitFirstWindow(%d,%d,%d,%d) = %dx%d, want %dx%d",
				c.name, c.wantW, c.wantH, c.visW, c.visH, w, h, c.gotW, c.gotH)
		}
	}
}
