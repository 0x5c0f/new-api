package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetRequestAuditStateInitializesNilInternals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set(requestAuditContextKey, &requestAuditState{})

	state := GetRequestAuditState(c)
	require.NotNil(t, state)
	require.NotNil(t, state.Audit)
	require.NotNil(t, state.RequestPayload)
	require.NotNil(t, state.ResponsePayload)
	require.NotNil(t, state.TracePayload)
}

func TestCaptureRequestAuditRelayInfoWithMinimalStateDoesNotPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set(requestAuditContextKey, &requestAuditState{
		Audit: &model.RequestAudit{},
	})

	require.NotPanics(t, func() {
		CaptureRequestAuditRelayInfo(c, &relaycommon.RelayInfo{
			RelayFormat:     types.RelayFormatOpenAI,
			OriginModelName: "deepseek-chat",
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: "deepseek-chat",
			},
		})
	})

	state := GetRequestAuditState(c)
	require.Equal(t, "deepseek-chat", state.Audit.UpstreamModelName)
	require.Equal(t, "deepseek-chat", state.Audit.ModelName)
}

func TestCaptureRequestAuditRelayInfoWithoutChannelMetaDoesNotPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set(requestAuditContextKey, &requestAuditState{
		Audit: &model.RequestAudit{},
	})

	require.NotPanics(t, func() {
		CaptureRequestAuditRelayInfo(c, &relaycommon.RelayInfo{
			RelayFormat:     types.RelayFormatOpenAI,
			OriginModelName: "deepseek-chat",
		})
	})

	state := GetRequestAuditState(c)
	require.Equal(t, "", state.Audit.UpstreamModelName)
	require.Equal(t, "deepseek-chat", state.Audit.ModelName)
}

func TestSyncRequestAuditRelayInfoCapturesMappedModel(t *testing.T) {
	state := &requestAuditState{
		Audit: &model.RequestAudit{
			ModelName: "alias-model",
		},
		ResponsePayload: map[string]any{},
		TracePayload:    map[string]any{},
		RelayInfo: &relaycommon.RelayInfo{
			OriginModelName: "alias-model",
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: "real-upstream-model",
				IsModelMapped:     true,
			},
		},
	}

	syncRequestAuditRelayInfo(state)

	require.Equal(t, "real-upstream-model", state.Audit.UpstreamModelName)
	modelResolution, ok := state.TracePayload["model_resolution"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "alias-model", modelResolution["requested_model"])
	require.Equal(t, "real-upstream-model", modelResolution["upstream_model"])
	require.Equal(t, true, modelResolution["is_model_mapped"])
}

func TestSyncRequestAuditRelayInfoFallsBackToLinkedLogMapping(t *testing.T) {
	state := &requestAuditState{
		Audit: &model.RequestAudit{
			ModelName: "dsr",
		},
		ResponsePayload: map[string]any{},
		TracePayload:    map[string]any{},
		RelayInfo: &relaycommon.RelayInfo{
			OriginModelName: "dsr",
		},
	}

	applyRequestAuditMetadataFromLinkedLog(state, &model.Log{
		ModelName: "dsr",
	}, map[string]any{
		"upstream_model_name": "deepseek-reasoner",
		"is_model_mapped":     true,
	})
	syncRequestAuditRelayInfo(state)

	require.Equal(t, "deepseek-reasoner", state.Audit.UpstreamModelName)
	modelResolution, ok := state.TracePayload["model_resolution"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "dsr", modelResolution["requested_model"])
	require.Equal(t, "deepseek-reasoner", modelResolution["upstream_model"])
	require.Equal(t, true, modelResolution["is_model_mapped"])
}
