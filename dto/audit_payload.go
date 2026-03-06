package dto

type RequestAuditRecord struct {
	RequestID string `json:"request_id"`
	Time      int64  `json:"time"`
	Path      string `json:"path"`
	Method    string `json:"method"`

	UserID    int    `json:"user_id"`
	TokenID   int    `json:"token_id"`
	ChannelID int    `json:"channel_id,omitempty"`
	Group     string `json:"group,omitempty"`

	Model      string      `json:"model,omitempty"`
	Stream     *bool       `json:"stream,omitempty"`
	Messages   interface{} `json:"messages,omitempty"`
	Metadata   interface{} `json:"metadata,omitempty"`
	Tools      interface{} `json:"tools,omitempty"`
	ToolChoice interface{} `json:"tool_choice,omitempty"`
	User       interface{} `json:"user,omitempty"`
	Input      interface{} `json:"input,omitempty"`

	StatusCode int    `json:"status_code"`
	LatencyMS  int    `json:"latency_ms"`
	ClientIP   string `json:"client_ip,omitempty"`
	UserAgent  string `json:"user_agent,omitempty"`

	Truncated bool `json:"truncated,omitempty"`
}

type RequestAuditQuery struct {
	RequestID string
	Time      int64
	UserID    int
	TokenID   int
	Path      string
	Method    string
}
