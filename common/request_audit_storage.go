package common

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// requestAuditDirName is the subdirectory name under the root log dir.
const requestAuditDirName = "request-audit"

// GetRequestAuditDir returns the root directory for request audit payload files.
// Path: {REQUEST_AUDIT_FILE_DIR || *LogDir}/request-audit
func GetRequestAuditDir() string {
	root := os.Getenv("REQUEST_AUDIT_FILE_DIR")
	if root == "" {
		root = *LogDir
	}
	return filepath.Join(root, requestAuditDirName)
}

// WriteRequestAuditPayload writes the combined audit payload to disk.
// Returns a relative path (relative to GetRequestAuditDir()) and the file size.
// Path format: {YYYY-MM-DD}/{requestID}/payload.json
func WriteRequestAuditPayload(requestID string, payload map[string]any) (relativePath string, size int64, err error) {
	if requestID == "" || payload == nil {
		return "", 0, fmt.Errorf("request_audits: empty requestID or nil payload")
	}

	data, err := Marshal(payload)
	if err != nil {
		return "", 0, fmt.Errorf("request_audits: marshal payload: %w", err)
	}

	now := time.Now()
	dateDir := now.Format("2006-01-02")
	fileDir := filepath.Join(GetRequestAuditDir(), dateDir, requestID)
	if err := os.MkdirAll(fileDir, 0755); err != nil {
		return "", 0, fmt.Errorf("request_audits: mkdir: %w", err)
	}

	filePath := filepath.Join(fileDir, "payload.json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", 0, fmt.Errorf("request_audits: write file: %w", err)
	}

	relativePath = filepath.Join(dateDir, requestID, "payload.json")
	return relativePath, int64(len(data)), nil
}

// ReadRequestAuditPayload reads a payload file by its relative path.
func ReadRequestAuditPayload(relativePath string) ([]byte, error) {
	if relativePath == "" {
		return nil, fmt.Errorf("request_audits: empty relative path")
	}
	fullPath := filepath.Join(GetRequestAuditDir(), relativePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("request_audits: read file %s: %w", relativePath, err)
	}
	return data, nil
}

// DeleteRequestAuditFilesByDate removes all audit files for a given date directory.
// dateDir should be in "YYYY-MM-DD" format.
func DeleteRequestAuditFilesByDate(dateDir string) error {
	if dateDir == "" {
		return nil
	}
	fullPath := filepath.Join(GetRequestAuditDir(), dateDir)
	if err := os.RemoveAll(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("request_audits: remove date dir %s: %w", dateDir, err)
	}
	return nil
}

// DeleteRequestAuditFile removes a single audit payload file by relative path.
func DeleteRequestAuditFile(relativePath string) error {
	if relativePath == "" {
		return nil
	}
	fullPath := filepath.Join(GetRequestAuditDir(), relativePath)
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("request_audits: remove file %s: %w", relativePath, err)
	}
	return nil
}
