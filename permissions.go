package main

import (
	"os"
	"path/filepath"
	"strings"
)

// Sandbox enforces file path restrictions
type Sandbox struct {
	allowedPaths  []string
	blockedPaths  []string
	maxFileSize   int64 // bytes
	confirmDelete bool
}

// NewSandbox creates a sandbox from config defaults
func NewSandbox() *Sandbox {
	home, _ := os.UserHomeDir()
	return &Sandbox{
		allowedPaths:  []string{"*"}, // Allow ALL paths by default
		blockedPaths: []string{
			home + string(filepath.Separator) + ".ssh" + string(filepath.Separator) + "*",
			home + string(filepath.Separator) + ".gnupg" + string(filepath.Separator) + "*",
			home + string(filepath.Separator) + ".aws" + string(filepath.Separator) + "*",
			home + string(filepath.Separator) + ".config" + string(filepath.Separator) + "*",
		},
		maxFileSize:   50 * 1024 * 1024, // 50 MB
		confirmDelete: true,
	}
}

// NewSandboxFromConfig creates a sandbox from config
func NewSandboxFromConfig(cfg *Config) *Sandbox {
	sb := NewSandbox()
	if len(cfg.AllowedPaths) > 0 {
		sb.allowedPaths = cfg.AllowedPaths
	}
	if len(cfg.BlockedPaths) > 0 {
		sb.blockedPaths = cfg.BlockedPaths
	}
	if cfg.MaxFileSizeMB > 0 {
		sb.maxFileSize = int64(cfg.MaxFileSizeMB) * 1024 * 1024
	}
	sb.confirmDelete = cfg.ConfirmDeletes
	return sb
}

// IsPathAllowed checks if a path is allowed (not blocked, within allowed zones)
func (s *Sandbox) IsPathAllowed(path string) (bool, string) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, "cannot resolve path"
	}
	absPath = filepath.Clean(absPath)

	// Check blocked paths first
	for _, blocked := range s.blockedPaths {
		if matchPath(absPath, blocked) {
			return false, "path is blocked by security policy"
		}
	}

	// Check allowed paths
	for _, allowed := range s.allowedPaths {
		if matchPath(absPath, allowed) {
			return true, ""
		}
	}

	return false, "path is outside allowed directories"
}

// IsFileSizeAllowed checks file size against limit
func (s *Sandbox) IsFileSizeAllowed(size int64) (bool, string) {
	if s.maxFileSize > 0 && size > s.maxFileSize {
		return false, "file exceeds maximum allowed size"
	}
	return true, ""
}

// NeedsDeleteConfirm returns whether deletes need confirmation
func (s *Sandbox) NeedsDeleteConfirm() bool {
	return s.confirmDelete
}

// matchPath checks if a path matches a pattern
// Pattern can be "*" to match everything, end with * to match all children, or exact match
func matchPath(absPath, pattern string) bool {
	// Wildcard "*" matches everything
	if pattern == "*" {
		return true
	}

	pattern = filepath.Clean(pattern)

	if strings.HasSuffix(pattern, "*") {
		// Directory wildcard: match parent directory
		prefix := pattern[:len(pattern)-1]
		// Remove trailing separator if any
		prefix = strings.TrimRight(prefix, string(filepath.Separator))
		prefix = filepath.Clean(prefix)

		// Exact match or child of prefix
		if absPath == prefix {
			return true
		}
		if strings.HasPrefix(absPath, prefix+string(filepath.Separator)) {
			return true
		}
		return false
	}

	// Exact match
	return absPath == pattern
}

// GetSafeDirectory returns a safe working directory
func (s *Sandbox) GetSafeDirectory() string {
	home, _ := os.UserHomeDir()
	return home
}

// ValidateOperation does a full pre-flight check for any file operation
func (s *Sandbox) ValidateOperation(operation string, path string) (bool, string) {
	allowed, reason := s.IsPathAllowed(path)
	if !allowed {
		return false, reason
	}

	// For delete operations, check if confirmation is needed
	if operation == "delete_file" && s.confirmDelete {
		return true, "confirm_delete"
	}

	return true, ""
}
