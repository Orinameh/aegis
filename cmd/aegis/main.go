package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"aegis/internal/banner"
	"aegis/internal/config"
	"aegis/internal/docker"
	"aegis/internal/guard"
	"aegis/internal/k8s"
	"aegis/internal/notify"
	"aegis/internal/system"
	"aegis/pkg/logger"
)

var (
	cfgFile     string
	logLevel    string
	dryRun      bool
	interactive bool
	autoApprove bool
	override    string
	noBanner    bool
	bannerStyle string
	version     = "1.0.0"
)

// rootCmd represents the base command. For backward compatibility, bare
// `aegis` acts as `aegis clean`.
var rootCmd = &cobra.Command{
	Use:   "aegis",
	Short: "🛡️ Aegis - Protected infrastructure cleaning utility",
	Long: `Aegis is a unified cloud-native cleanup utility with protection guards.
It safely prunes Docker resources and Kubernetes resources while protecting
critical components from accidental deletion.

Subcommands:
  aegis check   - read-only disk usage check that notifies when over threshold
  aegis clean   - run the destructive cleanup with protection guards
  aegis review  - list/clear actions denied by protection that await review

Running bare 'aegis' is equivalent to 'aegis clean'.`,
	Version: version,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runClean(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Read-only disk usage check that notifies when over threshold",
	Long: `Check disk usage and notify via webhook when it exceeds the configured
threshold. Performs no destructive actions, so it is safe to run unattended
(e.g. via a systemd timer or Kubernetes CronJob).

Exit codes:
  0 - check ran and disk is below the threshold (or notification is disabled)
  2 - check ran and disk usage exceeded the threshold
  1 - a real error occurred (could not read disk, could not send notification)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		exceeded, err := runCheck(cmd)
		if err != nil {
			return err
		}
		if exceeded {
			os.Exit(2)
		}
		return nil
	},
}

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Run the destructive cleanup with protection guards",
	Long: `Run Docker/Kubernetes cleanup guarded by protection rules. Interactively
prompts for strict-protected resources and requires a confirmation showing the
resource details before every deletion; use --auto-approve to skip prompts
(use with caution). In non-interactive mode (--interactive=false) no prompts
are shown and strict-protected resources are denied and queued for review
instead of hanging.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runClean(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "List or clear actions denied by protection that await review",
	Long: `Show actions that were denied by strict protection rules during
non-interactive runs and queued for human review.

  aegis review          list pending items
  aegis review --clear  empty the review queue`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReview(cmd, reviewClear)
	},
}

var reviewClear bool

func init() {
	rootCmd.AddCommand(checkCmd, cleanCmd, reviewCmd)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "config.yaml", "config file path")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "perform a dry run without making changes")
	rootCmd.PersistentFlags().BoolVar(&interactive, "interactive", true, "enable interactive mode for approvals")
	rootCmd.PersistentFlags().BoolVar(&autoApprove, "auto-approve", false, "automatically approve all deletions (use with caution)")
	rootCmd.PersistentFlags().StringVar(&override, "override", "", "override token for bypassing protection")
	rootCmd.PersistentFlags().BoolVar(&noBanner, "no-banner", false, "suppress banner display")
	rootCmd.PersistentFlags().StringVar(&bannerStyle, "banner-style", "simple", "banner style (simple, minimal, color, none)")
	reviewCmd.Flags().BoolVar(&reviewClear, "clear", false, "empty the review queue")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// printBanner renders the startup banner unless suppressed.
func printBanner() {
	if noBanner {
		return
	}
	bannerCfg := banner.NewConfig(version)
	bannerCfg.Style = banner.Style(bannerStyle)
	bannerCfg.Suppress = noBanner
	bannerCfg.Color = banner.SupportsColor()
	bannerCfg.Print()
}

// loadConfigAndLogger loads and validates config, applies CLI overrides, and
// initializes the logger. The returned cancel must be called to sync the log.
func loadConfigAndLogger(cmd *cobra.Command) (*config.Config, *zap.Logger, func(), error) {
	// The config file is only mandatory when the user explicitly passed
	// --config; otherwise we fall back to defaults.
	required := cmd.Flags().Changed("config")
	cfg, err := config.Load(cfgFile, required)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid config: %w", err)
	}

	// Override log level from flag when explicitly set
	if cmd.Flags().Changed("log-level") {
		cfg.LogLevel = logLevel
	}

	logCfg := logger.DefaultConfig()
	logCfg.Level = cfg.LogLevel
	logCfg.Encoding = cfg.LogEncoding
	logCfg.OutputPath = cfg.LogFile
	logCfg.Development = false

	log, err := logger.New(logCfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	return cfg, log, func() {
		if err := log.Sync(); err != nil {
			// Syncing stdout/stderr is not meaningful and always fails when
			// redirected or piped; only surface errors for real log files.
			if cfg.LogFile != "stdout" && cfg.LogFile != "stderr" && cfg.LogFile != "" {
				fmt.Fprintf(os.Stderr, "failed to sync logger: %v\n", err)
			}
		}
	}, nil
}

// runCheck is the read-only "check" path. Returns whether the threshold was
// exceeded. Zero destructive risk.
func runCheck(cmd *cobra.Command) (bool, error) {
	cfg, log, syncLog, err := loadConfigAndLogger(cmd)
	if err != nil {
		return false, err
	}
	defer syncLog()

	log.Info("Aegis check starting",
		zap.String("version", version),
		zap.String("config", cfgFile),
	)

	diskChecker := system.NewDiskChecker(log)
	exceeded, usage, err := diskChecker.IsThresholdExceeded("/", cfg.MaxDiskUsage)
	if err != nil {
		return false, fmt.Errorf("failed to check disk usage: %w", err)
	}

	if !exceeded {
		log.Info("disk usage below threshold",
			zap.Float64("used_percent", usage.UsedPercent),
			zap.Int("threshold_percent", cfg.MaxDiskUsage),
		)
		return false, nil
	}

	log.Warn("disk usage exceeds threshold",
		zap.Float64("used_percent", usage.UsedPercent),
		zap.Int("threshold_percent", cfg.MaxDiskUsage),
		zap.String("used", diskChecker.GetHumanReadableSize(usage.Used)),
		zap.String("total", diskChecker.GetHumanReadableSize(usage.Total)),
	)

	if !cfg.Notification.Enabled {
		log.Warn("threshold exceeded but notifications are disabled in config")
		return true, nil
	}

	pending, _ := guard.NewReviewQueue(cfg.Protection.ReviewQueuePath).CountPending()

	msg := fmt.Sprintf("🛡️ Aegis: disk usage at %.1f%% (threshold %d%%)\nUsed: %s / %s",
		usage.UsedPercent, cfg.MaxDiskUsage,
		diskChecker.GetHumanReadableSize(usage.Used),
		diskChecker.GetHumanReadableSize(usage.Total))
	if pending > 0 {
		msg += fmt.Sprintf("\n%d item(s) need your review — run `aegis review`", pending)
	} else {
		msg += "\nRun `aegis clean` to reclaim space"
	}

	notifier := notify.New(notify.Config{
		Enabled:    cfg.Notification.Enabled,
		WebhookURL: cfg.Notification.WebhookURL,
		Provider:   cfg.Notification.Provider,
		Timeout:    cfg.Notification.Timeout,
	})
	if err := notifier.Notify(msg); err != nil {
		log.Error("failed to send notification", zap.Error(err))
		return true, fmt.Errorf("failed to send notification: %w", err)
	}

	log.Info("notification sent",
		zap.String("provider", cfg.Notification.Provider),
		zap.String("webhook", cfg.Notification.WebhookURL),
	)
	return true, nil
}

// runClean is the destructive cleanup path (bare `aegis` and `aegis clean`).
func runClean(cmd *cobra.Command) error {
	printBanner()

	cfg, log, syncLog, err := loadConfigAndLogger(cmd)
	if err != nil {
		return err
	}
	defer syncLog()

	log.Info("Aegis starting",
		zap.String("version", version),
		zap.Bool("dry_run", dryRun),
		zap.String("config", cfgFile),
	)

	// Create context with cancellation and timeout
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// Setup signal handling: first signal cancels gracefully, second forces exit
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	go func() {
		<-sigChan
		log.Info("received signal, shutting down gracefully")
		cancel()
		<-sigChan
		log.Warn("second signal received, forcing exit")
		os.Exit(1)
	}()

	// Setup protection guard. Config provides defaults; explicitly-set
	// CLI flags override them.
	interactiveMode := cfg.Protection.InteractiveMode
	if cmd.Flags().Changed("interactive") {
		interactiveMode = interactive
	}

	autoApproveMode := autoApprove
	if cfg.Protection.RequireApproval && !cmd.Flags().Changed("auto-approve") {
		autoApproveMode = false
	}

	ruleSet := guard.NewDefaultRuleSet()
	if len(cfg.Protection.CustomRules) > 0 {
		customRules := convertCustomRules(cfg.Protection.CustomRules)
		ruleSet.Rules = append(ruleSet.Rules, customRules...)
	}
	if !cfg.Protection.Enabled {
		log.Warn("protection is disabled in config, resources will not be protected")
		ruleSet = &guard.RuleSet{}
	}

	guardCfg := &guard.Config{
		Logger:          log,
		DryRun:          dryRun,
		Interactive:     interactiveMode,
		AutoApprove:     autoApproveMode,
		StrictMode:      cfg.Protection.StrictMode,
		OverrideToken:   override,
		AuditLogPath:    cfg.Protection.AuditLogPath,
		ReviewQueuePath: cfg.Protection.ReviewQueuePath,
	}
	guardCfg.Rules = ruleSet

	g, err := guard.NewGuard(guardCfg)
	if err != nil {
		return fmt.Errorf("failed to create guard: %w", err)
	}

	// Initialize disk checker
	diskChecker := system.NewDiskChecker(log)

	// Check disk usage
	exceeded, usage, err := diskChecker.IsThresholdExceeded("/", cfg.MaxDiskUsage)
	if err != nil {
		return fmt.Errorf("failed to check disk usage: %w", err)
	}

	if !exceeded {
		log.Info("disk usage below threshold, skipping cleanup",
			zap.Float64("used_percent", usage.UsedPercent),
			zap.Int("threshold_percent", cfg.MaxDiskUsage),
		)
		return nil
	}

	log.Info("disk usage exceeds threshold, starting cleanup",
		zap.Float64("used_percent", usage.UsedPercent),
		zap.Int("threshold_percent", cfg.MaxDiskUsage),
		zap.String("used", diskChecker.GetHumanReadableSize(usage.Used)),
		zap.String("total", diskChecker.GetHumanReadableSize(usage.Total)),
	)

	// Execute cleanup tasks
	if err := executeCleanup(ctx, cfg, log, g); err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	log.Info("Aegis completed successfully")
	return nil
}

// runReview lists or clears the pending-review queue.
func runReview(cmd *cobra.Command, clear bool) error {
	cfg, log, syncLog, err := loadConfigAndLogger(cmd)
	if err != nil {
		return err
	}
	defer syncLog()

	queue := guard.NewReviewQueue(cfg.Protection.ReviewQueuePath)

	if clear {
		if err := queue.Clear(); err != nil {
			return fmt.Errorf("failed to clear review queue: %w", err)
		}
		log.Info("review queue cleared", zap.String("path", cfg.Protection.ReviewQueuePath))
		return nil
	}

	entries, err := queue.List()
	if err != nil {
		return fmt.Errorf("failed to read review queue: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No items pending review.")
		return nil
	}

	fmt.Printf("%d item(s) pending review (from %s):\n", len(entries), cfg.Protection.ReviewQueuePath)
	for i, e := range entries {
		fmt.Printf("  %d. [%s] %s %s via rule %q\n     %s\n",
			i+1, e.Timestamp.Format("2006-01-02 15:04:05"),
			e.Type, e.Resource, e.Rule, e.Reason)
	}
	return nil
}

func executeCleanup(ctx context.Context, cfg *config.Config, log *zap.Logger, g *guard.Guard) error {
	var errs []error

	// Execute Docker pruning
	if cfg.EnableDockerPrune {
		pruner, err := docker.NewPruner(log, g)
		if err != nil {
			log.Error("failed to create Docker pruner", zap.Error(err))
			errs = append(errs, fmt.Errorf("docker pruner creation failed: %w", err))
		} else {
			if err := pruner.Prune(ctx, &cfg.Docker); err != nil {
				log.Error("Docker pruner failed", zap.Error(err))
				errs = append(errs, fmt.Errorf("docker pruner failed: %w", err))
			}
			if err := pruner.Close(); err != nil {
				log.Error("failed to close Docker pruner", zap.Error(err))
				errs = append(errs, fmt.Errorf("docker pruner close failed: %w", err))
			}
		}
	}

	// Execute Kubernetes sweeping
	if cfg.EnableK8sPrune {
		sweeper, err := k8s.NewSweeper(log, g)
		if err != nil {
			log.Error("failed to create Kubernetes sweeper", zap.Error(err))
			errs = append(errs, fmt.Errorf("k8s sweeper creation failed: %w", err))
		} else {
			if err := sweeper.Sweep(ctx, &cfg.Kubernetes); err != nil {
				log.Error("Kubernetes sweeper failed", zap.Error(err))
				errs = append(errs, fmt.Errorf("kubernetes sweeper failed: %w", err))
			}
			if err := sweeper.Close(); err != nil {
				log.Error("failed to close Kubernetes sweeper", zap.Error(err))
				errs = append(errs, fmt.Errorf("kubernetes sweeper close failed: %w", err))
			}
		}
	}

	return errors.Join(errs...)
}

// convertCustomRules converts config rules to guard rules
func convertCustomRules(customRules []config.ProtectionRule) []guard.Rule {
	var rules []guard.Rule
	for _, r := range customRules {
		rules = append(rules, guard.Rule{
			ID:                r.ID,
			ResourceType:      guard.ResourceType(r.ResourceType),
			NamePatterns:      r.NamePatterns,
			NamespacePatterns: r.NamespacePatterns,
			Labels:            r.Labels,
			ProtectionLevel:   guard.ProtectionLevel(r.ProtectionLevel),
			Reason:            r.Reason,
			OverrideAllowed:   r.OverrideAllowed,
			RequiresApproval:  r.RequiresApproval,
		})
	}
	return rules
}
