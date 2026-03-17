package service

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

const (
	requestAuditContextKey = "request_audit_state"
	requestAuditTaskIDKey  = "request_audit_task_id"
	requestAuditMJIDKey    = "request_audit_mj_id"
)

type requestAuditState struct {
	Audit           *model.RequestAudit
	RequestPayload  map[string]any
	ResponsePayload map[string]any
	TracePayload    map[string]any
	RelayInfo       *relaycommon.RelayInfo
	Writer          *auditResponseWriter
}

func ensureRequestAuditState(state *requestAuditState) *requestAuditState {
	if state == nil {
		return nil
	}
	if state.Audit == nil {
		state.Audit = &model.RequestAudit{}
	}
	if state.RequestPayload == nil {
		state.RequestPayload = make(map[string]any)
	}
	if state.ResponsePayload == nil {
		state.ResponsePayload = make(map[string]any)
	}
	if state.TracePayload == nil {
		state.TracePayload = make(map[string]any)
	}
	return state
}

type auditResponseWriter struct {
	gin.ResponseWriter
	buffer       bytes.Buffer
	totalBytes   int64
	maxBytes     int
	truncated    bool
	firstWriteAt time.Time
}

func (w *auditResponseWriter) Write(data []byte) (int, error) {
	w.capture(data)
	return w.ResponseWriter.Write(data)
}

func (w *auditResponseWriter) WriteString(s string) (int, error) {
	w.capture(common.StringToByteSlice(s))
	return w.ResponseWriter.WriteString(s)
}

func (w *auditResponseWriter) capture(data []byte) {
	if len(data) == 0 {
		return
	}
	if w.firstWriteAt.IsZero() {
		w.firstWriteAt = time.Now()
	}
	w.totalBytes += int64(len(data))
	if w.maxBytes <= 0 || w.truncated {
		return
	}
	remaining := w.maxBytes - w.buffer.Len()
	if remaining <= 0 {
		w.truncated = true
		return
	}
	if len(data) > remaining {
		_, _ = w.buffer.Write(data[:remaining])
		w.truncated = true
		return
	}
	_, _ = w.buffer.Write(data)
}

func (w *auditResponseWriter) Bytes() []byte {
	return w.buffer.Bytes()
}

func ShouldEnableRequestAudit() bool {
	return operation_setting.SelfUseModeEnabled || operation_setting.DemoSiteEnabled
}

func GetRequestAuditMode() string {
	switch {
	case operation_setting.SelfUseModeEnabled:
		return "self"
	case operation_setting.DemoSiteEnabled:
		return "demo"
	default:
		return "external"
	}
}

func GetRequestAuditRetentionDays() int {
	return common.GetEnvOrDefault("REQUEST_AUDIT_RETENTION_DAYS", 30)
}

func getRequestAuditMaxTextBytes() int {
	return common.GetEnvOrDefault("REQUEST_AUDIT_MAX_TEXT_BYTES", 4<<20)
}

func BeginRequestAudit(c *gin.Context, routeGroup string) *requestAuditState {
	if c == nil || !ShouldEnableRequestAudit() {
		return nil
	}
	requestID := c.GetString(common.RequestIdKey)
	if requestID == "" {
		requestID = common.GetTimeString() + common.GetRandomString(8)
		c.Set(common.RequestIdKey, requestID)
	}
	state := &requestAuditState{
		Audit: &model.RequestAudit{
			RequestID:  requestID,
			CreatedAt:  common.GetTimestamp(),
			UpdatedAt:  common.GetTimestamp(),
			StartedAt:  time.Now().UnixMilli(),
			RouteGroup: routeGroup,
			RoutePath:  c.Request.URL.Path,
			Method:     c.Request.Method,
			Mode:       GetRequestAuditMode(),
			UserId:     c.GetInt("id"),
			Username:   c.GetString("username"),
			Group:      c.GetString("group"),
			TokenId:    c.GetInt("token_id"),
			TokenName:  c.GetString("token_name"),
		},
		RequestPayload:  make(map[string]any),
		ResponsePayload: make(map[string]any),
		TracePayload:    make(map[string]any),
	}
	state.Writer = &auditResponseWriter{
		ResponseWriter: c.Writer,
		maxBytes:       getRequestAuditMaxTextBytes(),
	}
	c.Writer = state.Writer
	c.Set(requestAuditContextKey, state)
	captureRequestAuditRequest(c, state)
	return state
}

func GetRequestAuditState(c *gin.Context) *requestAuditState {
	if c == nil {
		return nil
	}
	v, ok := c.Get(requestAuditContextKey)
	if !ok {
		return nil
	}
	state, _ := v.(*requestAuditState)
	return ensureRequestAuditState(state)
}

func CaptureRequestAuditRelayInfo(c *gin.Context, info *relaycommon.RelayInfo) {
	state := ensureRequestAuditState(GetRequestAuditState(c))
	if state == nil || info == nil {
		return
	}
	state.RelayInfo = info
	upstreamModelName := ""
	supportStreamOptions := false
	isModelMapped := false
	if info.ChannelMeta != nil {
		upstreamModelName = info.ChannelMeta.UpstreamModelName
		supportStreamOptions = info.ChannelMeta.SupportStreamOptions
		isModelMapped = info.ChannelMeta.IsModelMapped
	}
	state.Audit.RelayFormat = string(info.RelayFormat)
	state.Audit.RelayMode = info.RelayMode
	state.Audit.IsStream = info.IsStream
	state.Audit.IsPlayground = info.IsPlayground
	state.Audit.ModelName = info.OriginModelName
	state.Audit.UpstreamModelName = upstreamModelName
	state.Audit.Group = info.UsingGroup
	state.Audit.TokenId = info.TokenId
	state.Audit.UserId = info.UserId
	state.Audit.Username = common.GetStringIfEmpty(state.Audit.Username, c.GetString("username"))
	state.TracePayload["request_conversion"] = info.RequestConversionChain
	state.TracePayload["final_request_relay_format"] = info.GetFinalRequestRelayFormat()
	state.TracePayload["model_resolution"] = map[string]any{
		"requested_model": info.OriginModelName,
		"upstream_model":  upstreamModelName,
		"is_model_mapped": isModelMapped,
	}
	state.TracePayload["billing"] = map[string]any{
		"billing_source":                 info.BillingSource,
		"subscription_id":                info.SubscriptionId,
		"subscription_pre_consumed":      info.SubscriptionPreConsumed,
		"subscription_post_delta":        info.SubscriptionPostDelta,
		"subscription_plan_id":           info.SubscriptionPlanId,
		"subscription_plan_title":        info.SubscriptionPlanTitle,
		"subscription_total":             info.SubscriptionAmountTotal,
		"subscription_used_after_pre":    info.SubscriptionAmountUsedAfterPreConsume,
		"final_pre_consumed_quota":       info.FinalPreConsumedQuota,
		"reasoning_effort":               info.ReasoningEffort,
		"should_include_usage":           info.ShouldIncludeUsage,
		"support_stream_options":         supportStreamOptions,
		"use_price":                      info.PriceData.UsePrice,
		"model_ratio":                    info.PriceData.ModelRatio,
		"completion_ratio":               info.PriceData.CompletionRatio,
		"model_price":                    info.PriceData.ModelPrice,
		"group_ratio":                    info.PriceData.GroupRatioInfo.GroupRatio,
		"group_special_ratio":            info.PriceData.GroupRatioInfo.GroupSpecialRatio,
		"cache_ratio":                    info.PriceData.CacheRatio,
		"cache_creation_ratio":           info.PriceData.CacheCreationRatio,
		"cache_creation_ratio_5m":        info.PriceData.CacheCreation5mRatio,
		"cache_creation_ratio_1h":        info.PriceData.CacheCreation1hRatio,
		"other_ratios":                   info.PriceData.OtherRatios,
		"price_data_quota":               info.PriceData.Quota,
		"price_data_quota_to_preconsume": info.PriceData.QuotaToPreConsume,
	}
	if info.ChannelMeta != nil {
		state.Audit.ChannelId = info.ChannelId
		state.Audit.ChannelName = c.GetString("channel_name")
		state.Audit.ChannelType = info.ChannelType
		state.TracePayload["channel"] = map[string]any{
			"channel_id":               info.ChannelId,
			"channel_name":             c.GetString("channel_name"),
			"channel_type":             info.ChannelType,
			"channel_is_multi_key":     info.ChannelIsMultiKey,
			"channel_multi_key_index":  info.ChannelMultiKeyIndex,
			"channel_base_url":         info.ChannelBaseUrl,
			"api_type":                 info.ApiType,
			"api_version":              info.ApiVersion,
			"organization":             info.Organization,
			"upstream_model_name":      info.UpstreamModelName,
			"is_model_mapped":          info.IsModelMapped,
			"headers_override":         sanitizeObject(info.HeadersOverride),
			"param_override":           sanitizeObject(info.ParamOverride),
			"runtime_headers_override": sanitizeObject(info.RuntimeHeadersOverride),
		}
	}
}

func AppendRequestAuditAttempt(c *gin.Context, channelID int, channelName string, channelType int, retryIndex int, err error) {
	state := GetRequestAuditState(c)
	if state == nil {
		return
	}
	attempts, _ := state.TracePayload["attempts"].([]map[string]any)
	entry := map[string]any{
		"retry_index":  retryIndex,
		"channel_id":   channelID,
		"channel_name": channelName,
		"channel_type": channelType,
	}
	if err != nil {
		entry["error"] = err.Error()
	}
	attempts = append(attempts, entry)
	state.TracePayload["attempts"] = attempts
	state.Audit.RetryCount = len(attempts)
}

func SetRequestAuditTaskID(c *gin.Context, taskID string) {
	if taskID == "" {
		return
	}
	state := GetRequestAuditState(c)
	if state == nil {
		return
	}
	state.Audit.TaskID = taskID
	state.TracePayload["task_id"] = taskID
	c.Set(requestAuditTaskIDKey, taskID)
}

func SetRequestAuditRouteGroup(c *gin.Context, routeGroup string) {
	if routeGroup == "" {
		return
	}
	state := GetRequestAuditState(c)
	if state == nil {
		return
	}
	state.Audit.RouteGroup = routeGroup
}

func SetRequestAuditMJID(c *gin.Context, mjID string) {
	if mjID == "" {
		return
	}
	state := GetRequestAuditState(c)
	if state == nil {
		return
	}
	state.Audit.MjID = mjID
	state.TracePayload["mj_id"] = mjID
	c.Set(requestAuditMJIDKey, mjID)
}

func CaptureRequestAuditError(c *gin.Context, err error) {
	state := GetRequestAuditState(c)
	if state == nil || err == nil {
		return
	}
	state.TracePayload["error"] = map[string]any{
		"message": err.Error(),
	}
}

func FinishRequestAudit(c *gin.Context) error {
	state := GetRequestAuditState(c)
	if state == nil {
		return nil
	}
	state.Audit.UpdatedAt = common.GetTimestamp()
	state.Audit.FinishedAt = time.Now().UnixMilli()
	state.Audit.LatencyMs = state.Audit.FinishedAt - state.Audit.StartedAt
	state.Audit.StatusCode = c.Writer.Status()
	state.Audit.Success = c.Writer.Status() > 0 && c.Writer.Status() < http.StatusBadRequest
	if state.Writer != nil && !state.Writer.firstWriteAt.IsZero() {
		state.Audit.FirstResponseMs = state.Writer.firstWriteAt.UnixMilli() - state.Audit.StartedAt
	}
	if state.Audit.TaskID == "" {
		if taskID, ok := c.Get(requestAuditTaskIDKey); ok {
			state.Audit.TaskID = common.Interface2String(taskID)
		}
	}
	if state.Audit.MjID == "" {
		if mjID, ok := c.Get(requestAuditMJIDKey); ok {
			state.Audit.MjID = common.Interface2String(mjID)
		}
	}
	captureRequestAuditResponse(c, state)
	attachLinkedLogs(state)
	syncRequestAuditRelayInfo(state)
	state.Audit.RequestPayload = marshalAuditPart(state.RequestPayload)
	state.Audit.ResponsePayload = marshalAuditPart(state.ResponsePayload)
	state.Audit.TracePayload = marshalAuditPart(state.TracePayload)
	return model.UpsertRequestAudit(state.Audit)
}

func marshalAuditPart(v any) string {
	if v == nil {
		return ""
	}
	data, err := common.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

func attachLinkedLogs(state *requestAuditState) {
	if state == nil || state.Audit == nil || state.Audit.RequestID == "" {
		return
	}
	logs, err := model.GetRequestLogsByRequestID(state.Audit.RequestID)
	if err != nil || len(logs) == 0 {
		return
	}
	items := make([]map[string]any, 0, len(logs))
	for _, logItem := range logs {
		if logItem == nil {
			continue
		}
		otherPayload := safeParseJSON(logItem.Other)
		applyRequestAuditMetadataFromLinkedLog(state, logItem, otherPayload)
		items = append(items, map[string]any{
			"id":                logItem.Id,
			"type":              logItem.Type,
			"created_at":        logItem.CreatedAt,
			"content":           logItem.Content,
			"model_name":        logItem.ModelName,
			"quota":             logItem.Quota,
			"prompt_tokens":     logItem.PromptTokens,
			"completion_tokens": logItem.CompletionTokens,
			"use_time":          logItem.UseTime,
			"is_stream":         logItem.IsStream,
			"channel_id":        logItem.ChannelId,
			"token_id":          logItem.TokenId,
			"group":             logItem.Group,
			"other":             otherPayload,
		})
	}
	state.TracePayload["linked_logs"] = items
}

func applyRequestAuditMetadataFromLinkedLog(state *requestAuditState, logItem *model.Log, otherPayload any) {
	if state == nil || state.Audit == nil || logItem == nil {
		return
	}
	if state.Audit.ModelName == "" && logItem.ModelName != "" {
		state.Audit.ModelName = logItem.ModelName
	}
	otherMap, ok := otherPayload.(map[string]any)
	if !ok {
		return
	}
	upstreamModelName := common.Interface2String(otherMap["upstream_model_name"])
	if upstreamModelName != "" && state.Audit.UpstreamModelName == "" {
		state.Audit.UpstreamModelName = upstreamModelName
	}
	if state.Audit.ChannelName == "" {
		state.Audit.ChannelName = common.Interface2String(otherMap["use_channel_name"])
	}
}

func syncRequestAuditRelayInfo(state *requestAuditState) {
	if state == nil || state.Audit == nil || state.RelayInfo == nil {
		return
	}
	info := state.RelayInfo
	state.Audit.ModelName = common.GetStringIfEmpty(state.Audit.ModelName, info.OriginModelName)

	upstreamModelName := ""
	isModelMapped := false
	if info.ChannelMeta != nil {
		upstreamModelName = info.ChannelMeta.UpstreamModelName
		isModelMapped = info.ChannelMeta.IsModelMapped
	}
	if upstreamModelName == "" {
		if responseBody, ok := state.ResponsePayload["body_json"].(map[string]any); ok {
			if modelName, ok := responseBody["model"].(string); ok {
				upstreamModelName = modelName
			}
		}
	}
	if upstreamModelName != "" {
		state.Audit.UpstreamModelName = upstreamModelName
	}
	state.TracePayload["model_resolution"] = map[string]any{
		"requested_model": state.Audit.ModelName,
		"upstream_model":  state.Audit.UpstreamModelName,
		"is_model_mapped": isModelMapped || (state.Audit.UpstreamModelName != "" && state.Audit.UpstreamModelName != state.Audit.ModelName),
	}
}

func captureRequestAuditRequest(c *gin.Context, state *requestAuditState) {
	state.RequestPayload["headers"] = sanitizeHeaders(c.Request.Header)
	state.RequestPayload["query"] = sanitizeQuery(c.Request.URL.Query())
	state.RequestPayload["remote_ip"] = c.ClientIP()
	contentType := c.ContentType()
	state.RequestPayload["content_type"] = contentType
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodDelete || c.Request.Body == nil {
		state.RequestPayload["body_kind"] = "none"
		return
	}
	if strings.Contains(contentType, "multipart/form-data") {
		form, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			state.RequestPayload["body_kind"] = "multipart"
			state.RequestPayload["body_error"] = err.Error()
			return
		}
		state.RequestPayload["body_kind"] = "multipart"
		state.RequestPayload["body_form"] = sanitizeMultipartValues(form)
		state.RequestPayload["body_files"] = extractMultipartFiles(form)
		return
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		state.RequestPayload["body_kind"] = "unavailable"
		state.RequestPayload["body_error"] = err.Error()
		return
	}
	bodyBytes, err := storage.Bytes()
	if err != nil {
		state.RequestPayload["body_kind"] = "unavailable"
		state.RequestPayload["body_error"] = err.Error()
		return
	}
	state.RequestPayload["body_size"] = len(bodyBytes)
	if len(bodyBytes) == 0 {
		state.RequestPayload["body_kind"] = "empty"
		return
	}
	if isBinaryContentType(contentType) {
		state.RequestPayload["body_kind"] = "binary"
		state.RequestPayload["body_meta"] = map[string]any{
			"size":         len(bodyBytes),
			"content_type": contentType,
		}
		return
	}
	text := truncateAuditText(string(bodyBytes))
	state.RequestPayload["body_text"] = text
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		if values, err := url.ParseQuery(string(bodyBytes)); err == nil {
			state.RequestPayload["body_kind"] = "form"
			state.RequestPayload["body_form"] = sanitizeQuery(values)
			return
		}
	}
	var payload any
	if common.Unmarshal(bodyBytes, &payload) == nil {
		state.RequestPayload["body_kind"] = "json"
		state.RequestPayload["body_json"] = sanitizeObject(payload)
		return
	}
	state.RequestPayload["body_kind"] = "text"
}

func captureRequestAuditResponse(c *gin.Context, state *requestAuditState) {
	state.ResponsePayload["headers"] = sanitizeHeaders(c.Writer.Header())
	contentType := c.Writer.Header().Get("Content-Type")
	state.ResponsePayload["content_type"] = contentType
	if state.Writer == nil {
		state.ResponsePayload["body_kind"] = "unknown"
		return
	}
	state.ResponsePayload["body_size"] = state.Writer.totalBytes
	state.ResponsePayload["body_truncated"] = state.Writer.truncated
	bodyBytes := state.Writer.Bytes()
	if len(bodyBytes) == 0 {
		if isBinaryContentType(contentType) {
			state.ResponsePayload["body_kind"] = "binary"
			state.ResponsePayload["body_meta"] = map[string]any{
				"size":         state.Writer.totalBytes,
				"content_type": contentType,
			}
			return
		}
		state.ResponsePayload["body_kind"] = "empty"
		return
	}
	if isBinaryContentType(contentType) {
		state.ResponsePayload["body_kind"] = "binary"
		state.ResponsePayload["body_meta"] = map[string]any{
			"size":         state.Writer.totalBytes,
			"content_type": contentType,
			"captured":     len(bodyBytes),
		}
		return
	}
	rawText := truncateAuditText(string(bodyBytes))
	if strings.Contains(contentType, "text/event-stream") {
		state.ResponsePayload["body_kind"] = "event_stream"
		state.ResponsePayload["body_text"] = rawText
		if aggregated := aggregateSSEText(bodyBytes); aggregated != "" {
			state.ResponsePayload["aggregated_text"] = truncateAuditText(aggregated)
		}
		return
	}
	var payload any
	if common.Unmarshal(bodyBytes, &payload) == nil {
		state.ResponsePayload["body_kind"] = "json"
		state.ResponsePayload["body_text"] = rawText
		state.ResponsePayload["body_json"] = sanitizeObject(payload)
		return
	}
	state.ResponsePayload["body_kind"] = "text"
	state.ResponsePayload["body_text"] = rawText
}

func sanitizeHeaders(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(headers))
	for key := range headers {
		value := strings.TrimSpace(headers.Get(key))
		if value == "" {
			continue
		}
		if isSensitiveKey(key) {
			result[key] = maskSecret(value)
			continue
		}
		result[key] = truncateAuditText(value)
	}
	return result
}

func sanitizeQuery(values url.Values) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	result := make(map[string]any, len(values))
	for key, value := range values {
		if isSensitiveKey(key) {
			masked := make([]string, 0, len(value))
			for _, item := range value {
				masked = append(masked, maskSecret(item))
			}
			result[key] = masked
			continue
		}
		items := make([]string, 0, len(value))
		for _, item := range value {
			items = append(items, truncateAuditText(item))
		}
		result[key] = items
	}
	return result
}

func sanitizeMultipartValues(form *multipart.Form) map[string]any {
	if form == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(form.Value))
	for key, values := range form.Value {
		sanitized := make([]any, 0, len(values))
		for _, value := range values {
			sanitized = append(sanitized, sanitizePayloadValue(key, value))
		}
		result[key] = sanitized
	}
	return result
}

func extractMultipartFiles(form *multipart.Form) []map[string]any {
	if form == nil || len(form.File) == 0 {
		return []map[string]any{}
	}
	files := make([]map[string]any, 0)
	for field, headers := range form.File {
		for _, header := range headers {
			files = append(files, map[string]any{
				"field":        field,
				"filename":     header.Filename,
				"size":         header.Size,
				"mime_type":    header.Header.Get("Content-Type"),
				"ext":          filepath.Ext(header.Filename),
				"content_type": header.Header.Get("Content-Type"),
			})
		}
	}
	return files
}

func sanitizeObject(v any) any {
	switch value := v.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			result[key] = sanitizePayloadByKey(key, item)
		}
		return result
	case []any:
		result := make([]any, 0, len(value))
		for _, item := range value {
			result = append(result, sanitizeObject(item))
		}
		return result
	default:
		return sanitizePayloadValue("", value)
	}
}

func sanitizePayloadByKey(key string, value any) any {
	switch item := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(item))
		for subKey, subValue := range item {
			result[subKey] = sanitizePayloadByKey(subKey, subValue)
		}
		return result
	case []any:
		result := make([]any, 0, len(item))
		for _, subValue := range item {
			result = append(result, sanitizePayloadByKey(key, subValue))
		}
		return result
	default:
		return sanitizePayloadValue(key, item)
	}
}

func sanitizePayloadValue(key string, value any) any {
	switch item := value.(type) {
	case string:
		return sanitizeStringValue(key, item)
	default:
		return item
	}
}

func sanitizeStringValue(key, value string) any {
	if isSensitiveKey(key) {
		return maskSecret(value)
	}
	if isBinaryLikeField(key, value) {
		return map[string]any{
			"omitted": true,
			"reason":  "binary_or_base64",
			"size":    len(value),
			"field":   key,
		}
	}
	return truncateAuditText(value)
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "authorization", "cookie", "x-api-key", "x-goog-api-key", "mj-api-secret", "key", "api_key":
		return true
	default:
		return false
	}
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "****"
	}
	return value[:4] + "****" + value[len(value)-4:]
}

func isBinaryLikeField(key, value string) bool {
	lowerKey := strings.ToLower(key)
	if strings.HasPrefix(strings.TrimSpace(value), "data:") {
		return true
	}
	if strings.Contains(lowerKey, "base64") || strings.Contains(lowerKey, "image") || strings.Contains(lowerKey, "audio") || strings.Contains(lowerKey, "video") || strings.Contains(lowerKey, "file") {
		if len(value) > 128 {
			return true
		}
	}
	if len(value) < 256 {
		return false
	}
	trimmed := strings.Trim(value, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=\r\n")
	return len(trimmed) == 0
}

func isBinaryContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case contentType == "":
		return false
	case strings.HasPrefix(contentType, "image/"):
		return true
	case strings.HasPrefix(contentType, "audio/"):
		return true
	case strings.HasPrefix(contentType, "video/"):
		return true
	case strings.Contains(contentType, "octet-stream"):
		return true
	case strings.Contains(contentType, "application/pdf"):
		return true
	default:
		return false
	}
}

func truncateAuditText(text string) string {
	maxBytes := getRequestAuditMaxTextBytes()
	if maxBytes <= 0 || len(text) <= maxBytes {
		return text
	}
	return text[:maxBytes] + "\n...[truncated]"
}

func safeParseJSON(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var value any
	if err := common.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	return sanitizeObject(value)
}

func aggregateSSEText(body []byte) string {
	lines := strings.Split(string(body), "\n")
	var builder strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var item any
		if common.Unmarshal([]byte(payload), &item) != nil {
			continue
		}
		appendStreamText(&builder, item)
	}
	return builder.String()
}

func appendStreamText(builder *strings.Builder, payload any) {
	switch value := payload.(type) {
	case map[string]any:
		if choices, ok := value["choices"].([]any); ok {
			for _, choice := range choices {
				appendStreamText(builder, choice)
			}
		}
		if delta, ok := value["delta"].(map[string]any); ok {
			appendStreamText(builder, delta)
		}
		if message, ok := value["message"].(map[string]any); ok {
			appendStreamText(builder, message)
		}
		if output, ok := value["output"].([]any); ok {
			for _, item := range output {
				appendStreamText(builder, item)
			}
		}
		if content, ok := value["content"].([]any); ok {
			for _, item := range content {
				appendStreamText(builder, item)
			}
		}
		if text, ok := value["text"].(string); ok && text != "" {
			builder.WriteString(text)
		}
		if content, ok := value["content"].(string); ok && content != "" {
			builder.WriteString(content)
		}
		if reasoning, ok := value["reasoning_content"].(string); ok && reasoning != "" {
			builder.WriteString(reasoning)
		}
	case []any:
		for _, item := range value {
			appendStreamText(builder, item)
		}
	case string:
		if value != "" {
			builder.WriteString(value)
		}
	}
}

func StartRequestAuditCleanupTask() {
	startCleanupTaskForRetention(
		"request_audit_cleanup",
		time.Hour,
		func() error {
			days := GetRequestAuditRetentionDays()
			if days <= 0 {
				return nil
			}
			target := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
			_, err := model.DeleteOldRequestAudits(context.Background(), target, 500)
			return err
		},
	)
}

func startCleanupTaskForRetention(name string, interval time.Duration, fn func() error) {
	if !common.IsMasterNode {
		return
	}
	gopool.Go(func() {
		logger.LogInfo(context.Background(), fmt.Sprintf("%s task started: interval=%s", name, interval))
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if err := fn(); err != nil {
				logger.LogWarn(context.Background(), fmt.Sprintf("%s failed: %v", name, err))
			}
			<-ticker.C
		}
	})
}
