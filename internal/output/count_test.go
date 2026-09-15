package output

import "testing"

func TestCountedPicksTheForm(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
	}{
		{0, "0 checks passed"},
		{1, "1 check passed"},
		{2, "2 checks passed"},
	} {
		if got := Counted(c.n, "1 check passed", "%d checks passed"); got != c.want {
			t.Errorf("Counted(%d) is %q, want %q", c.n, got, c.want)
		}
	}
}

// The second number decides the noun in an "N of M" phrase, and that is
// the one that used to read as "1 of 1 steps".
func TestCountedPairAgreesWithTheTotal(t *testing.T) {
	for _, c := range []struct {
		n, of int
		want  string
	}{
		{1, 1, "1 of 1 step did not pass"},
		{1, 3, "1 of 3 steps did not pass"},
		{2, 3, "2 of 3 steps did not pass"},
		{0, 1, "0 of 1 step did not pass"},
	} {
		got := CountedPair(c.n, c.of, "%d of %d %s did not pass", "step", "steps")
		if got != c.want {
			t.Errorf("CountedPair(%d, %d) is %q, want %q", c.n, c.of, got, c.want)
		}
	}
}
