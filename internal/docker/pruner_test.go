package docker

import (
	"errors"
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

func TestIsProtectionDenial(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"deletion not approved", errors.New("deletion not approved for resource"), true},
		{"not allowed", errors.New("operation not allowed by protection rules"), true},
		{"critically protected", errors.New("critically protected resource"), true},
		{"strictly protected", errors.New("strictly protected resource"), true},
		{"unrelated error", errors.New("connection refused"), false},
		{"wrapped denial", errors.New("failed to delete container: deletion not approved"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isProtectionDenial(tt.err); got != tt.want {
				t.Errorf("isProtectionDenial() = %v, want %v", got, tt.want)
			}
		})
	}
}
