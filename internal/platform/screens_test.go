package platform

import (
	"image"
	"testing"
)

func TestFitOnScreens(t *testing.T) {
	r := image.Rect
	// A laptop (primary, task bar at the bottom) and a monitor to its
	// right, taller and higher up.
	laptop := r(0, 0, 1920, 1040)
	right := r(1920, -200, 4480, 1240)
	both := []image.Rectangle{laptop, right}
	for _, tt := range []struct {
		name  string
		win   image.Rectangle
		works []image.Rectangle
		want  image.Rectangle
	}{
		{"no screens known", r(5000, 5000, 5800, 5600), nil, r(5000, 5000, 5800, 5600)},
		{"only empty screens", r(5000, 5000, 5800, 5600), []image.Rectangle{{}, r(10, 10, 10, 20)}, r(5000, 5000, 5800, 5600)},
		{"inside the primary", r(100, 100, 900, 700), both, r(100, 100, 900, 700)},
		{"inside the second", r(2000, -100, 2800, 500), both, r(2000, -100, 2800, 500)},
		{"spanning both", r(1500, 100, 2300, 700), both, r(1500, 100, 2300, 700)},
		{"mostly off the right edge, enough left", r(4380, 100, 5180, 700), both, r(4380, 100, 5180, 700)},
		{"mostly below, enough left", r(100, 990, 900, 1590), both, r(100, 990, 900, 1590)},
		{"too little left at the right edge", r(4400, 100, 5200, 700), both, r(3680, 100, 4480, 700)},
		{"too little left at the bottom", r(100, 1000, 900, 1600), both, r(100, 440, 900, 1040)},
		{"top above the work area", r(100, -10, 900, 590), both, r(100, 0, 900, 600)},
		{"top under the second's top", r(2000, -300, 2800, 300), both, r(2000, -200, 2800, 400)},
		{"left of everything", r(-2000, 100, -1200, 700), both, r(0, 100, 800, 700)},
		{"on a monitor that is gone, nearer the second", r(5000, 300, 5800, 900), both, r(3680, 300, 4480, 900)},
		{"on a monitor that is gone, below the primary", r(300, 1500, 1100, 2100), both, r(300, 440, 1100, 1040)},
		{"far away diagonally", r(-9000, -9000, -8200, -8400), both, r(0, 0, 800, 600)},
		{"overlapping two, more of the second", r(1800, -300, 2600, 300), both, r(1920, -200, 2720, 400)},
		{"larger than the area", r(5000, 5000, 8000, 6600), both, r(1920, -200, 4920, 1400)},
		{"larger than every area", r(-50, -50, 5000, 3000), []image.Rectangle{laptop}, r(0, 0, 5050, 3050)},
		{"narrow window, whole width visible", r(1880, 100, 1920, 200), []image.Rectangle{laptop}, r(1880, 100, 1920, 200)},
		{"short window, whole height visible", r(100, 1010, 900, 1040), []image.Rectangle{laptop}, r(100, 1010, 900, 1040)},
		{"short window, partly below", r(100, 1020, 900, 1050), []image.Rectangle{laptop}, r(100, 1010, 900, 1040)},
		{"empty window on screen", r(10, 10, 10, 10), []image.Rectangle{laptop}, r(10, 10, 10, 10)},
		{"empty window off screen", r(-10, 10, -10, 10), []image.Rectangle{laptop}, r(0, 10, 0, 10)},
		{"equally near: the first wins", r(900, 2000, 1100, 2100), []image.Rectangle{r(0, 0, 1000, 1000), r(1000, 0, 2000, 1000)}, r(800, 900, 1000, 1000)},
		{"equally near, second listed first", r(900, 2000, 1100, 2100), []image.Rectangle{r(1000, 0, 2000, 1000), r(0, 0, 1000, 1000)}, r(1000, 900, 1200, 1000)},
		{"huge coordinates", r(-1<<30, -1<<30, -1<<30+800, -1<<30+600), both, r(0, 0, 800, 600)},
	} {
		if got := FitOnScreens(tt.win, tt.works); got != tt.want {
			t.Errorf("%s: FitOnScreens(%v, %v) = %v, want %v", tt.name, tt.win, tt.works, got, tt.want)
		}
		if got := FitOnScreens(tt.win, tt.works); got.Size() != tt.win.Size() {
			t.Errorf("%s: the size changed: %v", tt.name, got)
		}
	}
}

// A window FitOnScreens leaves alone is left alone again: fitting is
// idempotent, so a window restored and saved again stays put.
func TestFitOnScreensIdempotent(t *testing.T) {
	works := []image.Rectangle{image.Rect(0, 0, 1920, 1040), image.Rect(1920, -200, 4480, 1240)}
	for x := -6000; x <= 8000; x += 731 {
		for y := -4000; y <= 4000; y += 523 {
			for _, size := range []image.Point{{800, 600}, {40, 20}, {3000, 1500}} {
				win := image.Rectangle{Min: image.Pt(x, y), Max: image.Pt(x, y).Add(size)}
				once := FitOnScreens(win, works)
				if twice := FitOnScreens(once, works); twice != once {
					t.Fatalf("FitOnScreens(%v) = %v, then %v", win, once, twice)
				}
			}
		}
	}
}
