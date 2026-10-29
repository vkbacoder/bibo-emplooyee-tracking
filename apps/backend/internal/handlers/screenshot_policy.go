package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"ctracking/backend/internal/auth"
	"ctracking/backend/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ScreenshotPolicyHandler serves desktop client policy & capture request endpoints,
// and manager policy administration endpoints.
type ScreenshotPolicyHandler struct {
	store *store.Store
}

// NewScreenshotPolicyHandler wires the screenshot policy handler.
func NewScreenshotPolicyHandler(s *store.Store) *ScreenshotPolicyHandler {
	return &ScreenshotPolicyHandler{store: s}
}

// GetScreenshotPolicy serves GET /v1/screenshot-policy?employee_id={employee_id}
// For employee clients, derives the identity from JWT and verifies caller cannot fetch
// another employee's policy.
func (h *ScreenshotPolicyHandler) GetScreenshotPolicy(c *gin.Context) {
	callerID, ok := auth.UserID(c)
	if !ok {
		unauthorized(c, "unauthenticated")
		return
	}

	empIDQuery := c.Query("employee_id")
	targetEmployeeID := callerID

	if empIDQuery != "" {
		if _, err := uuid.Parse(empIDQuery); err != nil {
			badRequest(c, "employee_id must be a valid uuid")
			return
		}

		if empIDQuery != callerID {
			// Check if caller is an owner of a business that empIDQuery belongs to
			owns, err := h.store.OwnsEmployee(c.Request.Context(), callerID, empIDQuery)
			if err != nil {
				serverError(c, err)
				return
			}
			if !owns {
				c.JSON(http.StatusForbidden, gin.H{"error": "cannot view another employee's policy"})
				return
			}
			targetEmployeeID = empIDQuery
		}
	}

	bizID, err := h.store.ResolveBusinessForUser(c.Request.Context(), targetEmployeeID, nil)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusForbidden, gin.H{"error": "user belongs to no business"})
		return
	}
	if errors.Is(err, store.ErrAmbiguousBusiness) {
		badRequest(c, "user belongs to multiple businesses: specify business_id")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}

	effective, err := h.store.GetEffectiveScreenshotPolicy(c.Request.Context(), bizID, targetEmployeeID)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "business not found"})
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}

	// Exact format expected by the desktop client contract (integer unix timestamp)
	c.JSON(http.StatusOK, gin.H{
		"capture_enabled":            effective.CaptureEnabled,
		"continuous_capture_enabled": effective.ContinuousCaptureEnabled,
		"capture_interval_minutes":   effective.CaptureIntervalMinutes,
		"manual_capture_enabled":     effective.ManualCaptureEnabled,
		"policy_version":             effective.PolicyVersion,
		"policy_updated_at":          effective.PolicyUpdatedAt.Unix(),
		"employee_id":                effective.EmployeeID,
	})
}

type ackPolicyReq struct {
	EmployeeID     string  `json:"employee_id"`
	PolicyVersion  int     `json:"policy_version"`
	DeviceID       *string `json:"device_id"`
	AcknowledgedAt *int64  `json:"acknowledged_at"`
}

// AckScreenshotPolicy serves POST /v1/screenshot-policy/ack
func (h *ScreenshotPolicyHandler) AckScreenshotPolicy(c *gin.Context) {
	callerID, ok := auth.UserID(c)
	if !ok {
		unauthorized(c, "unauthenticated")
		return
	}

	var req ackPolicyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid body")
		return
	}

	targetEmployeeID := req.EmployeeID
	if targetEmployeeID == "" {
		targetEmployeeID = callerID
	} else if _, err := uuid.Parse(targetEmployeeID); err != nil {
		badRequest(c, "employee_id must be a valid uuid")
		return
	}

	if targetEmployeeID != callerID {
		owns, err := h.store.OwnsEmployee(c.Request.Context(), callerID, targetEmployeeID)
		if err != nil {
			serverError(c, err)
			return
		}
		if !owns {
			c.JSON(http.StatusForbidden, gin.H{"error": "cannot acknowledge policy for another employee"})
			return
		}
	}

	bizID, err := h.store.ResolveBusinessForUser(c.Request.Context(), targetEmployeeID, nil)
	if err != nil {
		serverError(c, err)
		return
	}

	if req.DeviceID != nil && *req.DeviceID != "" {
		if _, err := uuid.Parse(*req.DeviceID); err != nil {
			badRequest(c, "device_id must be a valid uuid")
			return
		}
	}

	if err := h.store.AckScreenshotPolicy(c.Request.Context(), bizID, targetEmployeeID, req.PolicyVersion, req.DeviceID); err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":         "acknowledged",
		"policy_version": req.PolicyVersion,
	})
}

// GetPendingScreenshotRequests serves GET /v1/screenshot-requests/pending?employee_id={employee_id}
func (h *ScreenshotPolicyHandler) GetPendingScreenshotRequests(c *gin.Context) {
	callerID, ok := auth.UserID(c)
	if !ok {
		unauthorized(c, "unauthenticated")
		return
	}

	empIDQuery := c.Query("employee_id")
	targetEmployeeID := callerID

	if empIDQuery != "" {
		if _, err := uuid.Parse(empIDQuery); err != nil {
			badRequest(c, "employee_id must be a valid uuid")
			return
		}
		if empIDQuery != callerID {
			owns, err := h.store.OwnsEmployee(c.Request.Context(), callerID, empIDQuery)
			if err != nil {
				serverError(c, err)
				return
			}
			if !owns {
				c.JSON(http.StatusForbidden, gin.H{"error": "cannot query requests for another employee"})
				return
			}
			targetEmployeeID = empIDQuery
		}
	}

	reqs, err := h.store.GetPendingScreenshotRequests(c.Request.Context(), targetEmployeeID)
	if err != nil {
		serverError(c, err)
		return
	}

	type reqItem struct {
		RequestID       string `json:"request_id"`
		ID              string `json:"id"`
		EmployeeID      string `json:"employee_id"`
		PolicyVersion   int    `json:"policy_version"`
		ExpiresAt       int64  `json:"expires_at"`
		CreatedAt       int64  `json:"created_at"`
		RequestedByID   string `json:"requested_by_id"`
		RequestedByName string `json:"requested_by_name"`
		RequestedBy     string `json:"requested_by"`
	}

	formatted := make([]reqItem, 0, len(reqs))
	for _, r := range reqs {
		requestedBy := r.RequestedByName
		if requestedBy == "" {
			requestedBy = r.RequestedByUserID
		}
		formatted = append(formatted, reqItem{
			RequestID:       r.ID,
			ID:              r.ID,
			EmployeeID:      r.EmployeeID,
			PolicyVersion:   r.PolicyVersion,
			ExpiresAt:       r.ExpiresAt.Unix(),
			CreatedAt:       r.CreatedAt.Unix(),
			RequestedByID:   r.RequestedByUserID,
			RequestedByName: r.RequestedByName,
			RequestedBy:     requestedBy,
		})
	}

	c.JSON(http.StatusOK, gin.H{"requests": formatted})
}

type ackRequestReq struct {
	RequestID            *string `json:"request_id"`
	EmployeeID           *string `json:"employee_id"`
	Status               string  `json:"status"`
	PolicyVersion        *int    `json:"policy_version"`
	ScreenshotUUID       *string `json:"screenshot_uuid"`
	ScreenshotClientUUID *string `json:"screenshot_client_uuid"`
	ErrorMessage         *string `json:"error_message"`
	CompletedAt          *int64  `json:"completed_at"`
}

// AckScreenshotRequest serves POST /v1/screenshot-requests/{request_id}/ack
func (h *ScreenshotPolicyHandler) AckScreenshotRequest(c *gin.Context) {
	callerID, ok := auth.UserID(c)
	if !ok {
		unauthorized(c, "unauthenticated")
		return
	}

	requestID := c.Param("request_id")
	if _, err := uuid.Parse(requestID); err != nil {
		badRequest(c, "request_id must be a valid uuid")
		return
	}

	var req ackRequestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid body")
		return
	}

	if req.Status == "" {
		badRequest(c, "status is required")
		return
	}

	// Support both screenshot_uuid (desktop client) and screenshot_client_uuid
	var clientUUID *string
	if req.ScreenshotUUID != nil && *req.ScreenshotUUID != "" {
		clientUUID = req.ScreenshotUUID
	} else if req.ScreenshotClientUUID != nil && *req.ScreenshotClientUUID != "" {
		clientUUID = req.ScreenshotClientUUID
	}

	if clientUUID != nil {
		if _, err := uuid.Parse(*clientUUID); err != nil {
			badRequest(c, "screenshot_uuid must be a valid uuid")
			return
		}
	}

	policyVer := 0
	if req.PolicyVersion != nil {
		policyVer = *req.PolicyVersion
	}

	updated, err := h.store.AckScreenshotRequest(c.Request.Context(), requestID, callerID, req.Status, policyVer, clientUUID, req.ErrorMessage)
	switch {
	case errors.Is(err, store.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
		return
	case errors.Is(err, store.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "not your capture request"})
		return
	case errors.Is(err, store.ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "request is already finalized or terminal"})
		return
	case errors.Is(err, store.ErrRequestExpired):
		c.JSON(http.StatusBadRequest, gin.H{"error": "request has expired", "status": "rejected_expired"})
		return
	case err != nil:
		serverError(c, err)
		return
	}

	resp := gin.H{
		"request_id": updated.ID,
		"status":     updated.Status,
	}
	if updated.AcknowledgedAt != nil {
		resp["acknowledged_at"] = updated.AcknowledgedAt.Unix()
	}
	if updated.CompletedAt != nil {
		resp["completed_at"] = updated.CompletedAt.Unix()
	}

	c.JSON(http.StatusOK, resp)
}

// --- Manager / Owner administration endpoints ---

// GetScreenshotSettings serves GET /v1/businesses/:id/screenshot-settings
func (h *ScreenshotPolicyHandler) GetScreenshotSettings(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	bizID := c.Param("id")

	owns, err := h.store.IsBusinessOwner(c.Request.Context(), callerID, bizID)
	if err != nil {
		serverError(c, err)
		return
	}
	if !owns {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your business"})
		return
	}

	biz, err := h.store.GetBusiness(c.Request.Context(), bizID)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "business not found"})
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"business_id":                biz.ID,
		"screenshots_enabled":        biz.ScreenshotsEnabled,
		"continuous_capture_enabled": biz.ContinuousCaptureEnabled,
		"screenshot_interval_s":      biz.ScreenshotIntervalS,
		"manual_capture_enabled":     biz.ManualCaptureEnabled,
		"allow_employee_override":    biz.AllowEmployeeOverride,
		"screenshot_mode":            biz.ScreenshotMode,
		"screenshot_skip_apps":       biz.ScreenshotSkipApps,
		"screenshot_retention_days":  biz.ScreenshotRetentionDays,
		"policy_version":             biz.PolicyVersion,
		"policy_updated_at":          biz.PolicyUpdatedAt.Format(time.RFC3339),
	})
}

// UpdateScreenshotSettings serves PATCH /v1/businesses/:id/screenshot-settings
func (h *ScreenshotPolicyHandler) UpdateScreenshotSettings(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	bizID := c.Param("id")

	owns, err := h.store.IsBusinessOwner(c.Request.Context(), callerID, bizID)
	if err != nil {
		serverError(c, err)
		return
	}
	if !owns {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your business"})
		return
	}

	var body map[string]json.RawMessage
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "invalid body")
		return
	}

	fields := map[string]any{}

	for _, bKey := range []string{"screenshots_enabled", "continuous_capture_enabled", "manual_capture_enabled", "allow_employee_override"} {
		if raw, ok := body[bKey]; ok {
			var b bool
			if json.Unmarshal(raw, &b) != nil {
				badRequest(c, bKey+" must be a boolean")
				return
			}
			fields[bKey] = b
		}
	}

	if raw, ok := body["screenshot_interval_s"]; ok {
		var n int
		if json.Unmarshal(raw, &n) != nil || n <= 0 {
			badRequest(c, "screenshot_interval_s must be a positive integer")
			return
		}
		fields["screenshot_interval_s"] = n
	}

	if raw, ok := body["idle_threshold_s"]; ok {
		var n int
		if json.Unmarshal(raw, &n) != nil || n <= 0 {
			badRequest(c, "idle_threshold_s must be a positive integer")
			return
		}
		fields["idle_threshold_s"] = n
	}

	if raw, ok := body["screenshot_mode"]; ok {
		var m string
		if json.Unmarshal(raw, &m) != nil || (m != "privacy" && m != "normal") {
			badRequest(c, "screenshot_mode must be 'privacy' or 'normal'")
			return
		}
		fields["screenshot_mode"] = m
	}

	if err := h.store.UpdateBusinessSettings(c.Request.Context(), bizID, fields); err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GetEmployeePolicy serves GET /v1/businesses/:id/employees/:employee_id/policy
func (h *ScreenshotPolicyHandler) GetEmployeePolicy(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	bizID := c.Param("id")
	empID := c.Param("employee_id")

	owns, err := h.store.IsBusinessOwner(c.Request.Context(), callerID, bizID)
	if err != nil {
		serverError(c, err)
		return
	}
	if !owns {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your business"})
		return
	}

	isMember, err := h.store.IsMember(c.Request.Context(), empID, bizID)
	if err != nil {
		serverError(c, err)
		return
	}
	if !isMember {
		c.JSON(http.StatusNotFound, gin.H{"error": "employee not found in this business"})
		return
	}

	detail, err := h.store.GetEmployeePolicyDetail(c.Request.Context(), bizID, empID)
	if err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, detail)
}

type updateEmployeePolicyReq struct {
	ScreenshotsEnabled        *bool `json:"screenshots_enabled"`
	ContinuousCaptureEnabled  *bool `json:"continuous_capture_enabled"`
	ScreenshotIntervalS       *int  `json:"screenshot_interval_s"`
	ManualCaptureEnabled      *bool `json:"manual_capture_enabled"`
}

// UpdateEmployeePolicy serves PATCH /v1/businesses/:id/employees/:employee_id/policy
func (h *ScreenshotPolicyHandler) UpdateEmployeePolicy(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	bizID := c.Param("id")
	empID := c.Param("employee_id")

	owns, err := h.store.IsBusinessOwner(c.Request.Context(), callerID, bizID)
	if err != nil {
		serverError(c, err)
		return
	}
	if !owns {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your business"})
		return
	}

	isMember, err := h.store.IsMember(c.Request.Context(), empID, bizID)
	if err != nil {
		serverError(c, err)
		return
	}
	if !isMember {
		c.JSON(http.StatusNotFound, gin.H{"error": "employee not found in this business"})
		return
	}

	var req updateEmployeePolicyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid body")
		return
	}

	if req.ScreenshotIntervalS != nil && *req.ScreenshotIntervalS <= 0 {
		badRequest(c, "screenshot_interval_s must be a positive integer or null")
		return
	}

	if err := h.store.UpdateEmployeePolicyOverrides(c.Request.Context(), bizID, empID,
		req.ScreenshotsEnabled, req.ContinuousCaptureEnabled, req.ManualCaptureEnabled, req.ScreenshotIntervalS); err != nil {
		serverError(c, err)
		return
	}

	detail, err := h.store.GetEmployeePolicyDetail(c.Request.Context(), bizID, empID)
	if err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, detail)
}

// ResetEmployeePolicy serves POST /v1/businesses/:id/employees/:employee_id/policy/reset
func (h *ScreenshotPolicyHandler) ResetEmployeePolicy(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	bizID := c.Param("id")
	empID := c.Param("employee_id")

	owns, err := h.store.IsBusinessOwner(c.Request.Context(), callerID, bizID)
	if err != nil {
		serverError(c, err)
		return
	}
	if !owns {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your business"})
		return
	}

	if err := h.store.ResetEmployeePolicy(c.Request.Context(), bizID, empID); err != nil {
		serverError(c, err)
		return
	}

	detail, err := h.store.GetEmployeePolicyDetail(c.Request.Context(), bizID, empID)
	if err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "policy": detail})
}

type createRequestReq struct {
	ExpiryMinutes *int `json:"expiry_minutes"`
}

// CreateScreenshotRequest serves POST /v1/businesses/:id/employees/:employee_id/screenshot-requests ("Capture Now")
func (h *ScreenshotPolicyHandler) CreateScreenshotRequest(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	bizID := c.Param("id")
	empID := c.Param("employee_id")

	owns, err := h.store.IsBusinessOwner(c.Request.Context(), callerID, bizID)
	if err != nil {
		serverError(c, err)
		return
	}
	if !owns {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your business"})
		return
	}

	isMember, err := h.store.IsMember(c.Request.Context(), empID, bizID)
	if err != nil {
		serverError(c, err)
		return
	}
	if !isMember {
		c.JSON(http.StatusNotFound, gin.H{"error": "employee not found in this business"})
		return
	}

	var req createRequestReq
	_ = c.ShouldBindJSON(&req) // optional body

	expiryDuration := 15 * time.Minute
	if req.ExpiryMinutes != nil && *req.ExpiryMinutes > 0 && *req.ExpiryMinutes <= 60 {
		expiryDuration = time.Duration(*req.ExpiryMinutes) * time.Minute
	}

	shotReq, err := h.store.CreateScreenshotRequest(c.Request.Context(), bizID, empID, callerID, expiryDuration)
	switch {
	case errors.Is(err, store.ErrCaptureDisabled):
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot request screenshot: capture is disabled by policy"})
		return
	case errors.Is(err, store.ErrManualCaptureDisabled):
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot request screenshot: manual requests are disabled by policy"})
		return
	case err != nil:
		serverError(c, err)
		return
	}

	c.JSON(http.StatusCreated, shotReq)
}

// ListEmployeeScreenshotRequests serves GET /v1/businesses/:id/employees/:employee_id/screenshot-requests
func (h *ScreenshotPolicyHandler) ListEmployeeScreenshotRequests(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	bizID := c.Param("id")
	empID := c.Param("employee_id")

	owns, err := h.store.IsBusinessOwner(c.Request.Context(), callerID, bizID)
	if err != nil {
		serverError(c, err)
		return
	}
	if !owns {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your business"})
		return
	}

	limit := 20
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	requests, err := h.store.ListEmployeeScreenshotRequests(c.Request.Context(), bizID, empID, limit)
	if err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"requests": requests})
}

// RetryScreenshotRequest serves POST /v1/businesses/:id/screenshot-requests/:request_id/retry
func (h *ScreenshotPolicyHandler) RetryScreenshotRequest(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	bizID := c.Param("id")
	requestID := c.Param("request_id")

	owns, err := h.store.IsBusinessOwner(c.Request.Context(), callerID, bizID)
	if err != nil {
		serverError(c, err)
		return
	}
	if !owns {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your business"})
		return
	}

	newReq, err := h.store.RetryScreenshotRequest(c.Request.Context(), bizID, requestID, callerID, 15*time.Minute)
	switch {
	case errors.Is(err, store.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
		return
	case errors.Is(err, store.ErrCaptureDisabled):
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot retry screenshot request: capture is disabled"})
		return
	case errors.Is(err, store.ErrManualCaptureDisabled):
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot retry screenshot request: manual capture is disabled"})
		return
	case err != nil:
		badRequest(c, err.Error())
		return
	}

	c.JSON(http.StatusCreated, newReq)
}
