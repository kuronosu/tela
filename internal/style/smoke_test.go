package style

import "testing"

// counterClasses are the classes of the ARCHITECTURE section 1 example.
const counterClasses = "flex flex-col items-center gap-6 p-10 rounded-3xl bg-linear-to-br from-indigo-500 to-fuchsia-600 shadow-2xl text-5xl font-bold text-white tracking-tight px-6 py-3 rounded-xl bg-white/15 backdrop-blur hover:bg-white/25 active:scale-95 transition"

func TestCounterClassesParse(t *testing.T) {
	c := Compile(counterClasses)
	if c.Err != nil {
		t.Fatalf("Compile: %v", c.Err)
	}
	if len(c.Rules) != 22 {
		t.Errorf("len(Rules) = %d, want 22", len(c.Rules))
	}
}
