package spool

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/joaoreis81/submitter-smtp-daemon/internal/logger"
)

// Stage represents a message processing stage
type Stage string

const (
	StageTmp        Stage = "tmp"        // Atomic write staging
	StageIncoming   Stage = "incoming"   // Accepted messages
	StageParsed     Stage = "parsed"     // Metadata extracted
	StageFiltered   Stage = "filtered"   // Header filters applied
	StageModified   Stage = "modified"   // Message modifications applied
	StageSigned     Stage = "signed"     // DKIM signed
	StageDelivering Stage = "delivering" // In delivery queue
	StageDone       Stage = "done"       // Successfully delivered
	StageFailed     Stage = "failed"     // Permanent failures
)

// AllStages returns all processing stages in order
func AllStages() []Stage {
	return []Stage{
		StageTmp,
		StageIncoming,
		StageParsed,
		StageFiltered,
		StageModified,
		StageSigned,
		StageDelivering,
		StageDone,
		StageFailed,
	}
}

// Spool manages message spooling and stage transitions
type Spool struct {
	basePath  string
	retention map[Stage]time.Duration
	logger    *logger.Logger
}

// Config contains spool configuration
type Config struct {
	BasePath  string
	Retention map[Stage]time.Duration
}

// New creates a new Spool manager
func New(cfg Config, log *logger.Logger) (*Spool, error) {
	s := &Spool{
		basePath:  cfg.BasePath,
		retention: cfg.Retention,
		logger:    log.WithComponent("spool"),
	}

	// Create all stage directories
	if err := s.initialize(); err != nil {
		return nil, fmt.Errorf("initializing spool: %w", err)
	}

	return s, nil
}

// initialize creates all spool directories
func (s *Spool) initialize() error {
	for _, stage := range AllStages() {
		dir := s.stageDir(stage)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating %s directory: %w", stage, err)
		}
		s.logger.Infof("Initialized spool directory: %s", dir)
	}
	return nil
}

// stageDir returns the directory path for a stage
func (s *Spool) stageDir(stage Stage) string {
	return filepath.Join(s.basePath, string(stage))
}

// stagePath returns the full path for a message in a stage
func (s *Spool) stagePath(stage Stage, msgID string) string {
	return filepath.Join(s.stageDir(stage), msgID+".eml")
}

// Write writes a message atomically to the spool
func (s *Spool) Write(msgID string, data []byte) error {
	// Write to tmp first (atomic)
	tmpPath := s.stagePath(StageTmp, msgID)

	// Create a temporary file with a unique name
	tmpFile := tmpPath + ".tmp"
	f, err := os.OpenFile(tmpFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer f.Close()

	// Write data
	if _, err := f.Write(data); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("writing data: %w", err)
	}

	// Sync to disk
	if err := f.Sync(); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("syncing file: %w", err)
	}
	f.Close()

	// Atomically rename to final path
	if err := os.Rename(tmpFile, tmpPath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("renaming file: %w", err)
	}

	// Move from tmp to incoming
	if err := s.Move(msgID, StageTmp, StageIncoming); err != nil {
		return fmt.Errorf("moving to incoming: %w", err)
	}

	s.logger.WithMessageID(msgID).Debug("Message written to spool")
	return nil
}

// Read reads a message from the spool
func (s *Spool) Read(stage Stage, msgID string) ([]byte, error) {
	path := s.stagePath(stage, msgID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("message not found in %s: %s", stage, msgID)
		}
		return nil, fmt.Errorf("reading message: %w", err)
	}
	return data, nil
}

// Exists checks if a message exists in a stage
func (s *Spool) Exists(stage Stage, msgID string) bool {
	path := s.stagePath(stage, msgID)
	_, err := os.Stat(path)
	return err == nil
}

// Move moves a message from one stage to another
func (s *Spool) Move(msgID string, from, to Stage) error {
	srcPath := s.stagePath(from, msgID)
	dstPath := s.stagePath(to, msgID)

	// Check source exists
	if _, err := os.Stat(srcPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("source message not found: %s/%s", from, msgID)
		}
		return fmt.Errorf("checking source: %w", err)
	}

	// Rename (atomic within same filesystem)
	if err := os.Rename(srcPath, dstPath); err != nil {
		// If rename fails (different filesystems), fall back to copy+delete
		if linkErr, ok := err.(*os.LinkError); ok && linkErr.Err.Error() == "invalid cross-device link" {
			if err := s.copyFile(srcPath, dstPath); err != nil {
				return fmt.Errorf("copying file: %w", err)
			}
			if err := os.Remove(srcPath); err != nil {
				return fmt.Errorf("removing source: %w", err)
			}
		} else {
			return fmt.Errorf("moving file: %w", err)
		}
	}

	s.logger.WithMessageID(msgID).Debugf("Moved message: %s -> %s", from, to)
	return nil
}

// copyFile copies a file with fsync
func (s *Spool) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return dstFile.Sync()
}

// Delete deletes a message from a stage
func (s *Spool) Delete(stage Stage, msgID string) error {
	path := s.stagePath(stage, msgID)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("deleting message: %w", err)
	}
	s.logger.WithMessageID(msgID).Debugf("Deleted message from %s", stage)
	return nil
}

// List lists all messages in a stage
func (s *Spool) List(stage Stage) ([]string, error) {
	dir := s.stageDir(stage)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory: %w", err)
	}

	var messages []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Remove .eml extension
		name := entry.Name()
		if len(name) > 4 && name[len(name)-4:] == ".eml" {
			messages = append(messages, name[:len(name)-4])
		}
	}

	return messages, nil
}

// Count returns the number of messages in a stage
func (s *Spool) Count(stage Stage) (int, error) {
	messages, err := s.List(stage)
	if err != nil {
		return 0, err
	}
	return len(messages), nil
}

// Cleanup removes old messages based on retention policy
func (s *Spool) Cleanup() error {
	now := time.Now()

	for stage, retention := range s.retention {
		if retention <= 0 {
			continue // No retention policy for this stage
		}

		dir := s.stageDir(stage)
		entries, err := os.ReadDir(dir)
		if err != nil {
			s.logger.WithError(err).Errorf("Reading directory for cleanup: %s", dir)
			continue
		}

		deleted := 0
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}

			// Check if file is older than retention period
			if now.Sub(info.ModTime()) > retention {
				path := filepath.Join(dir, entry.Name())
				if err := os.Remove(path); err != nil {
					s.logger.WithError(err).Warnf("Failed to delete old file: %s", path)
					continue
				}
				deleted++
			}
		}

		if deleted > 0 {
			s.logger.Infof("Cleaned up %d old messages from %s", deleted, stage)
		}
	}

	return nil
}

// StartCleanupWorker starts a background worker for cleanup
func (s *Spool) StartCleanupWorker(interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.logger.Infof("Started cleanup worker (interval: %s)", interval)

	for {
		select {
		case <-ticker.C:
			if err := s.Cleanup(); err != nil {
				s.logger.WithError(err).Error("Cleanup failed")
			}
		case <-stop:
			s.logger.Info("Cleanup worker stopped")
			return
		}
	}
}

// GetPath returns the full file path for a message in a stage
func (s *Spool) GetPath(stage Stage, msgID string) string {
	return s.stagePath(stage, msgID)
}
