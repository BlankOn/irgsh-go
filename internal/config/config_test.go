package config

import "testing"

func TestBuilderAttempts(t *testing.T) {
	for _, tc := range []struct{ configured, want int }{{-1, 3}, {0, 3}, {1, 1}, {5, 5}} {
		if got := (BuilderConfig{BuildAttempts: tc.configured}).Attempts(); got != tc.want {
			t.Errorf("configured %d: attempts = %d; want %d", tc.configured, got, tc.want)
		}
	}
}
