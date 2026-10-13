package gesture

import "time"

// Velocity estimation, after Flutter's VelocityTracker: the samples of the
// last 100 ms (at most 20), cut at the first gap longer than 40 ms (the
// pointer stopped there), fitted per axis by least squares. Flutter fits a
// quadratic and takes its slope at the latest sample; a straight line is
// enough for releasing a drag and is less sensitive to the jitter of the
// last samples.
const (
	velocityHorizon = 100 * time.Millisecond // Flutter's _horizonMilliseconds
	velocityStopGap = 40 * time.Millisecond  // Flutter's _assumePointerMoveStoppedMilliseconds
	velocitySamples = 20                     // Flutter's _historySize
	velocityMinFit  = 3                      // Flutter's _minSampleSize
)

type velocitySample struct {
	t   time.Duration
	pos Point
}

// VelocityTracker estimates the velocity of a pointer from its recent
// positions. The zero value is ready to use.
type VelocityTracker struct {
	ring [velocitySamples]velocitySample
	n    int // samples stored (up to velocitySamples)
	head int // index of the next sample
}

// Reset forgets every sample.
func (v *VelocityTracker) Reset() { v.n, v.head = 0, 0 }

// Add records the position p at time t (the backend clock).
func (v *VelocityTracker) Add(t time.Duration, p Point) {
	v.ring[v.head] = velocitySample{t, p}
	v.head = (v.head + 1) % velocitySamples
	if v.n < velocitySamples {
		v.n++
	}
}

// Velocity returns the estimated velocity, in logical pixels per second,
// at the latest sample: zero when fewer than two recent samples remain.
func (v *VelocityTracker) Velocity() Point {
	if v.n == 0 {
		return Point{}
	}
	latest := v.ring[(v.head+velocitySamples-1)%velocitySamples]
	// Collect the usable samples, newest first.
	var ts, xs, ys [velocitySamples]float64
	m := 0
	prev := latest.t
	for i := 0; i < v.n; i++ {
		s := v.ring[(v.head+velocitySamples-1-i)%velocitySamples]
		age := latest.t - s.t
		if age > velocityHorizon || prev-s.t > velocityStopGap {
			break
		}
		prev = s.t
		ts[m] = -age.Seconds()
		xs[m], ys[m] = float64(s.pos.X), float64(s.pos.Y)
		m++
	}
	if m < 2 {
		return Point{}
	}
	if m < velocityMinFit {
		// Two samples: the slope between them.
		dt := ts[0] - ts[1]
		if dt <= 0 {
			return Point{}
		}
		return Point{float32((xs[0] - xs[1]) / dt), float32((ys[0] - ys[1]) / dt)}
	}
	return Point{float32(slope(ts[:m], xs[:m])), float32(slope(ts[:m], ys[:m]))}
}

// slope returns the least-squares slope of y over t. Every product is
// rounded explicitly (no fused multiply-add), so the result is the same on
// every architecture.
func slope(t, y []float64) float64 {
	n := float64(len(t))
	var st, sy, stt, sty float64
	for i := range t {
		st += t[i]
		sy += y[i]
		stt += float64(t[i] * t[i])
		sty += float64(t[i] * y[i])
	}
	den := float64(n*stt) - float64(st*st)
	if den <= 0 {
		return 0
	}
	return (float64(n*sty) - float64(st*sy)) / den
}
