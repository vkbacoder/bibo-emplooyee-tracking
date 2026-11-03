package store_test

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"ctracking/backend/internal/db"
	"ctracking/backend/internal/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

const testDSN = "postgres://postgres@127.0.0.1:54339/ctracking_test?sslmode=disable"

// TestMigrationGooseUpDown verifies Goose Up to 13, Up to 14, Down to 13, and Up to 14.
func TestMigrationGooseUpDown(t *testing.T) {
	sqlDB, err := sql.Open("pgx", testDSN)
	if err != nil {
		t.Fatalf("failed to open sql db: %v", err)
	}
	defer sqlDB.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("set goose dialect: %v", err)
	}

	t.Log("Applying all migrations up to 00014...")
	if err := db.Migrate(testDSN); err != nil {
		t.Fatalf("db.Migrate failed: %v", err)
	}

	// Verify tables exist
	var exists bool
	err = sqlDB.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'teams')`).Scan(&exists)
	if err != nil || !exists {
		t.Fatalf("expected 'teams' table to exist after migrate, got %v, exists=%v", err, exists)
	}
	err = sqlDB.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'organization_audit_logs')`).Scan(&exists)
	if err != nil || !exists {
		t.Fatalf("expected 'organization_audit_logs' table to exist, got %v", err)
	}

	t.Log("Testing Goose Down (rolling back migration 00014)...")
	// Down 1 migration
	currentVer, err := goose.GetDBVersion(sqlDB)
	if err != nil {
		t.Fatalf("get db version: %v", err)
	}
	t.Logf("Current migration version before down: %d", currentVer)

	if err := goose.Down(sqlDB, "migrations"); err != nil {
		t.Fatalf("goose.Down failed: %v", err)
	}

	// Verify teams table dropped
	err = sqlDB.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'teams')`).Scan(&exists)
	if err != nil || exists {
		t.Fatalf("expected 'teams' table to be dropped after goose Down, got exists=%v", exists)
	}

	t.Log("Testing Goose Up (re-applying migration 00014)...")
	if err := db.Migrate(testDSN); err != nil {
		t.Fatalf("re-applying goose.Up failed: %v", err)
	}

	err = sqlDB.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'teams')`).Scan(&exists)
	if err != nil || !exists {
		t.Fatalf("expected 'teams' table to be re-created, got %v", err)
	}
	t.Log("Goose Up & Down migration cycle PASSED successfully!")
}

// TestComprehensiveRbacAndSecurityAudit executes all validation tests.
func TestComprehensiveRbacAndSecurityAudit(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("failed to connect to test db pool: %v", err)
	}
	defer pool.Close()

	st := store.New(pool)

	// Clean tables before running
	_, _ = pool.Exec(ctx, `TRUNCATE businesses, users CASCADE`)

	// -------------------------------------------------------------
	// Setup Test Entities: Business A and Business B
	// -------------------------------------------------------------
	bizA := uuid.New().String()
	bizB := uuid.New().String()

	ownerA := uuid.New().String()
	leadA1 := uuid.New().String()
	leadA2 := uuid.New().String()
	empA1 := uuid.New().String()
	empA2 := uuid.New().String()

	ownerB := uuid.New().String()
	leadB1 := uuid.New().String()
	empB1 := uuid.New().String()

	// Seed Users
	users := []struct {
		id    string
		email string
		name  string
	}{
		{ownerA, "ownerA@biz.com", "Owner A"},
		{leadA1, "leadA1@biz.com", "Lead A1 (Eng)"},
		{leadA2, "leadA2@biz.com", "Lead A2 (Sub-Eng)"},
		{empA1, "empA1@biz.com", "Emp A1"},
		{empA2, "empA2@biz.com", "Emp A2"},
		{ownerB, "ownerB@biz.com", "Owner B"},
		{leadB1, "leadB1@biz.com", "Lead B1"},
		{empB1, "empB1@biz.com", "Emp B1"},
	}
	for _, u := range users {
		userEmail := u.id + "@biz.com"
		_, err := pool.Exec(ctx,
			`INSERT INTO users (id, email, password_hash, display_name, account_type) 
			 VALUES ($1, $2, 'hash123', $3, 'manager')`,
			u.id, userEmail, u.name,
		)
		if err != nil {
			t.Fatalf("seed user %s: %v", u.name, err)
		}
	}

	// Seed Businesses
	_, err = pool.Exec(ctx, `INSERT INTO businesses (id, name, kind, owner_user_id) VALUES ($1, 'Business A', 'team', $2)`, bizA, ownerA)
	if err != nil {
		t.Fatalf("seed bizA: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO businesses (id, name, kind, owner_user_id) VALUES ($1, 'Business B', 'team', $2)`, bizB, ownerB)
	if err != nil {
		t.Fatalf("seed bizB: %v", err)
	}

	// Seed Memberships
	members := []struct {
		bizID  string
		userID string
		role   string
	}{
		{bizA, ownerA, "owner"},
		{bizA, leadA1, "team_lead"},
		{bizA, leadA2, "team_lead"},
		{bizA, empA1, "employee"},
		{bizA, empA2, "employee"},
		{bizB, ownerB, "owner"},
		{bizB, leadB1, "team_lead"},
		{bizB, empB1, "employee"},
	}
	for _, m := range members {
		_, err := pool.Exec(ctx,
			`INSERT INTO memberships (user_id, business_id, role) VALUES ($1, $2, $3)`,
			m.userID, m.bizID, m.role,
		)
		if err != nil {
			t.Fatalf("seed membership %s in %s: %v", m.userID, m.bizID, err)
		}
	}

	// -------------------------------------------------------------
	// 1. Cross-Business Isolation Tests
	// -------------------------------------------------------------
	t.Run("CrossBusiness_ParentTeamIntegrity", func(t *testing.T) {
		// Create Team in Biz B
		teamB, err := st.CreateTeam(ctx, bizB, "Biz B Team", nil, nil, nil, ownerB)
		if err != nil {
			t.Fatalf("create team in bizB: %v", err)
		}

		// Attempt to create Team in Biz A with Parent from Biz B
		_, err = st.CreateTeam(ctx, bizA, "Biz A Illegal Team", nil, nil, &teamB.ID, ownerA)
		if err == nil {
			t.Fatalf("expected error assigning parent team from different business, but got nil")
		}
		t.Logf("PASS: Cross-business parent team assignment blocked: %v", err)
	})

	t.Run("CrossBusiness_LeadIntegrity", func(t *testing.T) {
		// Attempt to assign user from Biz B as lead in Biz A
		_, err := st.CreateTeam(ctx, bizA, "Biz A Illegal Lead", nil, &leadB1, nil, ownerA)
		if err == nil {
			t.Fatalf("expected error assigning lead from different business, but got nil")
		}
		t.Logf("PASS: Cross-business team lead assignment blocked: %v", err)
	})

	// -------------------------------------------------------------
	// 2. Hierarchy Tree & Concurrent Cycle Prevention
	// -------------------------------------------------------------
	var teamEngID, teamSubEngID, teamQaid string
	t.Run("Hierarchy_CyclePrevention", func(t *testing.T) {
		teamEng, err := st.CreateTeam(ctx, bizA, "Engineering", nil, &leadA1, nil, ownerA)
		if err != nil {
			t.Fatalf("create teamEng: %v", err)
		}
		teamEngID = teamEng.ID

		teamSubEng, err := st.CreateTeam(ctx, bizA, "Sub-Eng", nil, &leadA2, &teamEng.ID, ownerA)
		if err != nil {
			t.Fatalf("create teamSubEng: %v", err)
		}
		teamSubEngID = teamSubEng.ID

		teamQA, err := st.CreateTeam(ctx, bizA, "QA", nil, nil, &teamSubEng.ID, ownerA)
		if err != nil {
			t.Fatalf("create teamQA: %v", err)
		}
		teamQaid = teamQA.ID

		// Direct cycle: teamEng parent set to teamSubEng (which is already child of teamEng)
		_, err = st.UpdateTeam(ctx, bizA, teamEngID, "Engineering", nil, &leadA1, &teamSubEngID, ownerA)
		if err == nil {
			t.Fatalf("expected cycle error setting parent of ancestor, got nil")
		}
		t.Logf("PASS: Immediate cycle blocked: %v", err)

		// Indirect cycle: teamEng parent set to teamQA (descendant of sub-eng)
		_, err = st.UpdateTeam(ctx, bizA, teamEngID, "Engineering", nil, &leadA1, &teamQaid, ownerA)
		if err == nil {
			t.Fatalf("expected indirect cycle error, got nil")
		}
		t.Logf("PASS: Deep transitive cycle blocked: %v", err)

		// Self-parenting check
		_, err = st.UpdateTeam(ctx, bizA, teamEngID, "Engineering", nil, &leadA1, &teamEngID, ownerA)
		if err == nil {
			t.Fatalf("expected self-parenting error, got nil")
		}
		t.Logf("PASS: Self-parenting blocked: %v", err)
	})

	t.Run("Hierarchy_ConcurrentAdvisoryLockCyclePrevention", func(t *testing.T) {
		// Team X and Team Y
		txTeamX, err := st.CreateTeam(ctx, bizA, "Concurrent Team X", nil, nil, nil, ownerA)
		if err != nil {
			t.Fatalf("create Team X: %v", err)
		}
		txTeamY, err := st.CreateTeam(ctx, bizA, "Concurrent Team Y", nil, nil, nil, ownerA)
		if err != nil {
			t.Fatalf("create Team Y: %v", err)
		}

		// Two concurrent transactions attempting cross-parenting simultaneously:
		// Tx 1: Y -> X
		// Tx 2: X -> Y
		var wg sync.WaitGroup
		errs := make([]error, 2)
		wg.Add(2)

		go func() {
			defer wg.Done()
			_, errs[0] = st.UpdateTeam(ctx, bizA, txTeamY.ID, "Concurrent Team Y", nil, nil, &txTeamX.ID, ownerA)
		}()

		go func() {
			defer wg.Done()
			_, errs[1] = st.UpdateTeam(ctx, bizA, txTeamX.ID, "Concurrent Team X", nil, nil, &txTeamY.ID, ownerA)
		}()

		wg.Wait()

		// Exactly one must succeed and one must fail with cycle detected
		successCount := 0
		failCount := 0
		for _, e := range errs {
			if e == nil {
				successCount++
			} else {
				failCount++
				t.Logf("Concurrent race caught cycle: %v", e)
			}
		}
		if successCount != 1 || failCount != 1 {
			t.Fatalf("expected exactly 1 success and 1 cycle detection failure, got success=%d, fail=%d", successCount, failCount)
		}
		t.Log("PASS: Concurrency-safe advisory locked cycle prevention verified!")
	})

	// -------------------------------------------------------------
	// 3. Team Member Assignment & Single Primary Team Constraint
	// -------------------------------------------------------------
	t.Run("TeamMember_AssignmentAndConstraint", func(t *testing.T) {
		err := st.AssignMemberToTeam(ctx, bizA, teamEngID, empA1, ownerA)
		if err != nil {
			t.Fatalf("assign empA1: %v", err)
		}

		// Attempt to assign empA1 to teamSubEng without reassigning
		// Note: uq_team_members_user_biz ensures one primary team per user in a business
		_, err = pool.Exec(ctx, `INSERT INTO team_members (business_id, team_id, user_id) VALUES ($1, $2, $3)`, bizA, teamSubEngID, empA1)
		if err == nil {
			t.Fatalf("expected duplicate primary team assignment error, got nil")
		}
		t.Logf("PASS: Single primary team per user in business enforced: %v", err)
	})

	// -------------------------------------------------------------
	// 4. Role Authorization & Scoping
	// -------------------------------------------------------------
	t.Run("RoleAuthorization_Scoping", func(t *testing.T) {
		// Owner has org-wide authority
		teamIDs, isOrgWide, err := st.GetAuthorizedTeamIDs(ctx, bizA, ownerA)
		if err != nil || !isOrgWide || teamIDs != nil {
			t.Fatalf("owner should have org-wide authority, got isOrgWide=%v, teamIDs=%v", isOrgWide, teamIDs)
		}

		// Lead A1 leads teamEng, which has child teamSubEng and grandchild teamQA
		leadA1Teams, isOrgWide, err := st.GetAuthorizedTeamIDs(ctx, bizA, leadA1)
		if err != nil || isOrgWide {
			t.Fatalf("leadA1 should not be org-wide")
		}
		// Should contain teamEngID, teamSubEngID, teamQaid
		expected := map[string]bool{teamEngID: true, teamSubEngID: true, teamQaid: true}
		if len(leadA1Teams) < 3 {
			t.Fatalf("leadA1 should see subtree of teams, got %v", leadA1Teams)
		}
		for _, id := range leadA1Teams {
			delete(expected, id)
		}
		if len(expected) > 0 {
			t.Fatalf("leadA1 missed subtree teams: %v", expected)
		}

		// Lead A2 leads only teamSubEng, so subtree should NOT include teamEngID
		leadA2Teams, _, err := st.GetAuthorizedTeamIDs(ctx, bizA, leadA2)
		if err != nil {
			t.Fatalf("leadA2 teams: %v", err)
		}
		for _, id := range leadA2Teams {
			if id == teamEngID {
				t.Fatalf("leadA2 subtree illegally contains parent teamEngID!")
			}
		}

		// Employee has empty authorized teams
		empTeams, isOrgWide, err := st.GetAuthorizedTeamIDs(ctx, bizA, empA1)
		if err != nil || isOrgWide || len(empTeams) != 0 {
			t.Fatalf("employee should have 0 authorized teams, got %v", empTeams)
		}
		t.Log("PASS: Subtree team scoping for Owner, Team Lead and Employee verified!")
	})

	// -------------------------------------------------------------
	// 5. Granular RBAC, Delegation & Independent Grant Isolation
	// -------------------------------------------------------------
	t.Run("Permission_DelegationAndCascadeRevocation", func(t *testing.T) {
		// 1. Owner grants delegable 'can_view_reports' to Lead A1
		err := st.GrantPermission(ctx, bizA, ownerA, leadA1, "can_view_reports", true)
		if err != nil {
			t.Fatalf("owner grant can_view_reports to leadA1: %v", err)
		}

		// 2. Lead A1 delegates 'can_view_reports' to Lead A2 (with delegable=true)
		err = st.GrantPermission(ctx, bizA, leadA1, leadA2, "can_view_reports", true)
		if err != nil {
			t.Fatalf("leadA1 delegate can_view_reports to leadA2: %v", err)
		}

		// 3. Lead A2 delegates 'can_view_reports' to Emp A2 (delegable=false)
		err = st.GrantPermission(ctx, bizA, leadA2, empA2, "can_view_reports", false)
		if err != nil {
			t.Fatalf("leadA2 delegate can_view_reports to empA2: %v", err)
		}

		// 4. Owner independently grants 'can_view_screenshots' directly to Emp A2
		err = st.GrantPermission(ctx, bizA, ownerA, empA2, "can_view_screenshots", false)
		if err != nil {
			t.Fatalf("owner grant can_view_screenshots to empA2: %v", err)
		}

		// 5. Verify Emp A2 has both
		hasReports, err := st.HasPermission(ctx, bizA, empA2, "can_view_reports")
		if err != nil || !hasReports {
			t.Fatalf("empA2 should have can_view_reports")
		}
		hasScreenshots, err := st.HasPermission(ctx, bizA, empA2, "can_view_screenshots")
		if err != nil || !hasScreenshots {
			t.Fatalf("empA2 should have can_view_screenshots")
		}

		// 6. Test unauthorized delegation: Lead A2 attempts to delegate 'can_manage_team' without holding it
		err = st.GrantPermission(ctx, bizA, leadA2, empA2, "can_manage_team", false)
		if err != store.ErrDelegationDisallowed {
			t.Fatalf("expected ErrDelegationDisallowed for unheld permission, got %v", err)
		}

		// 7. Revoke 'can_view_reports' from Lead A1
		err = st.RevokePermission(ctx, bizA, ownerA, leadA1, "can_view_reports")
		if err != nil {
			t.Fatalf("revoke permission from leadA1: %v", err)
		}

		// 8. Verify cascade revocation:
		// Lead A1 should NOT have can_view_reports
		has, _ := st.HasPermission(ctx, bizA, leadA1, "can_view_reports")
		if has {
			t.Fatalf("leadA1 should have lost can_view_reports")
		}
		// Lead A2 should have lost can_view_reports (downstream cascade)
		has, _ = st.HasPermission(ctx, bizA, leadA2, "can_view_reports")
		if has {
			t.Fatalf("leadA2 should have lost can_view_reports via cascade")
		}
		// Emp A2 should have lost can_view_reports (downstream cascade)
		has, _ = st.HasPermission(ctx, bizA, empA2, "can_view_reports")
		if has {
			t.Fatalf("empA2 should have lost can_view_reports via cascade")
		}

		// 9. Verify Independent Grant Isolation:
		// Emp A2's 'can_view_screenshots' was granted directly by Owner A and must still exist!
		hasScreenshots, _ = st.HasPermission(ctx, bizA, empA2, "can_view_screenshots")
		if !hasScreenshots {
			t.Fatalf("empA2 independently granted can_view_screenshots was improperly revoked!")
		}
		t.Log("PASS: Permission delegation, cascade revocation, and independent grant preservation verified!")
	})

	// -------------------------------------------------------------
	// 6. Business Suspension & Multi-Tenant Isolation
	// -------------------------------------------------------------
	t.Run("Suspension_BusinessScopedIsolation", func(t *testing.T) {
		// Suspend Emp A1 in Biz A
		err := st.SetMemberSuspended(ctx, bizA, ownerA, empA1, true, nil)
		if err != nil {
			t.Fatalf("suspend empA1: %v", err)
		}

		// Verify role check shows suspended
		_, isSuspended, err := st.GetMemberRole(ctx, bizA, empA1)
		if err != nil || !isSuspended {
			t.Fatalf("expected empA1 to be suspended in bizA")
		}

		// HasPermission must return false when suspended
		has, _ := st.HasPermission(ctx, bizA, empA1, "can_view_team")
		if has {
			t.Fatalf("suspended member must not possess permissions")
		}

		// Trigger check_team_member_not_suspended must reject team assignment
		err = st.AssignMemberToTeam(ctx, bizA, teamQaid, empA1, ownerA)
		if err == nil {
			t.Fatalf("expected trigger rejection assigning suspended member to team, got nil")
		}

		// Add Emp A1 to Biz B (cross-business account reuse)
		_, err = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role) VALUES ($1, $2, 'employee')`, bizB, empA1)
		if err != nil {
			t.Fatalf("add empA1 to bizB: %v", err)
		}

		// Verify Emp A1 is NOT suspended in Biz B!
		_, isSuspendedB, err := st.GetMemberRole(ctx, bizB, empA1)
		if err != nil || isSuspendedB {
			t.Fatalf("empA1 should NOT be suspended in bizB, suspension must be strictly business-scoped!")
		}

		// Reactivate Emp A1 in Biz A
		err = st.SetMemberSuspended(ctx, bizA, ownerA, empA1, false, nil)
		if err != nil {
			t.Fatalf("reactivate empA1: %v", err)
		}
		_, isSuspended, _ = st.GetMemberRole(ctx, bizA, empA1)
		if isSuspended {
			t.Fatalf("empA1 should be reactivated")
		}
		t.Log("PASS: Business-scoped suspension and cross-business isolation verified!")
	})

	// -------------------------------------------------------------
	// 7. Member Removal Workflow (8-Step Transaction) & Audit Log Immutability
	// -------------------------------------------------------------
	t.Run("MemberRemoval_WorkflowAndAuditImmutability", func(t *testing.T) {
		// Lead A1 leads teamEng. Attempting to remove Lead A1 without replacement must fail!
		err := st.RemoveMember(ctx, bizA, ownerA, leadA1, nil, false)
		if err != store.ErrReplacementRequired {
			t.Fatalf("expected ErrReplacementRequired removing team lead, got %v", err)
		}

		// Remove with replacement Lead A2
		err = st.RemoveMember(ctx, bizA, ownerA, leadA1, &leadA2, false)
		if err != nil {
			t.Fatalf("remove member with replacement: %v", err)
		}

		// Check teamEng lead is now Lead A2
		team, err := st.GetTeam(ctx, bizA, teamEngID)
		if err != nil || team.LeadUserID == nil || *team.LeadUserID != leadA2 {
			t.Fatalf("expected teamEng lead to be reassigned to leadA2, got %v", team)
		}

		// Check Lead A1 membership in Biz A is deleted
		_, _, err = st.GetMemberRole(ctx, bizA, leadA1)
		if err != store.ErrNotFound {
			t.Fatalf("expected ErrNotFound for removed member, got %v", err)
		}

		// Check Lead A1 user row in `users` STILL EXISTS (Identity Preserved)
		var userEmail string
		err = pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, leadA1).Scan(&userEmail)
		if err != nil || userEmail != leadA1+"@biz.com" {
			t.Fatalf("global user account was deleted! Identity must be preserved: %v", err)
		}

		// Check Audit Logs: must contain member.removed entry
		logs, err := st.ListAuditLogs(ctx, bizA, 50, 0)
		if err != nil || len(logs) == 0 {
			t.Fatalf("expected audit log entries, got %v", err)
		}
		var foundRemoval bool
		var auditLogID string
		for _, l := range logs {
			if l.Action == "member.removed" && l.TargetUserID != nil && *l.TargetUserID == leadA1 {
				foundRemoval = true
				auditLogID = l.ID
				break
			}
		}
		if !foundRemoval {
			t.Fatalf("did not find audit log entry for member.removed")
		}

		// Test Audit Log Immutability: UPDATE must be blocked by trigger
		_, err = pool.Exec(ctx, `UPDATE organization_audit_logs SET action = 'tampered' WHERE id = $1`, auditLogID)
		if err == nil {
			t.Fatalf("expected trigger to block UPDATE on audit logs, but it succeeded!")
		}
		t.Logf("PASS: Audit log UPDATE blocked: %v", err)

		// Test Audit Log Immutability: DELETE must be blocked by trigger
		_, err = pool.Exec(ctx, `DELETE FROM organization_audit_logs WHERE id = $1`, auditLogID)
		if err == nil {
			t.Fatalf("expected trigger to block DELETE on audit logs, but it succeeded!")
		}
		t.Logf("PASS: Audit log DELETE blocked: %v", err)
		t.Log("PASS: 8-Step member removal workflow and audit log immutability verified!")
	})

	// -------------------------------------------------------------
	// 8. Raw Membership Deletion Trigger Cleanup
	// -------------------------------------------------------------
	t.Run("MembershipDeletion_TriggerCleanup", func(t *testing.T) {
		// Assign Lead A2 to Team Sub-Eng
		_, err := pool.Exec(ctx, `UPDATE teams SET lead_user_id = $1 WHERE id = $2`, leadA2, teamSubEngID)
		if err != nil {
			t.Fatalf("set lead: %v", err)
		}

		// Grant a delegated permission to Lead A2
		_ = st.GrantPermission(ctx, bizA, ownerA, leadA2, "can_manage_team", false)

		// Execute raw SQL DELETE on memberships directly (bypassing Go workflow)
		_, err = pool.Exec(ctx, `DELETE FROM memberships WHERE business_id = $1 AND user_id = $2`, bizA, leadA2)
		if err != nil {
			t.Fatalf("raw membership delete: %v", err)
		}

		// Trigger cleanup_membership_on_delete MUST have cleared teams.lead_user_id
		var currentLead *string
		err = pool.QueryRow(ctx, `SELECT lead_user_id FROM teams WHERE id = $1`, teamSubEngID).Scan(&currentLead)
		if err != nil {
			t.Fatalf("query team lead: %v", err)
		}
		if currentLead != nil {
			t.Fatalf("expected teams.lead_user_id to be NULL after membership delete, got %v", *currentLead)
		}

		// Permissions must have been cascade deleted
		perms, err := st.ListUserPermissions(ctx, bizA, leadA2)
		if err != nil || len(perms) != 0 {
			t.Fatalf("expected permissions to be deleted, got %v", perms)
		}
		t.Log("PASS: Raw database trigger cleanup_membership_on_delete verified!")
	})
}
