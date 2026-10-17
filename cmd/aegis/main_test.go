package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/orinameh/aegis/internal/config"
	"github.com/orinameh/aegis/internal/guard"
)

// resetCleanScopeFlags clears every scope flag so tests start from the same
// state the CLI has when no scope flag was passed.
func resetCleanScopeFlags() {
	cleanDockerOnly = false
	cleanK8sOnly = false
	cleanImagesOnly = false
	cleanContainersOnly = false
	cleanVolumesOnly = false
	cleanNetworksOnly = false
	cleanBuildCacheOnly = false
	cleanPodsOnly = false
	cleanJobsOnly = false
	cleanPvcsOnly = false
}

func TestApplyCleanScope(t *testing.T) {
	defer resetCleanScopeFlags()

	newCfg := func() *config.Config {
		return &config.Config{
			EnableDockerPrune: true,
			EnableK8sPrune:    true,
			Docker: config.DockerConfig{
				PruneStopped:    true,
				PruneDangling:   true,
				PruneBuildCache: true,
				PruneNetworks:   false,
				PruneVolumes:    false,
			},
			Kubernetes: config.K8sConfig{
				DeleteFailedPods:    true,
				DeleteEvictedPods:   true,
				DeleteCompletedJobs: true,
			},
		}
	}

	tests := []struct {
		name    string
		arrange func()
		wantErr bool
		check   func(t *testing.T, cfg *config.Config)
	}{
		{
			name:    "no scope flag leaves config untouched",
			arrange: func() {},
			check: func(t *testing.T, cfg *config.Config) {
				if !cfg.EnableDockerPrune || !cfg.EnableK8sPrune {
					t.Error("expected both prune targets to remain enabled")
				}
				if !cfg.Docker.PruneStopped || !cfg.Docker.PruneDangling || !cfg.Docker.PruneBuildCache {
					t.Error("expected Docker sub-flags to remain untouched")
				}
				if !cfg.Kubernetes.DeleteFailedPods || !cfg.Kubernetes.DeleteEvictedPods {
					t.Error("expected Kubernetes sub-flags to remain untouched")
				}
			},
		},
		{
			name:    "docker-only disables Kubernetes",
			arrange: func() { cleanDockerOnly = true },
			check: func(t *testing.T, cfg *config.Config) {
				if !cfg.EnableDockerPrune || cfg.EnableK8sPrune {
					t.Errorf("expected docker-only, got docker=%v k8s=%v", cfg.EnableDockerPrune, cfg.EnableK8sPrune)
				}
			},
		},
		{
			name:    "images-only enables dangling images only",
			arrange: func() { cleanImagesOnly = true },
			check: func(t *testing.T, cfg *config.Config) {
				if !cfg.EnableDockerPrune || cfg.EnableK8sPrune {
					t.Error("expected Docker-only scope")
				}
				if !cfg.Docker.PruneDangling {
					t.Error("expected dangling image pruning to be enabled")
				}
				if cfg.Docker.PruneStopped || cfg.Docker.PruneBuildCache || cfg.Docker.PruneNetworks || cfg.Docker.PruneVolumes {
					t.Errorf("expected other Docker targets disabled, got %+v", cfg.Docker)
				}
			},
		},
		{
			name:    "build-cache-only enables build cache only",
			arrange: func() { cleanBuildCacheOnly = true },
			check: func(t *testing.T, cfg *config.Config) {
				if !cfg.Docker.PruneBuildCache {
					t.Error("expected build cache pruning to be enabled")
				}
				if cfg.Docker.PruneStopped || cfg.Docker.PruneDangling || cfg.Docker.PruneNetworks || cfg.Docker.PruneVolumes {
					t.Errorf("expected other Docker targets disabled, got %+v", cfg.Docker)
				}
			},
		},
		{
			name:    "pods-only enables Kubernetes pod deletion only",
			arrange: func() { cleanPodsOnly = true },
			check: func(t *testing.T, cfg *config.Config) {
				if cfg.EnableDockerPrune || !cfg.EnableK8sPrune {
					t.Error("expected Kubernetes-only scope")
				}
				if !cfg.Kubernetes.DeleteFailedPods || !cfg.Kubernetes.DeleteEvictedPods {
					t.Error("expected failed and evicted pod deletion enabled")
				}
				if cfg.Kubernetes.DeleteCompletedJobs || cfg.Kubernetes.DeleteSucceededJobs || cfg.Kubernetes.DeleteOrphanedPVCs {
					t.Errorf("expected other Kubernetes targets disabled, got %+v", cfg.Kubernetes)
				}
			},
		},
		{
			name:    "jobs-only enables job deletion only",
			arrange: func() { cleanJobsOnly = true },
			check: func(t *testing.T, cfg *config.Config) {
				if !cfg.Kubernetes.DeleteCompletedJobs || !cfg.Kubernetes.DeleteSucceededJobs {
					t.Error("expected completed and succeeded job deletion enabled")
				}
				if cfg.Kubernetes.DeleteFailedPods || cfg.Kubernetes.DeleteEvictedPods || cfg.Kubernetes.DeleteOrphanedPVCs {
					t.Errorf("expected other Kubernetes targets disabled, got %+v", cfg.Kubernetes)
				}
			},
		},
		{
			name:    "pvcs-only enables orphaned PVC deletion only",
			arrange: func() { cleanPvcsOnly = true },
			check: func(t *testing.T, cfg *config.Config) {
				if !cfg.Kubernetes.DeleteOrphanedPVCs {
					t.Error("expected orphaned PVC deletion enabled")
				}
				if cfg.Kubernetes.DeleteFailedPods || cfg.Kubernetes.DeleteEvictedPods ||
					cfg.Kubernetes.DeleteCompletedJobs || cfg.Kubernetes.DeleteSucceededJobs {
					t.Errorf("expected other Kubernetes targets disabled, got %+v", cfg.Kubernetes)
				}
			},
		},
		{
			name: "multiple scope flags are rejected",
			arrange: func() {
				cleanDockerOnly = true
				cleanImagesOnly = true
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetCleanScopeFlags()
			tc.arrange()

			cfg := newCfg()
			err := applyCleanScope(cfg, zap.NewNop())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error for multiple scope flags")
				}
				return
			}
			if err != nil {
				t.Fatalf("applyCleanScope returned error: %v", err)
			}
			tc.check(t, cfg)
		})
	}
}

func TestValidateListFilters(t *testing.T) {
	t.Cleanup(func() {
		listDockerTypes = nil
		listK8sKinds = nil
	})

	tests := []struct {
		name    string
		docker  []string
		k8s     []string
		wantErr bool
	}{
		{"no filters", nil, nil, false},
		{"valid docker types", []string{"containers", "images", "volumes", "networks"}, nil, false},
		{"valid k8s kinds", nil, []string{"pods", "jobs", "pvcs"}, false},
		{"mixed docker and k8s", []string{"images"}, []string{"pods"}, false},
		{"unknown docker type", []string{"imageses"}, nil, true},
		{"unknown k8s kind", nil, []string{"pod"}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			listDockerTypes = tc.docker
			listK8sKinds = tc.k8s

			err := validateListFilters()
			if tc.wantErr != (err != nil) {
				t.Fatalf("validateListFilters() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		bytes uint64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{2048, "2.0 KB"},
		{1500, "1.5 KB"},
		{5 * 1024 * 1024, "5.0 MB"},
		{3 * 1024 * 1024 * 1024, "3.0 GB"},
		{2 * 1024 * 1024 * 1024 * 1024, "2.0 TB"},
	}

	for _, tt := range tests {
		if got := humanBytes(tt.bytes); got != tt.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestSelectListTargets(t *testing.T) {
	tests := []struct {
		name                    string
		enableDocker, enableK8s bool
		dockerTypes, k8sKinds   []string
		wantDocker, wantK8s     bool
	}{
		{"both enabled, no filters", true, true, nil, nil, true, true},
		{"docker disabled, no filters", false, true, nil, nil, false, true},
		{"both disabled, no filters", false, false, nil, nil, false, false},
		{"both disabled, explicit docker", false, false, []string{"containers"}, nil, true, false},
		{"both disabled, explicit k8s", false, false, nil, []string{"pods"}, false, true},
		{"both disabled, explicit both", false, false, []string{"images"}, []string{"jobs"}, true, true},
		{"both enabled, explicit k8s only", true, true, nil, []string{"pods"}, false, true},
		{"both enabled, explicit docker only", true, true, []string{"volumes"}, nil, true, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotDocker, gotK8s := selectListTargets(tc.enableDocker, tc.enableK8s, tc.dockerTypes, tc.k8sKinds)
			if gotDocker != tc.wantDocker || gotK8s != tc.wantK8s {
				t.Errorf("selectListTargets() = (docker=%v, k8s=%v), want (docker=%v, k8s=%v)",
					gotDocker, gotK8s, tc.wantDocker, tc.wantK8s)
			}
		})
	}
}

func TestLeafCommandsRejectStrayArgs(t *testing.T) {
	for _, cmd := range []*cobra.Command{checkCmd, cleanCmd, reviewCmd, listCmd} {
		t.Run(cmd.Name(), func(t *testing.T) {
			if err := cmd.ValidateArgs(nil); err != nil {
				t.Errorf("expected no error for no args, got %v", err)
			}
			if err := cmd.ValidateArgs([]string{"stray"}); err == nil {
				t.Error("expected error for stray positional argument")
			}
		})
	}
}

func TestShortID(t *testing.T) {
	if got := shortID("0123456789abcdef"); got != "0123456789ab" {
		t.Errorf("expected 12-char prefix, got %q", got)
	}
	if got := shortID("abc"); got != "abc" {
		t.Errorf("expected short id unchanged, got %q", got)
	}
	if got := shortID("0123456789ab"); got != "0123456789ab" {
		t.Errorf("expected exactly-12-char id unchanged, got %q", got)
	}
}

func TestConvertCustomRules(t *testing.T) {
	in := []config.ProtectionRule{
		{
			ID:                "protect-production",
			ResourceType:      "pod",
			NamespacePatterns: []string{"^prod-.*", "^production$"},
			ProtectionLevel:   "critical",
			Reason:            "Production namespaces contain critical workloads",
			OverrideAllowed:   false,
			RequiresApproval:  true,
		},
	}

	out := convertCustomRules(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 converted rule, got %d", len(out))
	}

	rule := out[0]
	if rule.ID != "protect-production" {
		t.Errorf("unexpected rule ID: %q", rule.ID)
	}
	if rule.ResourceType != guard.ResourcePod {
		t.Errorf("unexpected resource type: %q", rule.ResourceType)
	}
	if rule.ProtectionLevel != guard.LevelCritical {
		t.Errorf("unexpected protection level: %q", rule.ProtectionLevel)
	}
	if len(rule.NamespacePatterns) != 2 || !strings.HasPrefix(rule.NamespacePatterns[0], "^prod") {
		t.Errorf("unexpected namespace patterns: %v", rule.NamespacePatterns)
	}
	if rule.OverrideAllowed || !rule.RequiresApproval {
		t.Errorf("expected override_allowed=false and requires_approval=true, got %v/%v", rule.OverrideAllowed, rule.RequiresApproval)
	}
}
