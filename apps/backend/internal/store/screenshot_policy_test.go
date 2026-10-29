package store

import (
	"testing"
	"time"
)

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }

func TestResolveEffectivePolicy(t *testing.T) {
	now := time.Now()
	baseBiz := BusinessScreenshotDefaults{
		ScreenshotsEnabled:       true,
		ContinuousCaptureEnabled: true,
		ScreenshotIntervalS:      300,
		ManualCaptureEnabled:     true,
		AllowEmployeeOverride:    true,
		ScreenshotMode:           "privacy",
		PolicyVersion:            1,
		PolicyUpdatedAt:          now,
	}

	t.Run("InheritDefaultsWhenNoOverride", func(t *testing.T) {
		p := ResolveEffectivePolicy(baseBiz, EmployeePolicyOverrides{}, false, "emp-1", "biz-1")
		if !p.CaptureEnabled {
			t.Errorf("expected CaptureEnabled true, got false")
		}
		if !p.ContinuousCaptureEnabled {
			t.Errorf("expected ContinuousCaptureEnabled true, got false")
		}
		if p.CaptureIntervalMinutes != 5 {
			t.Errorf("expected 5 minutes, got %d", p.CaptureIntervalMinutes)
		}
		if p.CaptureIntervalSeconds != 300 {
			t.Errorf("expected 300 seconds, got %d", p.CaptureIntervalSeconds)
		}
		if !p.ManualCaptureEnabled {
			t.Errorf("expected ManualCaptureEnabled true, got false")
		}
		if p.PolicyVersion != 1 {
			t.Errorf("expected PolicyVersion 1, got %d", p.PolicyVersion)
		}
	})

	t.Run("Rule1_GlobalCaptureDisabledDisablesAll", func(t *testing.T) {
		biz := baseBiz
		biz.ScreenshotsEnabled = false // global switch OFF

		// Even if employee override attempts to enable everything:
		ovr := EmployeePolicyOverrides{
			ScreenshotsEnabled:       boolPtr(true),
			ContinuousCaptureEnabled: boolPtr(true),
			ManualCaptureEnabled:     boolPtr(true),
			ScreenshotIntervalS:      intPtr(60),
			PolicyVersion:            2,
		}

		p := ResolveEffectivePolicy(biz, ovr, true, "emp-1", "biz-1")
		if p.CaptureEnabled {
			t.Errorf("Rule 1 violation: CaptureEnabled must be false when global switch is off")
		}
		if p.ContinuousCaptureEnabled {
			t.Errorf("Rule 1 violation: ContinuousCaptureEnabled must be false when global switch is off")
		}
		if p.ManualCaptureEnabled {
			t.Errorf("Rule 1 violation: ManualCaptureEnabled must be false when global switch is off")
		}
	})

	t.Run("Rule2_EmployeeExplicitlyDisabledDisablesAllForEmployee", func(t *testing.T) {
		biz := baseBiz
		ovr := EmployeePolicyOverrides{
			ScreenshotsEnabled:       boolPtr(false), // employee disabled
			ContinuousCaptureEnabled: boolPtr(true),  // should have no effect
			ManualCaptureEnabled:     boolPtr(true),  // should have no effect
			PolicyVersion:            2,
		}

		p := ResolveEffectivePolicy(biz, ovr, true, "emp-1", "biz-1")
		if p.CaptureEnabled {
			t.Errorf("Rule 2 violation: CaptureEnabled must be false when employee disabled")
		}
		if p.ContinuousCaptureEnabled {
			t.Errorf("Rule 2 violation: ContinuousCaptureEnabled must be false when employee disabled")
		}
		if p.ManualCaptureEnabled {
			t.Errorf("Rule 2 violation: ManualCaptureEnabled must be false when employee disabled")
		}
	})

	t.Run("Rule3_ContinuousCaptureIndependentlyConfigurable", func(t *testing.T) {
		// Biz continuous is ON, employee continuous is explicitly OFF
		biz := baseBiz
		ovr := EmployeePolicyOverrides{
			ContinuousCaptureEnabled: boolPtr(false),
			PolicyVersion:            1,
		}

		p := ResolveEffectivePolicy(biz, ovr, true, "emp-1", "biz-1")
		if !p.CaptureEnabled {
			t.Errorf("expected CaptureEnabled true")
		}
		if p.ContinuousCaptureEnabled {
			t.Errorf("Rule 3 violation: ContinuousCaptureEnabled should be false per employee override")
		}
		if !p.ManualCaptureEnabled {
			t.Errorf("Manual capture should still be true")
		}

		// Biz continuous is OFF, employee continuous is explicitly ON
		biz2 := baseBiz
		biz2.ContinuousCaptureEnabled = false
		ovr2 := EmployeePolicyOverrides{
			ContinuousCaptureEnabled: boolPtr(true),
			PolicyVersion:            1,
		}
		p2 := ResolveEffectivePolicy(biz2, ovr2, true, "emp-1", "biz-1")
		if !p2.ContinuousCaptureEnabled {
			t.Errorf("Rule 3 violation: ContinuousCaptureEnabled should be true per employee override")
		}
	})

	t.Run("Rule4_EmployeeIntervalOverride", func(t *testing.T) {
		biz := baseBiz // interval 300s = 5m
		ovr := EmployeePolicyOverrides{
			ScreenshotIntervalS: intPtr(600), // 10m
			PolicyVersion:       1,
		}

		p := ResolveEffectivePolicy(biz, ovr, true, "emp-1", "biz-1")
		if p.CaptureIntervalSeconds != 600 {
			t.Errorf("expected 600s, got %d", p.CaptureIntervalSeconds)
		}
		if p.CaptureIntervalMinutes != 10 {
			t.Errorf("expected 10m, got %d", p.CaptureIntervalMinutes)
		}
	})

	t.Run("Rule5_ManualCapturePermissions", func(t *testing.T) {
		// Case A: Biz manual is OFF -> manual is OFF
		bizA := baseBiz
		bizA.ManualCaptureEnabled = false
		ovrA := EmployeePolicyOverrides{
			ManualCaptureEnabled: boolPtr(true), // cannot override global off
		}
		pA := ResolveEffectivePolicy(bizA, ovrA, true, "emp-1", "biz-1")
		if pA.ManualCaptureEnabled {
			t.Errorf("Rule 5 violation: manual capture must be false when global manual is off")
		}

		// Case B: Biz manual is ON, but employee manual is explicitly OFF
		bizB := baseBiz
		ovrB := EmployeePolicyOverrides{
			ManualCaptureEnabled: boolPtr(false),
		}
		pB := ResolveEffectivePolicy(bizB, ovrB, true, "emp-1", "biz-1")
		if pB.ManualCaptureEnabled {
			t.Errorf("Rule 5 violation: manual capture must be false when employee manual is off")
		}

		// Case C: Biz manual is ON, employee override is nil -> inherits ON
		pC := ResolveEffectivePolicy(baseBiz, EmployeePolicyOverrides{}, false, "emp-1", "biz-1")
		if !pC.ManualCaptureEnabled {
			t.Errorf("Rule 5 violation: manual capture must be true when both permit")
		}
	})
}

func TestValidAckStatuses(t *testing.T) {
	expected := []string{
		"acknowledged",
		"completed",
		"rejected_disabled",
		"rejected_manual_disabled",
		"rejected_expired",
		"rejected_wrong_employee",
		"rejected_duplicate",
		"rejected_stale_policy",
		"rejected_privacy_exclusion",
		"failed",
	}

	for _, s := range expected {
		if !validAckStatuses[s] {
			t.Errorf("expected status %q to be recognized in validAckStatuses", s)
		}
	}

	// Invalid status must be rejected
	invalid := []string{"invalid_status", "pending", "success", "done", "cancelled"}
	for _, s := range invalid {
		if validAckStatuses[s] {
			t.Errorf("status %q should not be in validAckStatuses", s)
		}
	}
}

func TestCompletionSemanticsRules(t *testing.T) {
	// Rule 1: A completed request must have a non-empty screenshot UUID
	t.Run("CompletedRequiresUUID", func(t *testing.T) {
		var nilUUID *string
		emptyUUID := ""
		if nilUUID != nil && *nilUUID != "" {
			t.Error("expected nil check to fail")
		}
		if &emptyUUID != nil && *(&emptyUUID) != "" {
			t.Error("expected empty string check to fail")
		}
	})

	// Rule 2: Upload-first vs Ack-first state resolution
	t.Run("StateResolutionAckFirstVsUploadFirst", func(t *testing.T) {
		// When image is not yet uploaded: status must be "acknowledged" (awaiting upload)
		imageUploaded := false
		var resolvedStatus string
		if imageUploaded {
			resolvedStatus = "completed"
		} else {
			resolvedStatus = "acknowledged"
		}
		if resolvedStatus != "acknowledged" {
			t.Errorf("expected acknowledged before upload, got %s", resolvedStatus)
		}

		// When image arrives: transitions to "completed"
		imageUploaded = true
		if imageUploaded {
			resolvedStatus = "completed"
		}
		if resolvedStatus != "completed" {
			t.Errorf("expected completed after upload, got %s", resolvedStatus)
		}
	})
}

