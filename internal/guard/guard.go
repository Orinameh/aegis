package guard

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

type Guard struct {
	rules         *RuleSet
	logger        *zap.Logger
	mu            sync.RWMutex
	auditLogger   *AuditLogger
	reviewQueue   *ReviewQueue
	dryRun        bool
	interactive   bool
	autoApprove   bool
	strictMode    bool
	overrideToken string
}

// Config holds guard configuration
type Config struct {
	Rules           *RuleSet
	Logger          *zap.Logger
	DryRun          bool
	Interactive     bool
	AutoApprove     bool
	StrictMode      bool
	OverrideToken   string
	AuditLogPath    string
	ReviewQueuePath string
}

// NewGuard creates a new guard instance
func NewGuard(cfg *Config) (*Guard, error) {
	if cfg.Rules == nil {
		cfg.Rules = NewDefaultRuleSet()
	}

	auditLogger, err := NewAuditLogger(cfg.Logger, cfg.AuditLogPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create audit logger: %w", err)
	}

	var reviewQueue *ReviewQueue
	if cfg.ReviewQueuePath != "" {
		reviewQueue = NewReviewQueue(cfg.ReviewQueuePath)
	}

	return &Guard{
		rules:         cfg.Rules,
		logger:        cfg.Logger,
		auditLogger:   auditLogger,
		reviewQueue:   reviewQueue,
		dryRun:        cfg.DryRun,
		interactive:   cfg.Interactive,
		autoApprove:   cfg.AutoApprove,
		strictMode:    cfg.StrictMode,
		overrideToken: cfg.OverrideToken,
	}, nil
}

// enqueueReview records a denied action for later human review. This is
// what lets unattended (non-interactive) runs avoid blocking on stdin:
// instead of hanging, the denial is persisted and surfaced later.
func (g *Guard) enqueueReview(resource *Resource, rule *Rule, action, reason, approvedBy string) {
	if g.reviewQueue == nil {
		return
	}

	entry := &ReviewEntry{
		Timestamp:  time.Now(),
		Resource:   resource.String(),
		Type:       resource.Type,
		Rule:       rule.ID,
		Action:     action,
		Reason:     reason,
		ApprovedBy: approvedBy,
	}
	if err := g.reviewQueue.Enqueue(entry); err != nil {
		g.logger.Error("failed to enqueue review item", zap.Error(err))
	}
}

// CheckAndExecute checks permissions and executes the action if approved
func (g *Guard) CheckAndExecute(ctx context.Context, resource *Resource, action string, execute func() error) error {
	// Get permission
	permission, err := g.CheckPermission(ctx, resource, action)
	if err != nil {
		return fmt.Errorf("failed to check permission: %w", err)
	}

	// Log the permission check
	g.logger.Debug("permission check completed",
		zap.String("resource", resource.String()),
		zap.String("type", string(resource.Type)),
		zap.Bool("approved", permission.Approved),
		zap.String("reason", permission.Reason),
	)

	// If not approved, return error
	if !permission.Approved {
		return fmt.Errorf("deletion not approved: %s", permission.Reason)
	}

	// In dry run, just log and return
	if g.dryRun {
		g.logger.Info("DRY RUN: would execute action",
			zap.String("resource", resource.String()),
			zap.String("action", action),
		)
		return nil
	}

	// Require explicit confirmation before destroying anything. Protected
	// resources that were already approved via promptForApproval don't need
	// a second prompt; everything else (unprotected resources) is confirmed
	// here with full details. Skipped under --auto-approve and in
	// non-interactive mode so unattended runs never hang.
	if g.interactive && !g.autoApprove && permission.ApprovedBy != "interactive-user" {
		confirmed, err := g.confirmDestruction(resource, action)
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}
		if !confirmed {
			g.logger.Warn("deletion cancelled by user",
				zap.String("resource", resource.String()),
				zap.String("action", action),
			)
			return fmt.Errorf("deletion cancelled by user: %s", resource.String())
		}
	}

	// Execute the action
	if err := execute(); err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	// Log successful execution
	g.auditLogger.Log(&AuditEntry{
		Timestamp:  permission.Timestamp,
		Resource:   resource,
		Rule:       permission.Rule,
		Action:     action,
		Approved:   true,
		Reason:     permission.Reason,
		ApprovedBy: permission.ApprovedBy,
		DryRun:     g.dryRun,
	})

	return nil
}

// CheckPermission checks if a resource can be acted upon
func (g *Guard) CheckPermission(ctx context.Context, resource *Resource, action string) (*Permission, error) {
	// Find matching rules
	var matchingRules []*Rule
	for _, rule := range g.rules.Rules {
		if rule.Matches(resource) {
			ruleCopy := rule
			matchingRules = append(matchingRules, &ruleCopy)
		}
	}

	// If no rules match and we're not in strict mode, allow
	if len(matchingRules) == 0 && !g.strictMode {
		return &Permission{
			Approved:   true,
			Reason:     "No protection rules matched",
			ApprovedBy: "system",
			Timestamp:  time.Now(),
		}, nil
	}

	// If strict mode and no rules match, treat as warning
	if len(matchingRules) == 0 && g.strictMode {
		matchingRules = append(matchingRules, &Rule{
			ID:               "strict-mode-default",
			ResourceType:     resource.Type,
			ProtectionLevel:  LevelWarning,
			Reason:           "Strict mode enabled - all resources require approval",
			OverrideAllowed:  true,
			RequiresApproval: true,
		})
	}

	// Determine the highest protection level
	highestLevel := LevelNone
	var highestRule *Rule
	for _, rule := range matchingRules {
		switch rule.ProtectionLevel {
		case LevelCritical:
			if highestLevel != LevelCritical {
				highestLevel = LevelCritical
				highestRule = rule
			}
		case LevelStrict:
			if highestLevel != LevelCritical && highestLevel != LevelStrict {
				highestLevel = LevelStrict
				highestRule = rule
			}
		case LevelWarning:
			if highestLevel != LevelCritical && highestLevel != LevelStrict {
				highestLevel = LevelWarning
				highestRule = rule
			}
		}
	}

	// If no rule determined, allow
	if highestRule == nil {
		return &Permission{
			Approved:   true,
			Reason:     "Resource is not protected",
			ApprovedBy: "system",
			Timestamp:  time.Now(),
		}, nil
	}

	// Handle based on protection level
	return g.handleProtectionLevel(resource, highestRule, action)
}

// handleProtectionLevel handles the protection level decision
func (g *Guard) handleProtectionLevel(resource *Resource, rule *Rule, action string) (*Permission, error) {
	switch rule.ProtectionLevel {
	case LevelCritical:
		// Critical resources are never allowed to be deleted
		response := &Permission{
			Approved:   false,
			Reason:     fmt.Sprintf("Resource is critically protected: %s", rule.Reason),
			ApprovedBy: "system",
			Timestamp:  time.Now(),
			Rule:       rule,
		}

		g.auditLogger.Log(&AuditEntry{
			Timestamp:  response.Timestamp,
			Resource:   resource,
			Rule:       rule,
			Action:     action,
			Approved:   false,
			Reason:     response.Reason,
			ApprovedBy: "system",
			DryRun:     g.dryRun,
		})

		return response, nil

	case LevelStrict:
		// Check if override is allowed
		if !rule.OverrideAllowed {
			response := &Permission{
				Approved:   false,
				Reason:     fmt.Sprintf("Resource is strictly protected and override is not allowed: %s", rule.Reason),
				ApprovedBy: "system",
				Timestamp:  time.Now(),
				Rule:       rule,
			}

			g.auditLogger.Log(&AuditEntry{
				Timestamp:  response.Timestamp,
				Resource:   resource,
				Rule:       rule,
				Action:     action,
				Approved:   false,
				Reason:     response.Reason,
				ApprovedBy: "system",
				DryRun:     g.dryRun,
			})

			// In non-interactive runs this is a hard denial: queue it for
			// human review instead of hanging on stdin.
			if !g.interactive && g.reviewQueue != nil {
				g.enqueueReview(resource, rule, action, response.Reason, response.ApprovedBy)
			}

			return response, nil
		}

		// Check for override token
		if g.overrideToken != "" {
			if g.validateOverrideToken(g.overrideToken, resource) {
				return &Permission{
					Approved:   true,
					Reason:     fmt.Sprintf("Override token approved: %s", rule.Reason),
					ApprovedBy: "override-token",
					Timestamp:  time.Now(),
					Rule:       rule,
				}, nil
			}
		}

		if g.autoApprove {
			return &Permission{
				Approved:   true,
				Reason:     fmt.Sprintf("Auto-approved with warning: %s", rule.Reason),
				ApprovedBy: "auto-approve",
				Timestamp:  time.Now(),
				Rule:       rule,
			}, nil
		}

		if g.interactive {
			return g.promptForApproval(resource, rule, action)
		}

		// Default to denial
		response := &Permission{
			Approved:   false,
			Reason:     fmt.Sprintf("Resource is strictly protected and requires approval: %s", rule.Reason),
			ApprovedBy: "system",
			Timestamp:  time.Now(),
			Rule:       rule,
		}

		g.auditLogger.Log(&AuditEntry{
			Timestamp:  response.Timestamp,
			Resource:   resource,
			Rule:       rule,
			Action:     action,
			Approved:   false,
			Reason:     response.Reason,
			ApprovedBy: "system",
			DryRun:     g.dryRun,
		})

		// In non-interactive runs we can't prompt, so record it for review.
		if !g.interactive && g.reviewQueue != nil {
			g.enqueueReview(resource, rule, action, response.Reason, response.ApprovedBy)
		}

		return response, nil

	case LevelWarning:
		if g.autoApprove || g.dryRun {
			return &Permission{
				Approved:   true,
				Reason:     fmt.Sprintf("Auto-approved with warning: %s", rule.Reason),
				ApprovedBy: "auto-approve",
				Timestamp:  time.Now(),
				Rule:       rule,
			}, nil
		}

		if g.interactive {
			return g.promptForApproval(resource, rule, action)
		}

		// For warnings, default to approval in non-interactive mode
		return &Permission{
			Approved:   true,
			Reason:     fmt.Sprintf("Approved with warning: %s", rule.Reason),
			ApprovedBy: "system",
			Timestamp:  time.Now(),
			Rule:       rule,
		}, nil

	default:
		return &Permission{
			Approved:   true,
			Reason:     "Unhandled protection level, allowing deletion",
			ApprovedBy: "system",
			Timestamp:  time.Now(),
			Rule:       rule,
		}, nil
	}
}

// promptForApproval prompts the user for approval
func (g *Guard) promptForApproval(resource *Resource, rule *Rule, action string) (*Permission, error) {
	fmt.Printf("\n🛡️  PROTECTION WARNING\n")
	fmt.Printf("================================\n")
	fmt.Printf("Resource: %s\n", resource.String())
	fmt.Printf("Type: %s\n", resource.Type)
	fmt.Printf("Rule: %s\n", rule.ID)
	fmt.Printf("Protection Level: %s\n", rule.ProtectionLevel)
	fmt.Printf("Reason: %s\n", rule.Reason)
	fmt.Printf("Action: %s\n", action)
	fmt.Printf("\nDo you want to proceed? (y/N): ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(strings.ToLower(input))
	approved := input == "y" || input == "yes"

	response := &Permission{
		Approved:   approved,
		Reason:     fmt.Sprintf("User %s", map[bool]string{true: "approved", false: "rejected"}[approved]),
		ApprovedBy: "interactive-user",
		Timestamp:  time.Now(),
		Rule:       rule,
	}

	if !approved {
		response.Reason = fmt.Sprintf("User rejected: %s", rule.Reason)
	}

	g.auditLogger.Log(&AuditEntry{
		Timestamp:  response.Timestamp,
		Resource:   resource,
		Rule:       rule,
		Action:     action,
		Approved:   approved,
		Reason:     response.Reason,
		ApprovedBy: response.ApprovedBy,
		DryRun:     g.dryRun,
	})

	return response, nil
}

// confirmDestruction shows the full details of a resource about to be
// destroyed and asks for an explicit Y/N confirmation. Returns true only on
// an explicit "y" or "yes".
func (g *Guard) confirmDestruction(resource *Resource, action string) (bool, error) {
	fmt.Printf("\n⚠️  CONFIRM DESTRUCTION\n")
	fmt.Printf("================================\n")
	fmt.Printf("Action: %s\n", action)
	fmt.Printf("Type: %s\n", resource.Type)
	fmt.Printf("Resource: %s\n", resource.String())
	if len(resource.Labels) > 0 {
		fmt.Printf("Labels:\n")
		for k, v := range resource.Labels {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}
	if len(resource.Metadata) > 0 {
		fmt.Printf("Details:\n")
		for k, v := range resource.Metadata {
			fmt.Printf("  %s: %v\n", k, v)
		}
	}
	fmt.Printf("\nAre you sure you want to %s this resource? (y/N): ", action)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes", nil
}

// validateOverrideToken validates the override token
func (g *Guard) validateOverrideToken(token string, resource *Resource) bool {
	// Simple implementation - in production, validate against a secure store
	// Format: <resource-type>/<namespace>/<name>
	expectedPattern := fmt.Sprintf("%s/%s/%s", resource.Type, resource.Namespace, resource.Name)
	namespacePattern := fmt.Sprintf("%s/*/*", resource.Type)
	wildcardPattern := "*/*/*"

	return token == expectedPattern || token == namespacePattern || token == wildcardPattern
}
