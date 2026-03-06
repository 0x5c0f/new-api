package service

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	maxAuditSearchFiles      = 7
	auditRecordTimeTolerance = int64(15 * 60)
)

var auditWriteLock sync.Mutex

func WriteRequestAuditAsync(record dto.RequestAuditRecord) {
	if !common.AuditLogEnabled {
		return
	}
	recordCopy := record
	gopool.Go(func() {
		if err := WriteRequestAudit(recordCopy); err != nil {
			common.SysError(fmt.Sprintf("write request audit failed: %v", err))
		}
	})
}

func WriteRequestAudit(record dto.RequestAuditRecord) error {
	if !common.AuditLogEnabled {
		return nil
	}
	if record.Path == "" {
		return nil
	}
	dir := getAuditLogDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	recordDate := time.Unix(record.Time, 0)
	if record.Time <= 0 {
		recordDate = time.Now()
		record.Time = recordDate.Unix()
	}
	filePath := filepath.Join(dir, recordDate.Format("2006-01-02")+".jsonl")
	payload, err := common.Marshal(record)
	if err != nil {
		return err
	}
	auditWriteLock.Lock()
	defer auditWriteLock.Unlock()
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()
	_, err = file.Write(append(payload, '\n'))
	return err
}

func FindRequestAudit(query dto.RequestAuditQuery) (*dto.RequestAuditRecord, error) {
	files, err := getCandidateAuditFiles(query.Time)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}
	var best *dto.RequestAuditRecord
	for _, filePath := range files {
		record, findErr := findBestInFile(filePath, query)
		if findErr != nil {
			return nil, findErr
		}
		if record == nil {
			continue
		}
		if best == nil || record.Time > best.Time {
			best = record
		}
	}
	return best, nil
}

func findBestInFile(filePath string, query dto.RequestAuditQuery) (*dto.RequestAuditRecord, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var best *dto.RequestAuditRecord
	var bestScore int64 = -1
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record dto.RequestAuditRecord
		if err = common.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		score, matched := matchAuditRecord(record, query)
		if !matched {
			continue
		}
		if score > bestScore {
			recordCopy := record
			best = &recordCopy
			bestScore = score
		}
	}
	if err = scanner.Err(); err != nil {
		return nil, err
	}
	return best, nil
}

func matchAuditRecord(record dto.RequestAuditRecord, query dto.RequestAuditQuery) (int64, bool) {
	if query.RequestID != "" && record.RequestID != query.RequestID {
		return 0, false
	}
	if query.UserID > 0 && record.UserID != query.UserID {
		return 0, false
	}
	if query.TokenID > 0 && record.TokenID != query.TokenID {
		return 0, false
	}
	if query.Path != "" && record.Path != query.Path {
		return 0, false
	}
	if query.Method != "" && !strings.EqualFold(record.Method, query.Method) {
		return 0, false
	}
	score := int64(0)
	if query.RequestID != "" {
		score += 1_000_000
	}
	if query.UserID > 0 {
		score += 100_000
	}
	if query.TokenID > 0 {
		score += 50_000
	}
	if query.Path != "" {
		score += 30_000
	}
	if query.Time > 0 {
		diff := record.Time - query.Time
		if diff < 0 {
			diff = -diff
		}
		if diff > auditRecordTimeTolerance {
			return 0, false
		}
		score += (auditRecordTimeTolerance - diff)
	}
	score += record.Time
	return score, true
}

func getCandidateAuditFiles(referenceTime int64) ([]string, error) {
	dir := getAuditLogDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	fileNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".jsonl") {
			fileNames = append(fileNames, name)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(fileNames)))
	if len(fileNames) == 0 {
		return nil, nil
	}

	selected := make([]string, 0, maxAuditSearchFiles)
	if referenceTime > 0 {
		day := time.Unix(referenceTime, 0).Format("2006-01-02")
		candidates := map[string]struct{}{
			day + ".jsonl": {},
		}
		prev := time.Unix(referenceTime, 0).Add(-24 * time.Hour).Format("2006-01-02")
		next := time.Unix(referenceTime, 0).Add(24 * time.Hour).Format("2006-01-02")
		candidates[prev+".jsonl"] = struct{}{}
		candidates[next+".jsonl"] = struct{}{}
		for _, name := range fileNames {
			if _, ok := candidates[name]; ok {
				selected = append(selected, filepath.Join(dir, name))
			}
		}
	}
	for _, name := range fileNames {
		if len(selected) >= maxAuditSearchFiles {
			break
		}
		fullPath := filepath.Join(dir, name)
		alreadySelected := false
		for _, path := range selected {
			if path == fullPath {
				alreadySelected = true
				break
			}
		}
		if !alreadySelected {
			selected = append(selected, fullPath)
		}
	}
	return selected, nil
}

func getAuditLogDir() string {
	path := strings.TrimSpace(common.AuditLogPath)
	if path != "" {
		return path
	}
	if common.LogDir != nil && *common.LogDir != "" {
		return filepath.Join(*common.LogDir, "audit")
	}
	return "logs/audit"
}
