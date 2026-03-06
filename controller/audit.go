package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetAuditByRequestID(c *gin.Context) {
	requestID := strings.TrimSpace(c.Param("request_id"))
	if requestID == "" {
		common.ApiErrorMsg(c, "request_id is required")
		return
	}
	query := buildAuditQuery(c, requestID)
	record, err := service.FindRequestAudit(query)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"found":  record != nil,
			"record": record,
		},
	})
}

func MatchAuditByLogMeta(c *gin.Context) {
	query := buildAuditQuery(c, "")
	record, err := service.FindRequestAudit(query)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"found":  record != nil,
			"record": record,
		},
	})
}

func buildAuditQuery(c *gin.Context, requestID string) dto.RequestAuditQuery {
	userID := c.GetInt("id")
	role := c.GetInt("role")
	tokenID, _ := strconv.Atoi(c.Query("token_id"))
	if tokenID < 0 {
		tokenID = 0
	}
	createdAt, _ := strconv.ParseInt(c.Query("created_at"), 10, 64)
	path := strings.TrimSpace(c.Query("path"))
	method := strings.TrimSpace(c.Query("method"))
	query := dto.RequestAuditQuery{
		RequestID: requestID,
		Time:      createdAt,
		TokenID:   tokenID,
		Path:      path,
		Method:    method,
	}
	if role >= common.RoleAdminUser {
		adminUserID, _ := strconv.Atoi(c.Query("user_id"))
		if adminUserID > 0 {
			query.UserID = adminUserID
		} else {
			query.UserID = userID
		}
	} else {
		query.UserID = userID
	}
	return query
}
