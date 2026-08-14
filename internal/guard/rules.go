package guard

import (
	"regexp"
	"strings"
)

// ProtectionLevel defines the severity of protection
type ProtectionLevel string

const (
	LevelNone     ProtectionLevel = "none"
	LevelWarning  ProtectionLevel = "warning"
	LevelStrict   ProtectionLevel = "strict"
	LevelCritical ProtectionLevel = "critical"
)

// ResourceType defines the type of resource
type ResourceType string

const (
	ResourcePod       ResourceType = "pod"
	ResourceJob       ResourceType = "job"
	ResourceImage     ResourceType = "image"
	ResourceContainer ResourceType = "container"
	ResourceVolume    ResourceType = "volume"
	ResourceCache     ResourceType = "cache"
	ResourceNetwork   ResourceType = "network"
	ResourcePVC       ResourceType = "pvc"
)

// Rule defines a protection rule
type Rule struct {
	ID                string            `yaml:"id"`
	ResourceType      ResourceType      `yaml:"resource_type"`
	NamePatterns      []string          `yaml:"name_patterns"`
	NamespacePatterns []string          `yaml:"namespace_patterns"`
	Labels            map[string]string `yaml:"labels"`
	ProtectionLevel   ProtectionLevel   `yaml:"protection_level"`
	Reason            string            `yaml:"reason"`
	OverrideAllowed   bool              `yaml:"override_allowed"`
	RequiresApproval  bool              `yaml:"requires_approval"`
}

// RuleSet holds all protection rules
type RuleSet struct {
	Rules []Rule `yaml:"rules"`
}

// Resource represents a resource to be checked
type Resource struct {
	Type      ResourceType
	Name      string
	Namespace string
	Labels    map[string]string
	Metadata  map[string]any
}

// NewDefaultRuleSet creates a default set of protection rules
func NewDefaultRuleSet() *RuleSet {
	return &RuleSet{
		Rules: []Rule{
			{
				ID:                "protect-system-pods",
				ResourceType:      ResourcePod,
				NamespacePatterns: []string{"^kube-system$", "^kube-public$", "^kube-node-lease$"},
				ProtectionLevel:   LevelStrict,
				Reason:            "System namespaces contain critical Kubernetes components",
				OverrideAllowed:   true,
				RequiresApproval:  true,
			},
			{
				ID:               "protect-critical-images",
				ResourceType:     ResourceImage,
				NamePatterns:     []string{".*:latest$", ".*:stable$", "^alpine.*", "^ubuntu.*", "^debian.*", ".*/production/.*"},
				ProtectionLevel:  LevelWarning,
				Reason:           "Popular base images or production images are frequently referenced",
				OverrideAllowed:  true,
				RequiresApproval: false,
			},
			{
				ID:               "protect-orchestration-containers",
				ResourceType:     ResourceContainer,
				NamePatterns:     []string{"^k8s_", "^gke-", "^eks-", "^aks-", ".*-apiserver$", ".*-controller-manager$"},
				ProtectionLevel:  LevelStrict,
				Reason:           "Container names matching common orchestration prefixes may be system components",
				OverrideAllowed:  true,
				RequiresApproval: true,
			},
			{
				ID:           "protect-managed-resources",
				ResourceType: ResourcePod,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": ".*",
				},
				ProtectionLevel:  LevelWarning,
				Reason:           "Resources managed by Kubernetes operators should be handled carefully",
				OverrideAllowed:  true,
				RequiresApproval: false,
			},
			{
				ID:               "protect-persistent-volumes",
				ResourceType:     ResourceVolume,
				NamePatterns:     []string{".*-pv-.*", ".*-volume-.*", "pvc-.*", ".*-data-.*"},
				ProtectionLevel:  LevelCritical,
				Reason:           "Persistent volumes may contain important data",
				OverrideAllowed:  false,
				RequiresApproval: true,
			},
			{
				ID:               "protect-build-cache",
				ResourceType:     ResourceCache,
				NamePatterns:     []string{".*buildkit.*", ".*docker-builder.*", ".*maven.*", ".*gradle.*"},
				ProtectionLevel:  LevelWarning,
				Reason:           "Build cache may contain important build artifacts",
				OverrideAllowed:  true,
				RequiresApproval: false,
			},
			{
				ID:               "protect-statefulsets",
				ResourceType:     ResourcePod,
				NamePatterns:     []string{".*-[0-9]+$"},
				ProtectionLevel:  LevelWarning,
				Reason:           "StatefulSet pods may have persistent data",
				OverrideAllowed:  true,
				RequiresApproval: false,
			},
		},
	}
}

// Merge merges another rule set into this one
func (rs *RuleSet) Merge(other *RuleSet) {
	rs.Rules = append(rs.Rules, other.Rules...)
}

// Matches determines if a resource matches this rule

func (r *Rule) Matches(resource *Resource) bool {
	// Check resource type
	if r.ResourceType != resource.Type {
		return false
	}

	// Check name patterns
	if len(r.NamePatterns) > 0 {
		matched := false
		for _, pattern := range r.NamePatterns {
			if regexp.MustCompile(pattern).MatchString(resource.Name) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check namespace patterns
	if len(r.NamespacePatterns) > 0 {
		matched := false
		for _, pattern := range r.NamespacePatterns {
			if regexp.MustCompile(pattern).MatchString(resource.Namespace) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check labels
	if len(r.Labels) > 0 {
		matched := true
		for key, pattern := range r.Labels {
			value, exists := resource.Labels[key]
			if !exists || !regexp.MustCompile(pattern).MatchString(value) {
				matched = false
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// String returns a string representation of the resource
func (r *Resource) String() string {
	return strings.TrimPrefix(r.Namespace+"/"+r.Name, "/")
}
