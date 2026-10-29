package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrRequestNotPending is returned when trying to ack or modify a request not in pending state.
var ErrRequestNotPending = errors.New("request is not pending")

// ErrRequestExpired is returned when a request has passed its expiry time.
var ErrRequestExpired = errors.New("request has expired")

// ErrCaptureDisabled is returned when attempting a manual capture request while capture is disabled.
var ErrCaptureDisabled = errors.New("screenshot capture is disabled")

// ErrManualCaptureDisabled is returned when manual screenshot requests are disabled by policy.
var ErrManualCaptureDisabled = errors.New("manual screenshot capture is disabled")

// EffectiveScreenshotPolicy represents the resolved capture policy for an employee.
type EffectiveScreenshotPolicy struct {
	EmployeeID               string    `json:"employee_id"`
	BusinessID               string    `json:"business_id"`
	CaptureEnabled           bool      `json:"capture_enabled"`
	ContinuousCaptureEnabled bool      `json:"continuous_capture_enabled"`
	CaptureIntervalMinutes   int       `json:"capture_interval_minutes"`
	CaptureIntervalSeconds   int       `json:"capture_interval_seconds"`
	ManualCaptureEnabled     bool      `json:"manual_capture_enabled"`
	PolicyVersion            int       `json:"policy_version"`
	PolicyUpdatedAt          time.Time `json:"policy_updated_at"`
	LastAckVersion           *int      `json:"last_ack_version,omitempty"`
	LastAckAt                *time.Time `json:"last_ack_at,omitempty"`
}

// EmployeePolicyOverrides represents nullable override settings for an employee.
// Nullable fields mean "inherit from business default".
type EmployeePolicyOverrides struct {
	ScreenshotsEnabled        *bool      `json:"screenshots_enabled"`
	ContinuousCaptureEnabled  *bool      `json:"continuous_capture_enabled"`
	ScreenshotIntervalS       *int       `json:"screenshot_interval_s"`
	ManualCaptureEnabled      *bool      `json:"manual_capture_enabled"`
	PolicyVersion             int        `json:"policy_version"`
	PolicyUpdatedAt           time.Time  `json:"policy_updated_at"`
	LastAckVersion            *int       `json:"last_ack_version"`
	LastAckAt                 *time.Time `json:"last_ack_at"`
	LastAckDeviceID           *string    `json:"last_ack_device_id"`
}

// EmployeePolicyDetail groups effective policy, overrides, and business defaults.
type EmployeePolicyDetail struct {
	EmployeeID       string                    `json:"employee_id"`
	BusinessID       string                    `json:"business_id"`
	Effective        EffectiveScreenshotPolicy `json:"effective"`
	Overrides        EmployeePolicyOverrides   `json:"overrides"`
	BusinessDefaults BusinessScreenshotDefaults `json:"business_defaults"`
}

// BusinessScreenshotDefaults holds business-level defaults.
type BusinessScreenshotDefaults struct {
	ScreenshotsEnabled       bool      `json:"screenshots_enabled"`
	ContinuousCaptureEnabled bool      `json:"continuous_capture_enabled"`
	ScreenshotIntervalS      int       `json:"screenshot_interval_s"`
	ManualCaptureEnabled     bool      `json:"manual_capture_enabled"`
	AllowEmployeeOverride    bool      `json:"allow_employee_override"`
	ScreenshotMode           string    `json:"screenshot_mode"`
	PolicyVersion            int       `json:"policy_version"`
	PolicyUpdatedAt          time.Time `json:"policy_updated_at"`
}

// ScreenshotRequestRow represents a manual screenshot request.
type ScreenshotRequestRow struct {
	ID                   string     `json:"id"`
	BusinessID           string     `json:"business_id"`
	EmployeeID           string     `json:"employee_id"`
	RequestedByUserID    string     `json:"requested_by_user_id"`
	RequestedByName      string     `json:"requested_by_name"`
	Status               string     `json:"status"`
	PolicyVersion        int        `json:"policy_version"`
	CreatedAt            time.Time  `json:"created_at"`
	ExpiresAt            time.Time  `json:"expires_at"`
	AcknowledgedAt       *time.Time `json:"acknowledged_at,omitempty"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	ScreenshotClientUUID *string    `json:"screenshot_client_uuid,omitempty"`
	ErrorMessage         *string    `json:"error_message,omitempty"`
}

// GetEffectiveScreenshotPolicy calculates the effective policy following all 5 rules:
// 1. If global screenshot capture is disabled, all capture is disabled.
// 2. If the employee has explicitly disabled capture, capture is disabled for that employee.
// 3. Continuous capture is independently configurable.
// 4. The employee interval override applies when present; otherwise inherit the business interval.
// 5. Manual capture is permitted only if the global capture switch, employee capture permission,
//    and applicable manual-request settings allow it.
func (s *Store) GetEffectiveScreenshotPolicy(ctx context.Context, businessID, employeeID string) (*EffectiveScreenshotPolicy, error) {
	// 1. Fetch business screenshot settings
	var biz BusinessScreenshotDefaults
	err := s.pool.QueryRow(ctx, `
		SELECT screenshots_enabled, continuous_capture_enabled, screenshot_interval_s, manual_capture_enabled,
		       allow_employee_override, screenshot_mode, policy_version, policy_updated_at
		  FROM businesses WHERE id = $1`, businessID,
	).Scan(&biz.ScreenshotsEnabled, &biz.ContinuousCaptureEnabled, &biz.ScreenshotIntervalS, &biz.ManualCaptureEnabled,
		&biz.AllowEmployeeOverride, &biz.ScreenshotMode, &biz.PolicyVersion, &biz.PolicyUpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// 2. Fetch employee override if present
	var ovr EmployeePolicyOverrides
	row := s.pool.QueryRow(ctx, `
		SELECT screenshots_enabled, continuous_capture_enabled, screenshot_interval_s, manual_capture_enabled,
		       policy_version, policy_updated_at, last_ack_version, last_ack_at, last_ack_device_id
		  FROM employee_screenshot_policies
		 WHERE business_id = $1 AND employee_id = $2`, businessID, employeeID)
	hasOverride := true
	err = row.Scan(&ovr.ScreenshotsEnabled, &ovr.ContinuousCaptureEnabled, &ovr.ScreenshotIntervalS, &ovr.ManualCaptureEnabled,
		&ovr.PolicyVersion, &ovr.PolicyUpdatedAt, &ovr.LastAckVersion, &ovr.LastAckAt, &ovr.LastAckDeviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		hasOverride = false
		ovr.PolicyVersion = 0
		ovr.PolicyUpdatedAt = biz.PolicyUpdatedAt
	} else if err != nil {
		return nil, err
	}

	effective := ResolveEffectivePolicy(biz, ovr, hasOverride, employeeID, businessID)
	return &effective, nil
}

// ResolveEffectivePolicy is the pure calculation function implementing the policy rules.
func ResolveEffectivePolicy(biz BusinessScreenshotDefaults, ovr EmployeePolicyOverrides, hasOverride bool, employeeID, businessID string) EffectiveScreenshotPolicy {
	// Combined policy version and latest update timestamp
	combinedVersion := biz.PolicyVersion
	latestUpdatedAt := biz.PolicyUpdatedAt
	if hasOverride {
		combinedVersion += ovr.PolicyVersion
		if ovr.PolicyUpdatedAt.After(latestUpdatedAt) {
			latestUpdatedAt = ovr.PolicyUpdatedAt
		}
	}

	// Rule 1: If global screenshot capture is disabled, all capture is disabled.
	if !biz.ScreenshotsEnabled {
		return EffectiveScreenshotPolicy{
			EmployeeID:               employeeID,
			BusinessID:               businessID,
			CaptureEnabled:           false,
			ContinuousCaptureEnabled: false,
			CaptureIntervalMinutes:   calcMinutes(biz.ScreenshotIntervalS),
			CaptureIntervalSeconds:   biz.ScreenshotIntervalS,
			ManualCaptureEnabled:     false,
			PolicyVersion:            combinedVersion,
			PolicyUpdatedAt:          latestUpdatedAt,
			LastAckVersion:           ovr.LastAckVersion,
			LastAckAt:                ovr.LastAckAt,
		}
	}

	// Rule 2: If the employee has explicitly disabled capture, capture is disabled for that employee.
	if ovr.ScreenshotsEnabled != nil && !*ovr.ScreenshotsEnabled {
		return EffectiveScreenshotPolicy{
			EmployeeID:               employeeID,
			BusinessID:               businessID,
			CaptureEnabled:           false,
			ContinuousCaptureEnabled: false,
			CaptureIntervalMinutes:   calcMinutes(biz.ScreenshotIntervalS),
			CaptureIntervalSeconds:   biz.ScreenshotIntervalS,
			ManualCaptureEnabled:     false,
			PolicyVersion:            combinedVersion,
			PolicyUpdatedAt:          latestUpdatedAt,
			LastAckVersion:           ovr.LastAckVersion,
			LastAckAt:                ovr.LastAckAt,
		}
	}

	// At this point, capture is enabled for this employee.
	captureEnabled := true

	// Rule 3: Continuous capture is independently configurable.
	continuousCaptureEnabled := biz.ContinuousCaptureEnabled
	if ovr.ContinuousCaptureEnabled != nil {
		continuousCaptureEnabled = *ovr.ContinuousCaptureEnabled
	}

	// Rule 4: The employee interval override applies when present; otherwise inherit the business interval.
	intervalS := biz.ScreenshotIntervalS
	if ovr.ScreenshotIntervalS != nil && *ovr.ScreenshotIntervalS > 0 {
		intervalS = *ovr.ScreenshotIntervalS
	}

	// Rule 5: Manual capture is permitted only if the global capture switch, employee capture permission,
	// and applicable manual-request settings allow it.
	manualCaptureEnabled := biz.ManualCaptureEnabled
	if ovr.ManualCaptureEnabled != nil {
		manualCaptureEnabled = manualCaptureEnabled && *ovr.ManualCaptureEnabled
	}

	return EffectiveScreenshotPolicy{
		EmployeeID:               employeeID,
		BusinessID:               businessID,
		CaptureEnabled:           captureEnabled,
		ContinuousCaptureEnabled: continuousCaptureEnabled,
		CaptureIntervalMinutes:   calcMinutes(intervalS),
		CaptureIntervalSeconds:   intervalS,
		ManualCaptureEnabled:     manualCaptureEnabled,
		PolicyVersion:            combinedVersion,
		PolicyUpdatedAt:          latestUpdatedAt,
		LastAckVersion:           ovr.LastAckVersion,
		LastAckAt:                ovr.LastAckAt,
	}
}

func calcMinutes(seconds int) int {
	mins := seconds / 60
	if mins < 1 {
		return 1
	}
	return mins
}

// AckScreenshotPolicy records an employee's policy acknowledgment.
func (s *Store) AckScreenshotPolicy(ctx context.Context, businessID, employeeID string, version int, deviceID *string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO employee_screenshot_policies
		  (business_id, employee_id, last_ack_version, last_ack_at, last_ack_device_id, updated_at)
		VALUES ($1, $2, $3, now(), $4, now())
		ON CONFLICT (business_id, employee_id) DO UPDATE SET
		  last_ack_version = EXCLUDED.last_ack_version,
		  last_ack_at = now(),
		  last_ack_device_id = COALESCE(EXCLUDED.last_ack_device_id, employee_screenshot_policies.last_ack_device_id),
		  updated_at = now()`,
		businessID, employeeID, version, deviceID)
	return err
}

// GetEmployeePolicyDetail returns effective policy, overrides, and business defaults.
func (s *Store) GetEmployeePolicyDetail(ctx context.Context, businessID, employeeID string) (*EmployeePolicyDetail, error) {
	var biz BusinessScreenshotDefaults
	err := s.pool.QueryRow(ctx, `
		SELECT screenshots_enabled, continuous_capture_enabled, screenshot_interval_s, manual_capture_enabled,
		       allow_employee_override, screenshot_mode, policy_version, policy_updated_at
		  FROM businesses WHERE id = $1`, businessID,
	).Scan(&biz.ScreenshotsEnabled, &biz.ContinuousCaptureEnabled, &biz.ScreenshotIntervalS, &biz.ManualCaptureEnabled,
		&biz.AllowEmployeeOverride, &biz.ScreenshotMode, &biz.PolicyVersion, &biz.PolicyUpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var ovr EmployeePolicyOverrides
	row := s.pool.QueryRow(ctx, `
		SELECT screenshots_enabled, continuous_capture_enabled, screenshot_interval_s, manual_capture_enabled,
		       policy_version, policy_updated_at, last_ack_version, last_ack_at, last_ack_device_id
		  FROM employee_screenshot_policies
		 WHERE business_id = $1 AND employee_id = $2`, businessID, employeeID)
	hasOverride := true
	err = row.Scan(&ovr.ScreenshotsEnabled, &ovr.ContinuousCaptureEnabled, &ovr.ScreenshotIntervalS, &ovr.ManualCaptureEnabled,
		&ovr.PolicyVersion, &ovr.PolicyUpdatedAt, &ovr.LastAckVersion, &ovr.LastAckAt, &ovr.LastAckDeviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		hasOverride = false
		ovr.PolicyVersion = 0
		ovr.PolicyUpdatedAt = biz.PolicyUpdatedAt
	} else if err != nil {
		return nil, err
	}

	effective := ResolveEffectivePolicy(biz, ovr, hasOverride, employeeID, businessID)
	return &EmployeePolicyDetail{
		EmployeeID:       employeeID,
		BusinessID:       businessID,
		Effective:        effective,
		Overrides:        ovr,
		BusinessDefaults: biz,
	}, nil
}

// UpdateEmployeePolicyOverrides updates individual employee override settings.
// Passing nil for a setting resets it to inherit the business default.
func (s *Store) UpdateEmployeePolicyOverrides(ctx context.Context, businessID, employeeID string,
	screenshotsEnabled, continuousEnabled, manualEnabled *bool, intervalS *int) error {

	_, err := s.pool.Exec(ctx, `
		INSERT INTO employee_screenshot_policies
		  (business_id, employee_id, screenshots_enabled, continuous_capture_enabled, screenshot_interval_s, manual_capture_enabled, policy_version, policy_updated_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 1, now(), now())
		ON CONFLICT (business_id, employee_id) DO UPDATE SET
		  screenshots_enabled = EXCLUDED.screenshots_enabled,
		  continuous_capture_enabled = EXCLUDED.continuous_capture_enabled,
		  screenshot_interval_s = EXCLUDED.screenshot_interval_s,
		  manual_capture_enabled = EXCLUDED.manual_capture_enabled,
		  policy_version = employee_screenshot_policies.policy_version + 1,
		  policy_updated_at = now(),
		  updated_at = now()`,
		businessID, employeeID, screenshotsEnabled, continuousEnabled, intervalS, manualEnabled)
	return err
}

// ResetEmployeePolicy resets all employee overrides to inherited defaults.
func (s *Store) ResetEmployeePolicy(ctx context.Context, businessID, employeeID string) error {
	// We increment policy_version and set override columns to NULL so connected clients recognize the reset.
	ct, err := s.pool.Exec(ctx, `
		UPDATE employee_screenshot_policies
		   SET screenshots_enabled = NULL,
		       continuous_capture_enabled = NULL,
		       screenshot_interval_s = NULL,
		       manual_capture_enabled = NULL,
		       policy_version = policy_version + 1,
		       policy_updated_at = now(),
		       updated_at = now()
		 WHERE business_id = $1 AND employee_id = $2`, businessID, employeeID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		// Nothing to reset, row didn't exist
		return nil
	}
	return nil
}

// CreateScreenshotRequest creates an expiring manual screenshot request for an employee.
func (s *Store) CreateScreenshotRequest(ctx context.Context, businessID, employeeID, requesterUserID string, expiry time.Duration) (*ScreenshotRequestRow, error) {
	effective, err := s.GetEffectiveScreenshotPolicy(ctx, businessID, employeeID)
	if err != nil {
		return nil, err
	}
	if !effective.CaptureEnabled {
		return nil, ErrCaptureDisabled
	}
	if !effective.ManualCaptureEnabled {
		return nil, ErrManualCaptureDisabled
	}

	expiresAt := time.Now().Add(expiry)
	var req ScreenshotRequestRow
	err = s.pool.QueryRow(ctx, `
		INSERT INTO screenshot_requests
		  (business_id, employee_id, requested_by_user_id, status, policy_version, expires_at)
		VALUES ($1, $2, $3, 'pending', $4, $5)
		RETURNING id, business_id, employee_id, requested_by_user_id, status, policy_version, created_at, expires_at`,
		businessID, employeeID, requesterUserID, effective.PolicyVersion, expiresAt,
	).Scan(&req.ID, &req.BusinessID, &req.EmployeeID, &req.RequestedByUserID, &req.Status, &req.PolicyVersion, &req.CreatedAt, &req.ExpiresAt)
	if err != nil {
		return nil, err
	}

	// Fetch requester display name
	_ = s.pool.QueryRow(ctx, `SELECT display_name FROM users WHERE id = $1`, requesterUserID).Scan(&req.RequestedByName)

	return &req, nil
}

// GetPendingScreenshotRequests returns active, non-expired pending requests for an employee.
// It also marks any past-due pending requests as 'rejected_expired'.
func (s *Store) GetPendingScreenshotRequests(ctx context.Context, employeeID string) ([]ScreenshotRequestRow, error) {
	// Auto-expire past-due requests
	_, _ = s.pool.Exec(ctx, `
		UPDATE screenshot_requests
		   SET status = 'rejected_expired', completed_at = now()
		 WHERE employee_id = $1 AND status = 'pending' AND expires_at <= now()`, employeeID)

	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.business_id, r.employee_id, r.requested_by_user_id, u.display_name,
		       r.status, r.policy_version, r.created_at, r.expires_at,
		       r.acknowledged_at, r.completed_at, r.screenshot_client_uuid, r.error_message
		  FROM screenshot_requests r
		  JOIN users u ON u.id = r.requested_by_user_id
		 WHERE r.employee_id = $1 AND r.status = 'pending' AND r.expires_at > now()
		 ORDER BY r.created_at ASC`, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ScreenshotRequestRow{}
	for rows.Next() {
		var r ScreenshotRequestRow
		if err := rows.Scan(&r.ID, &r.BusinessID, &r.EmployeeID, &r.RequestedByUserID, &r.RequestedByName,
			&r.Status, &r.PolicyVersion, &r.CreatedAt, &r.ExpiresAt,
			&r.AcknowledgedAt, &r.CompletedAt, &r.ScreenshotClientUUID, &r.ErrorMessage); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// validAckStatuses sets the accepted terminal and ack statuses from the desktop client.
var validAckStatuses = map[string]bool{
	"acknowledged":                true,
	"completed":                   true,
	"rejected_disabled":           true,
	"rejected_manual_disabled":    true,
	"rejected_expired":            true,
	"rejected_wrong_employee":     true,
	"rejected_duplicate":          true,
	"rejected_stale_policy":       true,
	"rejected_privacy_exclusion":  true,
	"failed":                      true,
}

// AckScreenshotRequest records an acknowledgment or terminal state transition from the client.
func (s *Store) AckScreenshotRequest(ctx context.Context, requestID, employeeID, status string,
	policyVersion int, clientUUID, errMsg *string) (*ScreenshotRequestRow, error) {

	if !validAckStatuses[status] {
		return nil, fmt.Errorf("invalid request status: %s", status)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var req ScreenshotRequestRow
	err = tx.QueryRow(ctx, `
		SELECT r.id, r.business_id, r.employee_id, r.requested_by_user_id, u.display_name,
		       r.status, r.policy_version, r.created_at, r.expires_at,
		       r.acknowledged_at, r.completed_at, r.screenshot_client_uuid, r.error_message
		  FROM screenshot_requests r
		  JOIN users u ON u.id = r.requested_by_user_id
		 WHERE r.id = $1
		   FOR UPDATE`, requestID,
	).Scan(&req.ID, &req.BusinessID, &req.EmployeeID, &req.RequestedByUserID, &req.RequestedByName,
		&req.Status, &req.PolicyVersion, &req.CreatedAt, &req.ExpiresAt,
		&req.AcknowledgedAt, &req.CompletedAt, &req.ScreenshotClientUUID, &req.ErrorMessage)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// Verify request belongs to the authenticated employee
	if req.EmployeeID != employeeID {
		return nil, ErrForbidden
	}

	// Idempotency: If request is already completed with the same client UUID, return OK
	if req.Status == "completed" {
		if clientUUID != nil && req.ScreenshotClientUUID != nil && *clientUUID == *req.ScreenshotClientUUID {
			return &req, nil
		}
		return nil, ErrConflict
	}

	// Check if request is already in another terminal state (e.g. rejected or failed)
	if req.Status != "pending" && req.Status != "acknowledged" {
		if req.Status == status {
			return &req, nil
		}
		return nil, ErrConflict
	}

	// Check expiration
	if time.Now().After(req.ExpiresAt) && status != "rejected_expired" {
		_, _ = tx.Exec(ctx, `UPDATE screenshot_requests SET status = 'rejected_expired', completed_at = now() WHERE id = $1`, requestID)
		_ = tx.Commit(ctx)
		return nil, ErrRequestExpired
	}

	now := time.Now()
	ackAt := req.AcknowledgedAt
	if ackAt == nil {
		ackAt = &now
	}

	var completedAt *time.Time
	finalStatus := status
	scUUID := req.ScreenshotClientUUID
	if clientUUID != nil && *clientUUID != "" {
		scUUID = clientUUID
	}

	if status == "completed" {
		if scUUID == nil || *scUUID == "" {
			return nil, errors.New("screenshot_uuid is required for completed status")
		}

		// Prevent duplicate reuse of the same screenshot UUID across different completed requests
		var otherReqID string
		err := tx.QueryRow(ctx, `
			SELECT id FROM screenshot_requests
			 WHERE screenshot_client_uuid = $1 AND id != $2 AND status = 'completed'
			 LIMIT 1`, *scUUID, requestID,
		).Scan(&otherReqID)
		if err == nil {
			return nil, fmt.Errorf("screenshot_uuid %s is already used by request %s: %w", *scUUID, otherReqID, ErrConflict)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}

		// Check if the screenshot has already been uploaded to the backend
		var shotUserID, shotBizID string
		err = tx.QueryRow(ctx, `
			SELECT user_id, business_id FROM screenshots
			 WHERE client_uuid = $1`, *scUUID,
		).Scan(&shotUserID, &shotBizID)

		if errors.Is(err, pgx.ErrNoRows) {
			// Image has not been uploaded yet: mark acknowledged and await upload to satisfy completion semantics
			finalStatus = "acknowledged"
			completedAt = nil
		} else if err != nil {
			return nil, err
		} else {
			// Verify screenshot ownership (cannot claim a screenshot belonging to another employee or business)
			if shotUserID != employeeID || shotBizID != req.BusinessID {
				return nil, ErrForbidden
			}
			// Image is already uploaded: link and complete immediately!
			finalStatus = "completed"
			completedAt = &now
			_, err = tx.Exec(ctx, `
				UPDATE screenshots
				   SET request_id = $1
				 WHERE client_uuid = $2 AND user_id = $3 AND business_id = $4`,
				requestID, *scUUID, employeeID, req.BusinessID)
			if err != nil {
				return nil, err
			}
		}
	} else if status != "acknowledged" {
		// Non-completed terminal status (rejected_* or failed)
		completedAt = &now
	}

	_, err = tx.Exec(ctx, `
		UPDATE screenshot_requests
		   SET status = $1,
		       acknowledged_at = $2,
		       completed_at = $3,
		       screenshot_client_uuid = $4,
		       error_message = $5
		 WHERE id = $6`,
		finalStatus, ackAt, completedAt, scUUID, errMsg, requestID)
	if err != nil {
		return nil, err
	}

	req.Status = finalStatus
	req.AcknowledgedAt = ackAt
	req.CompletedAt = completedAt
	req.ScreenshotClientUUID = scUUID
	req.ErrorMessage = errMsg

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &req, nil
}

// LinkUploadedScreenshot links an uploaded screenshot to its manual request and completes it.
// It matches by client_uuid (or optional requestID), strictly checking employee_id and business_id.
func (s *Store) LinkUploadedScreenshot(ctx context.Context, employeeID, businessID, clientUUID string, requestID *string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var req ScreenshotRequestRow
	var query string
	var args []any

	if requestID != nil && *requestID != "" {
		query = `
			SELECT r.id, r.business_id, r.employee_id, r.requested_by_user_id, u.display_name,
			       r.status, r.policy_version, r.created_at, r.expires_at,
			       r.acknowledged_at, r.completed_at, r.screenshot_client_uuid, r.error_message
			  FROM screenshot_requests r
			  JOIN users u ON u.id = r.requested_by_user_id
			 WHERE r.id = $1 AND r.employee_id = $2 AND r.business_id = $3
			   FOR UPDATE`
		args = []any{*requestID, employeeID, businessID}
	} else {
		query = `
			SELECT r.id, r.business_id, r.employee_id, r.requested_by_user_id, u.display_name,
			       r.status, r.policy_version, r.created_at, r.expires_at,
			       r.acknowledged_at, r.completed_at, r.screenshot_client_uuid, r.error_message
			  FROM screenshot_requests r
			  JOIN users u ON u.id = r.requested_by_user_id
			 WHERE r.employee_id = $1 AND r.business_id = $2 AND r.screenshot_client_uuid = $3 AND r.status IN ('pending', 'acknowledged')
			 ORDER BY r.created_at DESC
			 LIMIT 1
			   FOR UPDATE`
		args = []any{employeeID, businessID, clientUUID}
	}

	err = tx.QueryRow(ctx, query, args...).Scan(
		&req.ID, &req.BusinessID, &req.EmployeeID, &req.RequestedByUserID, &req.RequestedByName,
		&req.Status, &req.PolicyVersion, &req.CreatedAt, &req.ExpiresAt,
		&req.AcknowledgedAt, &req.CompletedAt, &req.ScreenshotClientUUID, &req.ErrorMessage,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		// Normal background capture tick (not associated with any in-flight manual request)
		return nil
	}
	if err != nil {
		return err
	}

	if req.Status == "completed" {
		// Already completed; ensure screenshots.request_id points to req.ID
		_, _ = tx.Exec(ctx, `
			UPDATE screenshots
			   SET request_id = $1
			 WHERE client_uuid = $2 AND user_id = $3 AND business_id = $4 AND (request_id IS NULL OR request_id = $1)`,
			req.ID, clientUUID, employeeID, businessID)
		return tx.Commit(ctx)
	}

	// If request expired before image arrived
	if time.Now().After(req.ExpiresAt) {
		_, _ = tx.Exec(ctx, `UPDATE screenshot_requests SET status = 'rejected_expired', completed_at = now() WHERE id = $1`, req.ID)
		return tx.Commit(ctx)
	}

	// Link screenshot to request
	_, err = tx.Exec(ctx, `
		UPDATE screenshots
		   SET request_id = $1
		 WHERE client_uuid = $2 AND user_id = $3 AND business_id = $4`,
		req.ID, clientUUID, employeeID, businessID)
	if err != nil {
		return err
	}

	// Mark request completed!
	now := time.Now()
	_, err = tx.Exec(ctx, `
		UPDATE screenshot_requests
		   SET status = 'completed',
		       screenshot_client_uuid = $1,
		       acknowledged_at = COALESCE(acknowledged_at, $2),
		       completed_at = $2
		 WHERE id = $3`,
		clientUUID, now, req.ID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// LinkScreenshotToRequest maintains backward compatibility by delegating to LinkUploadedScreenshot.
func (s *Store) LinkScreenshotToRequest(ctx context.Context, requestID, employeeID, businessID, clientUUID string) error {
	return s.LinkUploadedScreenshot(ctx, employeeID, businessID, clientUUID, &requestID)
}

// ListEmployeeScreenshotRequests returns the most recent requests for an employee.
func (s *Store) ListEmployeeScreenshotRequests(ctx context.Context, businessID, employeeID string, limit int) ([]ScreenshotRequestRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.business_id, r.employee_id, r.requested_by_user_id, u.display_name,
		       r.status, r.policy_version, r.created_at, r.expires_at,
		       r.acknowledged_at, r.completed_at, r.screenshot_client_uuid, r.error_message
		  FROM screenshot_requests r
		  JOIN users u ON u.id = r.requested_by_user_id
		 WHERE r.business_id = $1 AND r.employee_id = $2
		 ORDER BY r.created_at DESC
		 LIMIT $3`, businessID, employeeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ScreenshotRequestRow{}
	for rows.Next() {
		var r ScreenshotRequestRow
		if err := rows.Scan(&r.ID, &r.BusinessID, &r.EmployeeID, &r.RequestedByUserID, &r.RequestedByName,
			&r.Status, &r.PolicyVersion, &r.CreatedAt, &r.ExpiresAt,
			&r.AcknowledgedAt, &r.CompletedAt, &r.ScreenshotClientUUID, &r.ErrorMessage); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetScreenshotRequest returns a single screenshot request by id.
func (s *Store) GetScreenshotRequest(ctx context.Context, businessID, requestID string) (*ScreenshotRequestRow, error) {
	var r ScreenshotRequestRow
	err := s.pool.QueryRow(ctx, `
		SELECT r.id, r.business_id, r.employee_id, r.requested_by_user_id, u.display_name,
		       r.status, r.policy_version, r.created_at, r.expires_at,
		       r.acknowledged_at, r.completed_at, r.screenshot_client_uuid, r.error_message
		  FROM screenshot_requests r
		  JOIN users u ON u.id = r.requested_by_user_id
		 WHERE r.business_id = $1 AND r.id = $2`, businessID, requestID,
	).Scan(&r.ID, &r.BusinessID, &r.EmployeeID, &r.RequestedByUserID, &r.RequestedByName,
		&r.Status, &r.PolicyVersion, &r.CreatedAt, &r.ExpiresAt,
		&r.AcknowledgedAt, &r.CompletedAt, &r.ScreenshotClientUUID, &r.ErrorMessage)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// RetryScreenshotRequest re-issues an eligible failed or expired request as a new pending request.
func (s *Store) RetryScreenshotRequest(ctx context.Context, businessID, requestID, requesterUserID string, expiry time.Duration) (*ScreenshotRequestRow, error) {
	oldReq, err := s.GetScreenshotRequest(ctx, businessID, requestID)
	if err != nil {
		return nil, err
	}
	// Verify that the request is failed or expired
	if oldReq.Status == "pending" || oldReq.Status == "completed" {
		return nil, fmt.Errorf("request in state %s cannot be retried", oldReq.Status)
	}

	return s.CreateScreenshotRequest(ctx, businessID, oldReq.EmployeeID, requesterUserID, expiry)
}
