package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

// Config represents the entire configuration structure
type Config struct {
	EnableDockerPrune bool             `yaml:"enable_docker_prune" mapstructure:"enable_docker_prune"`
	EnableK8sPrune    bool             `yaml:"enable_k8s_prune" mapstructure:"enable_k8s_prune"`
	MaxDiskUsage      int              `yaml:"max_disk_usage_percent" mapstructure:"max_disk_usage_percent"`
	LogLevel          string           `yaml:"log_level" mapstructure:"log_level"`
	LogEncoding       string           `yaml:"log_encoding" mapstructure:"log_encoding"`
	LogFile           string           `yaml:"log_file" mapstructure:"log_file"`
	Timeout           time.Duration    `yaml:"timeout" mapstructure:"timeout"`
	Docker            DockerConfig     `yaml:"docker" mapstructure:"docker"`
	Kubernetes        K8sConfig        `yaml:"kubernetes" mapstructure:"kubernetes"`
	Protection        ProtectionConfig `yaml:"protection" mapstructure:"protection"`
	Notification      Notification     `yaml:"notification" mapstructure:"notification"`
}

// Notification holds webhook notification settings used by `aegis check`.
type Notification struct {
	Enabled    bool          `yaml:"enabled" mapstructure:"enabled"`
	WebhookURL string        `yaml:"webhook_url" mapstructure:"webhook_url"`
	Provider   string        `yaml:"provider" mapstructure:"provider"`
	Timeout    time.Duration `yaml:"timeout" mapstructure:"timeout"`
}

// DockerConfig holds Docker-specific settings
type DockerConfig struct {
	PruneBuildCache bool `yaml:"prune_build_cache" mapstructure:"prune_build_cache"`
	PruneStopped    bool `yaml:"prune_stopped_containers" mapstructure:"prune_stopped_containers"`
	PruneDangling   bool `yaml:"prune_dangling_images" mapstructure:"prune_dangling_images"`
	PruneNetworks   bool `yaml:"prune_networks" mapstructure:"prune_networks"`
	PruneVolumes    bool `yaml:"prune_volumes" mapstructure:"prune_volumes"`
}

// K8sConfig holds Kubernetes-specific settings
type K8sConfig struct {
	DeleteFailedPods    bool     `yaml:"delete_failed_pods" mapstructure:"delete_failed_pods"`
	DeleteEvictedPods   bool     `yaml:"delete_evicted_pods" mapstructure:"delete_evicted_pods"`
	DeleteCompletedJobs bool     `yaml:"delete_completed_jobs" mapstructure:"delete_completed_jobs"`
	DeleteSucceededJobs bool     `yaml:"delete_succeeded_jobs" mapstructure:"delete_succeeded_jobs"`
	DeleteOrphanedPVCs  bool     `yaml:"delete_orphaned_pvcs" mapstructure:"delete_orphaned_pvcs"`
	IncludeNamespaces   []string `yaml:"include_namespaces" mapstructure:"include_namespaces"`
	ExcludeNamespaces   []string `yaml:"exclude_namespaces" mapstructure:"exclude_namespaces"`
	JobRetentionDays    int      `yaml:"job_retention_days" mapstructure:"job_retention_days"`
}

// ProtectionConfig holds protection guard settings
type ProtectionConfig struct {
	Enabled         bool             `yaml:"enabled" mapstructure:"enabled"`
	StrictMode      bool             `yaml:"strict_mode" mapstructure:"strict_mode"`
	RequireApproval bool             `yaml:"require_approval" mapstructure:"require_approval"`
	InteractiveMode bool             `yaml:"interactive_mode" mapstructure:"interactive_mode"`
	AuditLogPath    string           `yaml:"audit_log_path" mapstructure:"audit_log_path"`
	ReviewQueuePath string           `yaml:"review_queue_path" mapstructure:"review_queue_path"`
	CustomRules     []ProtectionRule `yaml:"custom_rules" mapstructure:"custom_rules"`
}

// ProtectionRule defines a protection rule
type ProtectionRule struct {
	ID                string            `yaml:"id" mapstructure:"id"`
	ResourceType      string            `yaml:"resource_type" mapstructure:"resource_type"`
	NamePatterns      []string          `yaml:"name_patterns" mapstructure:"name_patterns"`
	NamespacePatterns []string          `yaml:"namespace_patterns" mapstructure:"namespace_patterns"`
	Labels            map[string]string `yaml:"labels" mapstructure:"labels"`
	ProtectionLevel   string            `yaml:"protection_level" mapstructure:"protection_level"`
	Reason            string            `yaml:"reason" mapstructure:"reason"`
	OverrideAllowed   bool              `yaml:"override_allowed" mapstructure:"override_allowed"`
	RequiresApproval  bool              `yaml:"requires_approval" mapstructure:"requires_approval"`
}

// Load loads configuration from file and environment.
//
// required controls whether a missing config file is fatal: when the path
// was explicitly requested (e.g. via --config) it must exist; when it is
// just a default path, Load falls back to built-in defaults.
func Load(path string, required bool) (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("enable_docker_prune", true)
	v.SetDefault("enable_k8s_prune", true)
	v.SetDefault("max_disk_usage_percent", 85)
	v.SetDefault("log_level", "info")
	v.SetDefault("log_encoding", "json")
	v.SetDefault("log_file", "stdout")
	v.SetDefault("timeout", "5m")

	// Docker defaults
	v.SetDefault("docker.prune_stopped_containers", true)
	v.SetDefault("docker.prune_dangling_images", true)
	v.SetDefault("docker.prune_build_cache", true)
	v.SetDefault("docker.prune_networks", false)
	v.SetDefault("docker.prune_volumes", false)

	// Kubernetes defaults
	v.SetDefault("kubernetes.delete_failed_pods", true)
	v.SetDefault("kubernetes.delete_evicted_pods", true)
	v.SetDefault("kubernetes.delete_completed_jobs", true)
	v.SetDefault("kubernetes.delete_succeeded_jobs", false)
	v.SetDefault("kubernetes.delete_orphaned_pvcs", false)
	v.SetDefault("kubernetes.include_namespaces", []string{})
	v.SetDefault("kubernetes.exclude_namespaces", []string{"kube-system", "kube-public", "kube-node-lease"})
	v.SetDefault("kubernetes.job_retention_days", 7)

	// Protection defaults
	v.SetDefault("protection.enabled", true)
	v.SetDefault("protection.strict_mode", false)
	v.SetDefault("protection.require_approval", true)
	v.SetDefault("protection.interactive_mode", true)
	v.SetDefault("protection.audit_log_path", "logs/aegis-audit.log")
	v.SetDefault("protection.review_queue_path", "logs/aegis-review.json")

	// Notification defaults
	v.SetDefault("notification.enabled", false)
	v.SetDefault("notification.webhook_url", "")
	v.SetDefault("notification.provider", "generic")
	v.SetDefault("notification.timeout", "10s")

	// Read config file
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				if required {
					return nil, fmt.Errorf("config file not found: %s", path)
				}
			} else {
				return nil, fmt.Errorf("failed to access config file: %w", err)
			}
		} else {
			v.SetConfigFile(path)
			if err := v.ReadInConfig(); err != nil {
				return nil, fmt.Errorf("failed to read config file: %w", err)
			}
		}
	}

	// Environment variables override
	v.SetEnvPrefix("AEGIS")
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// LoadYAML loads configuration from YAML file directly
func LoadYAML(path string) (*Config, error) {
	data, err := os.ReadFile(path)

	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// Validate validates the configuration
func (cfg *Config) Validate() error {
	if cfg.MaxDiskUsage < 1 || cfg.MaxDiskUsage > 100 {
		return fmt.Errorf("max_disk_usage_percent must be between 1 and 100")
	}

	if cfg.Timeout < time.Second {
		return fmt.Errorf("timeout must be at least 1 second")
	}

	if cfg.Notification.Enabled && cfg.Notification.WebhookURL == "" {
		return fmt.Errorf("notification.enabled requires notification.webhook_url to be set")
	}

	switch cfg.Notification.Provider {
	case "generic", "slack", "discord", "ntfy", "":
	default:
		return fmt.Errorf("unknown notification provider %q (want generic, slack, discord, or ntfy)", cfg.Notification.Provider)
	}

	return nil

}
