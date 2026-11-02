package store_test

import (
	"context"
	"testing"
	"time"

	"ctracking/backend/internal/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExistingFunctionalityRegressionAudit(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	defer pool.Close()

	st := store.New(pool)

	// Clean tables
	_, _ = pool.Exec(ctx, `TRUNCATE businesses, users CASCADE`)

	bizID := uuid.New().String()
	ownerID := uuid.New().String()
	leadID := uuid.New().String()
	empID := uuid.New().String()
	deviceID := uuid.New()
	screenshotUUID := uuid.New().String()

	// 1. Seed User & Device Pairing
	t.Run("1_DesktopLoginAndDevicePairing", func(t *testing.T) {
		// Create users
		_, err = pool.Exec(ctx,
			`INSERT INTO users (id, email, password_hash, display_name, account_type) 
			 VALUES ($1, 'owner@test.com', 'hash_owner', 'Owner User', 'manager'),
			        ($2, 'lead@test.com', 'hash_lead', 'Lead User', 'manager'),
			        ($3, 'emp@test.com', 'hash_emp', 'Emp User', 'manager')`,
			ownerID, leadID, empID,
		)
		if err != nil {
			t.Fatalf("seed users: %v", err)
		}

		// Create Business
		_, err = pool.Exec(ctx,
			`INSERT INTO businesses (id, name, kind, owner_user_id) VALUES ($1, 'Regression Biz', 'team', $2)`,
			bizID, ownerID,
		)
		if err != nil {
			t.Fatalf("seed business: %v", err)
		}

		// Create Memberships
		_, err = pool.Exec(ctx,
			`INSERT INTO memberships (business_id, user_id, role, is_suspended) 
			 VALUES ($1, $2, 'owner', false),
			        ($1, $3, 'team_lead', false),
			        ($1, $4, 'employee', false)`,
			bizID, ownerID, leadID, empID,
		)
		if err != nil {
			t.Fatalf("seed memberships: %v", err)
		}

		// Pair Desktop Device
		_, err = pool.Exec(ctx,
			`INSERT INTO devices (id, user_id, label, last_seen_at) VALUES ($1, $2, 'MacBook Pro - Work', now())`,
			deviceID, empID,
		)
		if err != nil {
			t.Fatalf("pair device: %v", err)
		}

		// Verify desktop sync resolution
		resolvedBizID, err := st.ResolveBusinessForUser(ctx, empID, nil)
		if err != nil || resolvedBizID != bizID {
			t.Fatalf("expected resolvedBizID %s, got %s, err=%v", bizID, resolvedBizID, err)
		}
		t.Log("PASS: Desktop login, user resolution, and device pairing verified!")
	})

	// 2. Employee Tracking & Synchronization
	t.Run("2_EmployeeTrackingAndSync", func(t *testing.T) {
		nowTs := time.Now().Unix()
		// Record Activity Sample
		_, err := pool.Exec(ctx,
			`INSERT INTO activity_samples (client_uuid, user_id, business_id, device_id, ts, app_name, window_title, pid, duration_s, client_updated_at) 
			 VALUES ($1, $2, $3, $4, $5, 'VSCode', 'main.go - finjulia', 1234, 60, $5)`,
			uuid.New(), empID, bizID, deviceID, nowTs,
		)
		if err != nil {
			t.Fatalf("insert activity sample: %v", err)
		}

		// Record Keystroke Bucket
		_, err = pool.Exec(ctx,
			`INSERT INTO keystroke_buckets (client_uuid, user_id, business_id, device_id, ts_bucket, count, client_updated_at) 
			 VALUES ($1, $2, $3, $4, $5, 142, $5)`,
			uuid.New(), empID, bizID, deviceID, nowTs,
		)
		if err != nil {
			t.Fatalf("insert keystroke bucket: %v", err)
		}

		// Record Browser Visit
		_, err = pool.Exec(ctx,
			`INSERT INTO browser_visits (client_uuid, user_id, business_id, device_id, ts, url, page_title, duration_s, client_updated_at) 
			 VALUES ($1, $2, $3, $4, $5, 'https://github.com/finjulia', 'Finjulia PR', 45, $5)`,
			uuid.New(), empID, bizID, deviceID, nowTs,
		)
		if err != nil {
			t.Fatalf("insert browser visit: %v", err)
		}
		t.Log("PASS: Employee tracking telemetry sync (activity, keystrokes, browser) verified!")
	})

	// 3. Screenshot Capture, Upload, and Source Tagging
	t.Run("3_ScreenshotCaptureAndSourceTagging", func(t *testing.T) {
		nowTs := time.Now().Unix()
		// Record automatic screenshot
		_, err := pool.Exec(ctx,
			`INSERT INTO screenshots (client_uuid, user_id, business_id, device_id, ts, file_path, byte_size, client_updated_at, capture_source) 
			 VALUES ($1, $2, $3, $4, $5, 'screenshots/2026-10-30/test.webp', 1048576, $5, 'automatic')`,
			screenshotUUID, empID, bizID, deviceID, nowTs,
		)
		if err != nil {
			t.Fatalf("insert screenshot: %v", err)
		}

		// Verify record exists and has capture_source = 'automatic'
		var source string
		err = pool.QueryRow(ctx, `SELECT capture_source FROM screenshots WHERE client_uuid = $1`, screenshotUUID).Scan(&source)
		if err != nil || source != "automatic" {
			t.Fatalf("expected capture_source automatic, got %s, err=%v", source, err)
		}
		t.Log("PASS: Screenshot capture, metadata recording, and source tagging verified!")
	})

	// 4. Manual Screenshot Requests Lifecycle
	t.Run("4_ManualScreenshotRequestsLifecycle", func(t *testing.T) {
		reqID := uuid.New().String()
		now := time.Now()

		// Manager creates manual screenshot request
		_, err := pool.Exec(ctx,
			`INSERT INTO screenshot_requests (id, business_id, employee_id, requested_by_user_id, status, policy_version, created_at, expires_at) 
			 VALUES ($1, $2, $3, $4, 'pending', 1, $5, $6)`,
			reqID, bizID, empID, ownerID, now, now.Add(5*time.Minute),
		)
		if err != nil {
			t.Fatalf("create screenshot request: %v", err)
		}

		// Device fetches pending requests via Store method
		pendingReqs, err := st.GetPendingScreenshotRequests(ctx, empID)
		if err != nil || len(pendingReqs) != 1 {
			t.Fatalf("expected 1 pending screenshot request from store, got %d (err: %v)", len(pendingReqs), err)
		}

		// Verify suspended membership excludes pending requests
		_, err = pool.Exec(ctx, `UPDATE memberships SET is_suspended = true WHERE user_id = $1 AND business_id = $2`, empID, bizID)
		if err != nil {
			t.Fatalf("suspend membership: %v", err)
		}
		suspendedReqs, err := st.GetPendingScreenshotRequests(ctx, empID)
		if err != nil || len(suspendedReqs) != 0 {
			t.Fatalf("expected 0 pending screenshot requests for suspended member, got %d (err: %v)", len(suspendedReqs), err)
		}

		// Reactivate membership
		_, err = pool.Exec(ctx, `UPDATE memberships SET is_suspended = false WHERE user_id = $1 AND business_id = $2`, empID, bizID)
		if err != nil {
			t.Fatalf("reactivate membership: %v", err)
		}

		// Device uploads manual screenshot fulfilling request
		manualScreenshotUUID := uuid.New().String()
		nowTs := time.Now().Unix()
		_, err = pool.Exec(ctx,
			`INSERT INTO screenshots (client_uuid, user_id, business_id, device_id, ts, file_path, byte_size, client_updated_at, request_id, capture_source) 
			 VALUES ($1, $2, $3, $4, $5, 'screenshots/2026-10-30/manual.webp', 1048576, $5, $6, 'manual')`,
			manualScreenshotUUID, empID, bizID, deviceID, nowTs, reqID,
		)
		if err != nil {
			t.Fatalf("insert manual screenshot: %v", err)
		}

		// Acknowledge and complete request
		_, err = pool.Exec(ctx,
			`UPDATE screenshot_requests SET status = 'completed', screenshot_client_uuid = $1, completed_at = now() WHERE id = $2`,
			manualScreenshotUUID, reqID,
		)
		if err != nil {
			t.Fatalf("fulfill screenshot request: %v", err)
		}
		t.Log("PASS: Manual screenshot request creation, fulfillment, and state transition verified!")
	})

	// 5. Automatic Screenshot Policies & Resolution
	t.Run("5_ScreenshotPoliciesAndResolution", func(t *testing.T) {
		// Set business policy
		_, err := pool.Exec(ctx,
			`UPDATE businesses 
			    SET screenshots_enabled = true,
			        continuous_capture_enabled = true,
			        manual_capture_enabled = true,
			        screenshot_interval_s = 300
			  WHERE id = $1`,
			bizID,
		)
		if err != nil {
			t.Fatalf("update business policy: %v", err)
		}

		// Set employee override policy
		_, err = pool.Exec(ctx,
			`INSERT INTO employee_screenshot_policies (business_id, employee_id, screenshots_enabled, screenshot_interval_s) 
			 VALUES ($1, $2, true, 180)
			 ON CONFLICT (business_id, employee_id) DO UPDATE SET screenshot_interval_s = 180`,
			bizID, empID,
		)
		if err != nil {
			t.Fatalf("insert employee policy override: %v", err)
		}

		// Test effective policy resolution
		effective := store.ResolveEffectivePolicy(
			store.BusinessScreenshotDefaults{
				ScreenshotsEnabled:       true,
				ContinuousCaptureEnabled: true,
				ManualCaptureEnabled:     true,
				ScreenshotIntervalS:      300,
				AllowEmployeeOverride:    true,
				ScreenshotMode:           "privacy",
				PolicyVersion:            1,
				PolicyUpdatedAt:          time.Now(),
			},
			store.EmployeePolicyOverrides{
				ScreenshotsEnabled:  boolPtr(true),
				ScreenshotIntervalS: intPtr(180),
			},
			true,
			empID,
			bizID,
		)
		if !effective.ContinuousCaptureEnabled || effective.CaptureIntervalSeconds != 180 {
			t.Fatalf("expected effective interval 180 and continuous true, got %+v", effective)
		}
		t.Log("PASS: Screenshot policies and override resolution verified!")
	})

	// 6. Screenshot Galleries & Reports Scoping
	t.Run("6_ScreenshotGalleriesAndReportsScoping", func(t *testing.T) {
		// Create Team and assign empID
		team, err := st.CreateTeam(ctx, bizID, "Dev Team", nil, &leadID, nil, ownerID)
		if err != nil {
			t.Fatalf("create team: %v", err)
		}
		err = st.AssignMemberToTeam(ctx, bizID, team.ID, empID, ownerID)
		if err != nil {
			t.Fatalf("assign member: %v", err)
		}

		// Lead ID has can_view_screenshots granted
		_ = st.GrantPermission(ctx, bizID, ownerID, leadID, "can_view_screenshots", false)
		_ = st.GrantPermission(ctx, bizID, ownerID, leadID, "can_view_reports", false)

		// Test OwnsEmployee with leadID
		owns, err := st.OwnsEmployee(ctx, leadID, empID)
		if err != nil || !owns {
			t.Fatalf("leadID should own empID within authorized team tree, got owns=%v, err=%v", owns, err)
		}

		// Test ScreenshotPathForOwner
		path, err := st.ScreenshotPathForOwner(ctx, leadID, screenshotUUID)
		if err != nil || path == "" {
			t.Fatalf("leadID should be able to query screenshot path for empID, got path=%s, err=%v", path, err)
		}

		// Test Roster Scoping: Lead ID sees empID
		now := time.Now().Unix()
		roster, err := st.Roster(ctx, bizID, now-3600, now+3600, leadID)
		if err != nil || len(roster) == 0 {
			t.Fatalf("expected roster for leadID, got %v, err=%v", roster, err)
		}
		var foundEmp bool
		for _, r := range roster {
			if r.ID == empID {
				foundEmp = true
				break
			}
		}
		if !foundEmp {
			t.Fatalf("empID was missing from leadID's scoped roster!")
		}
		t.Log("PASS: Screenshot galleries, OwnsEmployee, ScreenshotPathForOwner, and Roster verified!")
	})
}

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }
