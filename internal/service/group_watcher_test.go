package service

import (
	"testing"
	"time"
)

func TestNextOccurrence(t *testing.T) {
	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name         string
		hour, minute int
		want         time.Duration
	}{
		{name: "later today", hour: 11, minute: 0, want: 1 * time.Hour},
		{name: "already passed today", hour: 9, minute: 0, want: 23 * time.Hour},
		{name: "exactly now", hour: 10, minute: 0, want: 24 * time.Hour},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextOccurrence(now, tc.hour, tc.minute); got != tc.want {
				t.Errorf("nextOccurrence(%02d:%02d) = %v, want %v", tc.hour, tc.minute, got, tc.want)
			}
		})
	}
}
