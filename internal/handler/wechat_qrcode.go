package handler

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/im/wechat"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/gin-gonic/gin"
)

// qrCodeService is a singleton for WeChat QR code operations.
var qrCodeService = wechat.NewQRCodeService()

// WeChatGetQRCode generates a QR code for WeChat login.
// POST /api/v1/wechat/qrcode
//
// WeChatGetQRCode godoc
// @Summary      Get a WeChat login QR code
// @Description  Requests a WeChat QR code for scan-to-login binding (no request body)
// @Tags         IM Channels
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "QR code info (qrcode_url + qrcode identifier)"
// @Failure      500  {object}  map[string]interface{}  "Failed to generate QR code"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /wechat/qrcode [post]
func (h *IMHandler) WeChatGetQRCode(c *gin.Context) {
	ctx := c.Request.Context()

	result, err := qrCodeService.GetLoginQRCode(ctx)
	if err != nil {
		logger.Errorf(ctx, "[WeChat] Failed to generate QR code: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate QR code: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"qrcode_url": result.QRCodeURL,
			"qrcode":     result.QRCode,
		},
	})
}

// WeChatPollQRCodeStatus checks the scan status of a WeChat QR code.
// POST /api/v1/wechat/qrcode/status
//
// WeChatPollQRCodeStatus godoc
// @Summary      Poll WeChat QR code status
// @Description  Checks whether the given QR code has been scanned/confirmed/expired; returns credentials when confirmed
// @Tags         IM Channels
// @Accept       json
// @Produce      json
// @Param        request  body      map[string]interface{}  true  "{qrcode: string}"
// @Success      200      {object}  map[string]interface{}  "Scan status"
// @Failure      400      {object}  map[string]interface{}  "Invalid request parameters"
// @Failure      500      {object}  map[string]interface{}  "Server error"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /wechat/qrcode/status [post]
func (h *IMHandler) WeChatPollQRCodeStatus(c *gin.Context) {
	ctx := c.Request.Context()

	var req struct {
		QRCode string `json:"qrcode" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "qrcode is required"})
		return
	}

	result, err := qrCodeService.PollQRCodeStatus(ctx, req.QRCode)
	if err != nil {
		logger.Errorf(ctx, "[WeChat] Failed to poll QR code status: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check QR code status"})
		return
	}

	resp := gin.H{
		"status": result.Status,
	}

	// Only include credentials when login is confirmed
	if result.Status == "confirmed" {
		resp["credentials"] = gin.H{
			"bot_token":     result.BotToken,
			"ilink_bot_id":  result.ILinkBotID,
			"ilink_user_id": result.ILinkUserID,
		}
		// Include baseurl if the server returned one (may override default)
		if result.BaseURL != "" {
			resp["baseurl"] = result.BaseURL
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": resp})
}
