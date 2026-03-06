package middleware

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

var base64LikePattern = regexp.MustCompile(`^[A-Za-z0-9+/=]+$`)

type auditSanitizeConfig struct {
	maxContentLength int
	maxPayloadLength int
}

func RequestAuditCapture() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !shouldCaptureRequestAudit(c) {
			c.Next()
			return
		}
		startTime := time.Now()
		cfg := buildAuditSanitizeConfig()
		record := dto.RequestAuditRecord{
			RequestID: c.GetString(common.RequestIdKey),
			Time:      time.Now().Unix(),
			Path:      c.Request.URL.Path,
			Method:    c.Request.Method,
			ClientIP:  truncateAuditString(c.ClientIP(), cfg.maxContentLength, nil),
			UserAgent: truncateAuditString(c.Request.UserAgent(), cfg.maxContentLength, nil),
		}
		if err := captureRequestAuditBody(c, &record, cfg); err != nil {
			logger.LogWarn(c, fmt.Sprintf("capture request audit body failed: %v", err))
		}

		c.Next()

		record.UserID = c.GetInt("id")
		record.TokenID = c.GetInt("token_id")
		record.ChannelID = c.GetInt("channel_id")
		record.Group = c.GetString("group")
		if c.Writer != nil {
			record.StatusCode = c.Writer.Status()
		}
		latencyMs := int(time.Since(startTime).Milliseconds())
		if latencyMs < 0 {
			latencyMs = 0
		}
		record.LatencyMS = latencyMs
		if record.RequestID == "" {
			record.RequestID = c.GetString(common.RequestIdKey)
		}
		enforceAuditPayloadLimit(&record, cfg.maxPayloadLength)
		service.WriteRequestAuditAsync(record)
	}
}

func shouldCaptureRequestAudit(c *gin.Context) bool {
	if !common.AuditLogEnabled || c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	if c.Request.Method != http.MethodPost {
		return false
	}
	path := c.Request.URL.Path
	return path == "/v1/chat/completions" || path == "/v1/responses"
}

func captureRequestAuditBody(c *gin.Context, record *dto.RequestAuditRecord, cfg auditSanitizeConfig) error {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return err
	}
	defer func() {
		if _, seekErr := storage.Seek(0, io.SeekStart); seekErr == nil {
			c.Request.Body = io.NopCloser(storage)
		}
	}()

	bodyBytes, err := storage.Bytes()
	if err != nil {
		return err
	}
	if len(bodyBytes) == 0 {
		return nil
	}

	requestBody := make(map[string]interface{})
	if err = common.Unmarshal(bodyBytes, &requestBody); err != nil {
		record.Truncated = true
		record.Input = map[string]interface{}{
			"truncated": true,
			"preview":   truncateAuditString(string(bodyBytes), cfg.maxContentLength, nil),
		}
		return nil
	}

	sanitized := sanitizeAuditValue(requestBody, "", cfg, 0, &record.Truncated)
	sanitizedReq, ok := sanitized.(map[string]interface{})
	if !ok {
		return nil
	}

	record.Model = auditValueToString(sanitizedReq["model"], cfg.maxContentLength, &record.Truncated)
	if streamRaw, ok := sanitizedReq["stream"].(bool); ok {
		stream := streamRaw
		record.Stream = &stream
	}
	record.Messages = sanitizedReq["messages"]
	record.Metadata = sanitizedReq["metadata"]
	record.Tools = sanitizedReq["tools"]
	record.ToolChoice = sanitizedReq["tool_choice"]
	record.User = sanitizedReq["user"]
	record.Input = sanitizedReq["input"]
	return nil
}

func buildAuditSanitizeConfig() auditSanitizeConfig {
	maxContent := common.AuditLogMaxContentLength
	if maxContent <= 0 {
		maxContent = 4000
	}
	maxPayload := common.AuditLogMaxPayloadLength
	if maxPayload <= 0 {
		maxPayload = 32768
	}
	return auditSanitizeConfig{
		maxContentLength: maxContent,
		maxPayloadLength: maxPayload,
	}
}

func sanitizeAuditValue(value interface{}, key string, cfg auditSanitizeConfig, depth int, truncated *bool) interface{} {
	if value == nil {
		return nil
	}
	if depth > 16 {
		if truncated != nil {
			*truncated = true
		}
		return "[depth-truncated]"
	}
	if isAuditSensitiveKey(key) {
		if truncated != nil {
			*truncated = true
		}
		return "[redacted]"
	}
	switch val := value.(type) {
	case map[string]interface{}:
		sanitized := make(map[string]interface{}, len(val))
		for childKey, childValue := range val {
			sanitized[childKey] = sanitizeAuditValue(childValue, childKey, cfg, depth+1, truncated)
		}
		return sanitized
	case []interface{}:
		sanitized := make([]interface{}, 0, len(val))
		for _, childValue := range val {
			sanitized = append(sanitized, sanitizeAuditValue(childValue, key, cfg, depth+1, truncated))
		}
		return sanitized
	case string:
		return sanitizeAuditString(val, key, cfg.maxContentLength, truncated)
	case bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return val
	default:
		return sanitizeAuditString(fmt.Sprintf("%v", val), key, cfg.maxContentLength, truncated)
	}
}

func sanitizeAuditString(value string, key string, maxLength int, truncated *bool) string {
	if value == "" {
		return ""
	}
	keyLower := strings.ToLower(strings.TrimSpace(key))
	text := strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(text), "bearer ") {
		if truncated != nil {
			*truncated = true
		}
		return "Bearer [redacted]"
	}
	if strings.HasPrefix(strings.ToLower(text), "sk-") || strings.HasPrefix(strings.ToLower(text), "pk-") {
		if len(text) >= 12 {
			if truncated != nil {
				*truncated = true
			}
			return text[:6] + "***[redacted]***"
		}
	}
	if strings.HasPrefix(strings.ToLower(text), "data:") && strings.Contains(strings.ToLower(text), ";base64,") {
		if truncated != nil {
			*truncated = true
		}
		return fmt.Sprintf("[data-url-redacted,length=%d]", len(text))
	}
	if strings.Contains(keyLower, "image_url") && len([]rune(text)) > maxLength {
		if truncated != nil {
			*truncated = true
		}
		return fmt.Sprintf("[image-url-truncated,length=%d]", len(text))
	}
	if len(text) > maxLength && looksLikeLongBase64(text) {
		if truncated != nil {
			*truncated = true
		}
		return fmt.Sprintf("[base64-truncated,length=%d]", len(text))
	}
	return truncateAuditString(text, maxLength, truncated)
}

func looksLikeLongBase64(value string) bool {
	if len(value) < 512 {
		return false
	}
	return base64LikePattern.MatchString(value)
}

func truncateAuditString(value string, maxLength int, truncated *bool) string {
	if maxLength <= 0 {
		maxLength = 4000
	}
	runes := []rune(value)
	if len(runes) <= maxLength {
		return value
	}
	if truncated != nil {
		*truncated = true
	}
	return fmt.Sprintf("%s...(truncated,total=%d)", string(runes[:maxLength]), len(runes))
}

func auditValueToString(value interface{}, maxLength int, truncated *bool) string {
	if value == nil {
		return ""
	}
	switch val := value.(type) {
	case string:
		return truncateAuditString(val, maxLength, truncated)
	default:
		jsonBytes, err := common.Marshal(value)
		if err != nil {
			return truncateAuditString(fmt.Sprintf("%v", value), maxLength, truncated)
		}
		return truncateAuditString(string(jsonBytes), maxLength, truncated)
	}
}

func enforceAuditPayloadLimit(record *dto.RequestAuditRecord, maxLength int) {
	if maxLength <= 0 {
		maxLength = 32768
	}
	for i := 0; i < 4; i++ {
		payloadBytes, err := common.Marshal(record)
		if err != nil {
			record.Truncated = true
			return
		}
		if len([]rune(string(payloadBytes))) <= maxLength {
			return
		}
		record.Truncated = true
		switch i {
		case 0:
			record.Messages = trimAuditField(record.Messages, maxLength/4)
		case 1:
			record.Input = trimAuditField(record.Input, maxLength/4)
		case 2:
			record.Tools = trimAuditField(record.Tools, maxLength/5)
		case 3:
			record.Metadata = trimAuditField(record.Metadata, maxLength/5)
		}
	}
}

func trimAuditField(value interface{}, maxLength int) interface{} {
	if value == nil {
		return nil
	}
	jsonBytes, err := common.Marshal(value)
	if err != nil {
		return map[string]interface{}{"truncated": true}
	}
	jsonText := string(jsonBytes)
	if len([]rune(jsonText)) <= maxLength {
		return value
	}
	return map[string]interface{}{
		"truncated": true,
		"preview":   truncateAuditString(jsonText, maxLength, nil),
	}
}

func isAuditSensitiveKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return false
	}
	switch k {
	case "authorization", "api_key", "apikey", "x-api-key", "password", "secret", "token", "access_token", "refresh_token":
		return true
	}
	if strings.Contains(k, "api_key") {
		return true
	}
	if strings.HasSuffix(k, "_token") || strings.HasSuffix(k, "-token") {
		return true
	}
	if strings.HasSuffix(k, "_secret") || strings.HasSuffix(k, "-secret") {
		return true
	}
	if strings.HasSuffix(k, "_password") || strings.HasSuffix(k, "-password") {
		return true
	}
	return false
}
