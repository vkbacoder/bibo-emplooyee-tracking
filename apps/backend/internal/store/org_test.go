package store

import (
	"testing"
)

func TestValidPermissions(t *testing.T) {
	expectedPermissions := []string{
		"can_view_team",
		"can_manage_employees",
		"can_create_team_leads",
		"can_assign_employees",
		"can_manage_screenshot_policy",
		"can_view_screenshots",
		"can_view_reports",
		"can_manage_team",
		"can_view_audit_logs",
	}

	if len(ValidPermissions) != 9 {
		t.Fatalf("expected exactly 9 permissions, got %d", len(ValidPermissions))
	}

	for _, perm := range expectedPermissions {
		if !ValidPermissions[perm] {
			t.Errorf("missing expected permission: %s", perm)
		}
	}

	invalidPerms := []string{"super_admin", "can_delete_database", "random_perm"}
	for _, inv := range invalidPerms {
		if ValidPermissions[inv] {
			t.Errorf("expected invalid permission %s to be false", inv)
		}
	}
}

func TestOrgRoles(t *testing.T) {
	if len(ValidRoles) != 3 {
		t.Fatalf("expected exactly 3 roles, got %d", len(ValidRoles))
	}

	if !ValidRoles[RoleOwner] || !ValidRoles[RoleTeamLead] || !ValidRoles[RoleEmployee] {
		t.Errorf("ValidRoles must include RoleOwner (%s), RoleTeamLead (%s), RoleEmployee (%s)", RoleOwner, RoleTeamLead, RoleEmployee)
	}

	if ValidRoles["admin"] {
		t.Errorf("'admin' is not a valid membership role; it must be rejected")
	}
}

