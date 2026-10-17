package docker

import (
	"testing"
)

func TestHumanizeBytes(t *testing.T) {
	tests := []struct {
		name  string
		bytes uint64
		want  string
	}{
		{"zero", 0, "0 B"},
		{"bytes", 512, "512 B"},
		{"kib", 2048, "2.0 KB"},
		{"mib", 5 * 1024 * 1024, "5.0 MB"},
		{"gib", 3 * 1024 * 1024 * 1024, "3.0 GB"},
		{"fractional", 1500, "1.5 KB"},
		{"huge", 2 * 1024 * 1024 * 1024 * 1024, "2.0 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := humanizeBytes(tt.bytes); got != tt.want {
				t.Errorf("humanizeBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}
