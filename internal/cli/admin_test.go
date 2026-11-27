package cli

import "testing"

func TestParsePort(t *testing.T) {
	cases := map[string]int{"": 0, " 3306 ": 3306, "22": 22, "0": 0, "-1": 0, "70000": 0, "abc": 0}
	for in, want := range cases {
		if got := parsePort(in); got != want {
			t.Errorf("parsePort(%q)=%d want %d", in, got, want)
		}
	}
}
