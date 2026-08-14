package guard

import (
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
)

// Permission represents a permission decision
type Permission struct {
	Approved   bool
	Reason     string
	ApprovedBy string
	Timestamp  time.Time
	Rule       *Rule
}

// AuditEntry represents an audit log entry
type AuditEntry struct {
	Timestamp  time.Time
	Resource   *Resource
	Rule       *Rule
	Action     string
	Approved   bool
	Reason     string
	ApprovedBy string
	DryRun     bool
}

// AuditLogger handles audit logging
type AuditLogger struct {
	logger   *zap.Logger
	filePath string
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(logger *zap.Logger, filePath string) (*AuditLogger, error) {
	// Ensure directory exists
	if filePath != "" {
		if err := os.MkdirAll(getDir(filePath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create audit log directory: %w", err)
		}
	}

	return &AuditLogger{
		logger:   logger,
		filePath: filePath,
	}, nil
}

// Log logs an audit entry
func (a *AuditLogger) Log(entry *AuditEntry) {
	if entry.Rule == nil {
		// No rule matched, log as such
		a.logger.Info("audit: no protection rule matched",
			zap.String("resource", entry.Resource.String()),
			zap.String("type", string(entry.Resource.Type)),
			zap.String("action", entry.Action),
			zap.Bool("approved", entry.Approved),
			zap.String("reason", entry.Reason),
		)
	} else {
		a.logger.Info("audit: protection rule triggered",
			zap.String("rule_id", entry.Rule.ID),
			zap.String("resource", entry.Resource.String()),
			zap.String("type", string(entry.Resource.Type)),
			zap.String("action", entry.Action),
			zap.Bool("approved", entry.Approved),
			zap.String("reason", entry.Reason),
			zap.String("approved_by", entry.ApprovedBy),
			zap.Bool("dry_run", entry.DryRun),
		)
	}

	// Write to file if configured
	if a.filePath != "" {
		a.writeToFile(entry)
	}
}

// writeToFile writes an audit entry to file
func (a *AuditLogger) writeToFile(entry *AuditEntry) {
	f, err := os.OpenFile(a.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		a.logger.Error("failed to open audit log file", zap.Error(err))
		return
	}
	defer func() { _ = f.Close() }()

	ruleID := "none"
	if entry.Rule != nil {
		ruleID = entry.Rule.ID
	}

	entryStr := fmt.Sprintf("[%s] Resource: %s Type: %s Rule: %s Action: %s Approved: %v Reason: %s ApprovedBy: %s DryRun: %v\n",
		entry.Timestamp.Format(time.RFC3339),
		entry.Resource.String(),
		entry.Resource.Type,
		ruleID,
		entry.Action,
		entry.Approved,
		entry.Reason,
		entry.ApprovedBy,
		entry.DryRun,
	)

	if _, err := f.WriteString(entryStr); err != nil {
		a.logger.Error("failed to write to audit log file", zap.Error(err))
	}
}

// getDir returns the directory part of a file path
func getDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}
