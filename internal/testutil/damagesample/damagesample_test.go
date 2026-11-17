package damagesample

import "testing"

// A subtest follows the closest parent that set a sampling, a test that
// set none (or a sibling) compares every frame, and the setting ends with
// its test.
func TestEveryFollowsTheParent(t *testing.T) {
	t.Run("outer", func(t *testing.T) {
		mu.Lock()
		every[t.Name()] = 6
		mu.Unlock()
		t.Cleanup(func() {
			mu.Lock()
			delete(every, t.Name())
			mu.Unlock()
		})
		t.Run("inner/x1.5", func(t *testing.T) {
			if k := Every(t); k != 6 {
				t.Errorf("Every = %d, want the parent's 6", k)
			}
		})
	})
	t.Run("outer_sibling", func(t *testing.T) {
		if k := Every(t); k != 0 {
			t.Errorf("Every = %d, want 0", k)
		}
	})
	if k := Every(t); k != 0 {
		t.Errorf("Every = %d after the subtest ended, want 0", k)
	}
	Short(t, 3)
	want := 0
	if testing.Short() {
		want = 3
	}
	if k := Every(t); k != want {
		t.Errorf("after Short(t, 3): Every = %d, want %d", k, want)
	}
}
