package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ctracking/backend/internal/auth"
	"ctracking/backend/internal/filestore"
	"ctracking/backend/internal/handlers"
	"ctracking/backend/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testDSN = "postgres://postgres@127.0.0.1:5432/ctracking_test?sslmode=disable"

func setupTestRouter(st *store.Store, authMgr *auth.Manager, optionalFiles ...*filestore.Store) *gin.Engine {
	var files *filestore.Store
	if len(optionalFiles) > 0 {
		files = optionalFiles[0]
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())

	orgH := handlers.NewOrgHandler(st)
	policyH := handlers.NewScreenshotPolicyHandler(st)
	reportsH := handlers.NewReportsHandler(st, files)

	authed := r.Group("/v1", authMgr.Required())
	{
		authed.GET("/screenshot-policy", policyH.GetScreenshotPolicy)
		authed.POST("/screenshot-policy/ack", policyH.AckScreenshotPolicy)

		// Standard routes matching server.go
		biz := authed.Group("/businesses/:id")
		{
			biz.GET("/my-access", orgH.MyAccess)
			biz.GET("/members", orgH.ListMembers)
			biz.GET("/teams", orgH.ListTeams)
			biz.POST("/teams", orgH.CreateTeam)
			biz.GET("/teams/:team_id", orgH.GetTeam)
			biz.PUT("/teams/:team_id", orgH.UpdateTeam)
			biz.DELETE("/teams/:team_id", orgH.DeleteTeam)
			biz.GET("/teams/:team_id/members", orgH.ListTeamMembers)
			biz.POST("/teams/:team_id/members", orgH.AssignTeamMember)
			biz.DELETE("/teams/:team_id/members/:user_id", orgH.RemoveTeamMember)

			biz.POST("/members/:user_id/promote", orgH.PromoteMember)
			biz.POST("/members/:user_id/demote", orgH.DemoteMember)
			biz.POST("/members/:user_id/suspend", orgH.SuspendMember)
			biz.POST("/members/:user_id/reactivate", orgH.ReactivateMember)
			biz.DELETE("/members/:user_id", orgH.RemoveMember)

			biz.GET("/members/:user_id/permissions", orgH.ListMemberPermissions)
			biz.POST("/members/:user_id/permissions", orgH.GrantPermission)
			biz.DELETE("/members/:user_id/permissions/:permission", orgH.RevokePermission)

			biz.GET("/audit-logs", orgH.ListAuditLogs)
		}

		// Reports and screenshots
		authed.GET("/reports/employees", reportsH.Roster)
		authed.GET("/reports/employees/:id/activity", reportsH.Activity)
		authed.GET("/reports/employees/:id/keystrokes", reportsH.Keystrokes)
		authed.GET("/reports/employees/:id/browser", reportsH.Browser)
		authed.GET("/reports/employees/:id/screenshots", reportsH.Screenshots)
		authed.GET("/screenshots/:client_uuid", reportsH.ScreenshotImage)

		org := authed.Group("/org/businesses/:id")
		{
			org.GET("/access", orgH.MyAccess)
			org.GET("/members", orgH.ListMembers)
			org.GET("/teams", orgH.ListTeams)
			org.POST("/teams", orgH.CreateTeam)
			org.GET("/teams/:team_id", orgH.GetTeam)
			org.PUT("/teams/:team_id", orgH.UpdateTeam)
			org.DELETE("/teams/:team_id", orgH.DeleteTeam)
			org.GET("/teams/:team_id/members", orgH.ListTeamMembers)
			org.POST("/teams/:team_id/members", orgH.AssignTeamMember)
			org.DELETE("/teams/:team_id/members/:user_id", orgH.RemoveTeamMember)

			org.POST("/members/:user_id/promote", orgH.PromoteMember)
			org.POST("/members/:user_id/demote", orgH.DemoteMember)
			org.POST("/members/:user_id/suspend", orgH.SuspendMember)
			org.POST("/members/:user_id/reactivate", orgH.ReactivateMember)
			org.DELETE("/members/:user_id", orgH.RemoveMember)

			org.GET("/members/:user_id/permissions", orgH.ListMemberPermissions)
			org.POST("/members/:user_id/permissions", orgH.GrantPermission)
			org.DELETE("/members/:user_id/permissions/:permission", orgH.RevokePermission)

			org.GET("/audit-logs", orgH.ListAuditLogs)
		}
	}
	return r
}

func TestHttpAuthorizationSecurity(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	defer pool.Close()

	st := store.New(pool)
	authMgr := auth.NewManager("test-jwt-secret-xyz-1234567890")
	router := setupTestRouter(st, authMgr)

	// Clean tables
	_, _ = pool.Exec(ctx, `TRUNCATE businesses, users CASCADE`)

	// Create Business A and Business B
	bizA := uuid.New().String()
	bizB := uuid.New().String()

	ownerA := uuid.New().String()
	leadA := uuid.New().String()
	empA := uuid.New().String()
	suspendedA := uuid.New().String()

	ownerB := uuid.New().String()
	empB := uuid.New().String()

	// Seed Users
	users := []struct {
		id   string
		name string
	}{
		{ownerA, "Owner A"},
		{leadA, "Lead A"},
		{empA, "Emp A"},
		{suspendedA, "Suspended A"},
		{ownerB, "Owner B"},
		{empB, "Emp B"},
	}
	for _, u := range users {
		_, err := pool.Exec(ctx,
			`INSERT INTO users (id, email, password_hash, display_name, account_type) 
			 VALUES ($1, $2, 'hash123', $3, 'manager')`,
			u.id, u.id+"@test.com", u.name,
		)
		if err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}

	// Seed Businesses
	_, _ = pool.Exec(ctx, `INSERT INTO businesses (id, name, kind, owner_user_id) VALUES ($1, 'Biz A', 'team', $2)`, bizA, ownerA)
	_, _ = pool.Exec(ctx, `INSERT INTO businesses (id, name, kind, owner_user_id) VALUES ($1, 'Biz B', 'team', $2)`, bizB, ownerB)

	// Seed Memberships
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'owner', false)`, bizA, ownerA)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'team_lead', false)`, bizA, leadA)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'employee', false)`, bizA, empA)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'employee', true)`, bizA, suspendedA)

	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'owner', false)`, bizB, ownerB)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'employee', false)`, bizB, empB)

	// Issue Tokens
	tokens := make(map[string]string)
	for _, u := range users {
		pair, err := authMgr.Issue(u.id)
		if err != nil {
			t.Fatalf("issue token for %s: %v", u.name, err)
		}
		tokens[u.id] = pair.AccessToken
	}

	makeReq := func(method, url, userID string, body any) *httptest.ResponseRecorder {
		var reqBody []byte
		if body != nil {
			reqBody, _ = json.Marshal(body)
		}
		req, _ := http.NewRequest(method, url, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		if tok, ok := tokens[userID]; ok {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// 1. Owner can access organizational data
	t.Run("Owner_AccessAllowed", func(t *testing.T) {
		rec := makeReq("GET", fmt.Sprintf("/v1/org/businesses/%s/teams", bizA), ownerA, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("owner should have 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		t.Log("PASS: Owner successfully accessed teams")
	})

	// 2. Employee cannot access Web Admin org routes
	t.Run("Employee_AccessBlocked", func(t *testing.T) {
		rec := makeReq("GET", fmt.Sprintf("/v1/org/businesses/%s/teams", bizA), empA, nil)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("employee should receive 403 Forbidden, got %d", rec.Code)
		}
		t.Log("PASS: Employee denied Web Admin access (403 Forbidden)")
	})

	// 3. Suspended member access blocked immediately
	t.Run("SuspendedMember_AccessBlocked", func(t *testing.T) {
		rec := makeReq("GET", fmt.Sprintf("/v1/org/businesses/%s/access", bizA), suspendedA, nil)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("suspended member should receive 403 Forbidden, got %d", rec.Code)
		}
		t.Log("PASS: Suspended member rejected with 403 Forbidden")
	})

	// 4. Cross-business ID manipulation
	t.Run("CrossBusiness_ManipulationBlocked", func(t *testing.T) {
		// Owner A tries to access Business B
		rec := makeReq("GET", fmt.Sprintf("/v1/org/businesses/%s/teams", bizB), ownerA, nil)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("accessing other business should return 403, got %d", rec.Code)
		}
		t.Log("PASS: Cross-business URL manipulation rejected (403 Forbidden)")
	})

	// 5. Team Lead without 'can_manage_team' cannot create teams
	t.Run("TeamLead_LackingPermissionBlocked", func(t *testing.T) {
		payload := map[string]any{"name": "Unauthorized Team"}
		rec := makeReq("POST", fmt.Sprintf("/v1/org/businesses/%s/teams", bizA), leadA, payload)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("team lead lacking can_manage_team should receive 403 Forbidden, got %d", rec.Code)
		}
		t.Log("PASS: Team lead lacking can_manage_team denied team creation (403 Forbidden)")
	})

	// 6. Grant 'can_manage_team' to Team Lead -> Now allowed!
	t.Run("TeamLead_WithPermissionAllowed", func(t *testing.T) {
		_ = st.GrantPermission(ctx, bizA, ownerA, leadA, "can_manage_team", true)
		payload := map[string]any{"name": "Authorized Sub-Team"}
		rec := makeReq("POST", fmt.Sprintf("/v1/org/businesses/%s/teams", bizA), leadA, payload)
		if rec.Code != http.StatusCreated {
			t.Fatalf("team lead with can_manage_team should receive 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}
		t.Log("PASS: Team lead with granted permission successfully created team (201 Created)")
	})

	// 7. Direct API manipulation: Employee attempting member removal directly
	t.Run("Employee_DirectRemovalAttemptBlocked", func(t *testing.T) {
		rec := makeReq("DELETE", fmt.Sprintf("/v1/org/businesses/%s/members/%s", bizA, leadA), empA, nil)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("employee direct delete request should return 403 Forbidden, got %d", rec.Code)
		}
		t.Log("PASS: Direct API manipulation by employee rejected (403 Forbidden)")
	})

	// 8. Direct API manipulation: Team Lead attempting unauthorized promotion
	t.Run("TeamLead_UnauthorizedPromotionBlocked", func(t *testing.T) {
		// Lead A does NOT have 'can_create_team_leads'
		payload := map[string]any{"permissions": []string{"can_view_reports"}}
		rec := makeReq("POST", fmt.Sprintf("/v1/org/businesses/%s/members/%s/promote", bizA, empA), leadA, payload)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("team lead without can_create_team_leads should return 403 Forbidden, got %d", rec.Code)
		}
		t.Log("PASS: Unauthorized promotion attempt by Team Lead rejected (403 Forbidden)")
	})

	// 9. Suspended member cannot fetch or acknowledge screenshot policy (returns 403 Forbidden)
	t.Run("SuspendedMember_ScreenshotPolicyBlocked", func(t *testing.T) {
		recGet := makeReq("GET", "/v1/screenshot-policy", suspendedA, nil)
		if recGet.Code != http.StatusForbidden {
			t.Fatalf("suspended member GetScreenshotPolicy should return 403 Forbidden, got %d: %s", recGet.Code, recGet.Body.String())
		}

		ackPayload := map[string]any{"policy_version": 1}
		recAck := makeReq("POST", "/v1/screenshot-policy/ack", suspendedA, ackPayload)
		if recAck.Code != http.StatusForbidden {
			t.Fatalf("suspended member AckScreenshotPolicy should return 403 Forbidden, got %d: %s", recAck.Code, recAck.Body.String())
		}
		t.Log("PASS: Suspended member GetScreenshotPolicy and AckScreenshotPolicy return 403 Forbidden")
	})

	// 10. Team member listing authorization and scoping
	t.Run("TeamMembers_ListingAuthorization", func(t *testing.T) {
		// Create a team in Biz A
		team, err := st.CreateTeam(ctx, bizA, "Engineering", nil, &leadA, nil, ownerA)
		if err != nil {
			t.Fatalf("create test team: %v", err)
		}
		// Assign empA to the team
		if err := st.AssignMemberToTeam(ctx, bizA, team.ID, empA, ownerA); err != nil {
			t.Fatalf("assign member: %v", err)
		}

		// Owner can list team members
		recOwner := makeReq("GET", fmt.Sprintf("/v1/org/businesses/%s/teams/%s/members", bizA, team.ID), ownerA, nil)
		if recOwner.Code != http.StatusOK {
			t.Fatalf("owner list team members should return 200, got %d: %s", recOwner.Code, recOwner.Body.String())
		}

		// Employee from same business cannot list team members directly (requires owner or team_lead role)
		recEmp := makeReq("GET", fmt.Sprintf("/v1/org/businesses/%s/teams/%s/members", bizA, team.ID), empA, nil)
		if recEmp.Code != http.StatusForbidden {
			t.Fatalf("employee list team members should return 403, got %d", recEmp.Code)
		}

		// Cross-business owner (ownerB) cannot list team members of bizA
		recCross := makeReq("GET", fmt.Sprintf("/v1/org/businesses/%s/teams/%s/members", bizA, team.ID), ownerB, nil)
		if recCross.Code != http.StatusForbidden {
			t.Fatalf("cross-business owner list team members should return 403, got %d", recCross.Code)
		}
		t.Log("PASS: Team member listing authorization enforces role and business scoping")
	})
}

// TestListTeamMembersAuthorization_IndependentScenarios independently tests all 10 required
// authorization regression scenarios for GET /v1/businesses/:id/teams/:team_id/members.
func TestListTeamMembersAuthorization_IndependentScenarios(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	defer pool.Close()

	st := store.New(pool)
	authMgr := auth.NewManager("test-jwt-secret-xyz-1234567890")
	router := setupTestRouter(st, authMgr)

	// Clean test fixtures
	_, _ = pool.Exec(ctx, `TRUNCATE businesses, users CASCADE`)

	bizA := uuid.New().String()
	bizB := uuid.New().String()

	ownerA := uuid.New().String()
	teamLead1 := uuid.New().String()
	teamLead2 := uuid.New().String()
	emp1 := uuid.New().String()
	emp2 := uuid.New().String()
	hrPermitted := uuid.New().String()
	hrUnpermitted := uuid.New().String()
	suspendedUser := uuid.New().String()

	ownerB := uuid.New().String()
	teamLeadB := uuid.New().String()

	users := []struct {
		id   string
		name string
	}{
		{ownerA, "Owner Org A"},
		{teamLead1, "Lead Team 1 Org A"},
		{teamLead2, "Lead Team 2 Org A"},
		{emp1, "Employee 1 Org A"},
		{emp2, "Employee 2 Org A"},
		{hrPermitted, "HR Permitted Org A"},
		{hrUnpermitted, "HR Unpermitted Org A"},
		{suspendedUser, "Suspended User Org A"},
		{ownerB, "Owner Org B"},
		{teamLeadB, "Lead Org B"},
	}
	for _, u := range users {
		_, err := pool.Exec(ctx,
			`INSERT INTO users (id, email, password_hash, display_name, account_type) 
			 VALUES ($1, $2, 'hash123', $3, 'manager')`,
			u.id, u.id+"@example.com", u.name,
		)
		if err != nil {
			t.Fatalf("seed user %s: %v", u.name, err)
		}
	}

	// Seed Businesses
	_, _ = pool.Exec(ctx, `INSERT INTO businesses (id, name, kind, owner_user_id) VALUES ($1, 'Acme A', 'team', $2)`, bizA, ownerA)
	_, _ = pool.Exec(ctx, `INSERT INTO businesses (id, name, kind, owner_user_id) VALUES ($1, 'Beta B', 'team', $2)`, bizB, ownerB)

	// Seed Memberships
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'owner', false)`, bizA, ownerA)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'team_lead', false)`, bizA, teamLead1)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'team_lead', false)`, bizA, teamLead2)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'employee', false)`, bizA, emp1)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'employee', false)`, bizA, emp2)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'employee', false)`, bizA, hrPermitted)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'employee', false)`, bizA, hrUnpermitted)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'employee', true)`, bizA, suspendedUser)

	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'owner', false)`, bizB, ownerB)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'team_lead', false)`, bizB, teamLeadB)

	// Delegated permission for hrPermitted: can_view_team
	if err := st.GrantPermission(ctx, bizA, ownerA, hrPermitted, "can_view_team", true); err != nil {
		t.Fatalf("grant hr permission: %v", err)
	}

	// Create Teams in Biz A
	team1A, err := st.CreateTeam(ctx, bizA, "Frontend Team Alpha", nil, &teamLead1, nil, ownerA)
	if err != nil {
		t.Fatalf("create team 1A: %v", err)
	}
	team2A, err := st.CreateTeam(ctx, bizA, "Backend Team Beta", nil, &teamLead2, nil, ownerA)
	if err != nil {
		t.Fatalf("create team 2A: %v", err)
	}
	// Create Team in Biz B
	teamB, err := st.CreateTeam(ctx, bizB, "External Team Gamma", nil, &teamLeadB, nil, ownerB)
	if err != nil {
		t.Fatalf("create team B: %v", err)
	}

	// Assign members to teams
	if err := st.AssignMemberToTeam(ctx, bizA, team1A.ID, emp1, ownerA); err != nil {
		t.Fatalf("assign emp1 to team1A: %v", err)
	}
	if err := st.AssignMemberToTeam(ctx, bizA, team2A.ID, emp2, ownerA); err != nil {
		t.Fatalf("assign emp2 to team2A: %v", err)
	}

	// Issue auth tokens
	tokens := make(map[string]string)
	for _, u := range users {
		pair, err := authMgr.Issue(u.id)
		if err != nil {
			t.Fatalf("issue token: %v", err)
		}
		tokens[u.id] = pair.AccessToken
	}

	makeReq := func(userID, businessID, teamID string) *httptest.ResponseRecorder {
		url := fmt.Sprintf("/v1/businesses/%s/teams/%s/members", businessID, teamID)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+tokens[userID])
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// 1. Employee accessing their own team -> 403 Forbidden
	t.Run("Scenario1_Employee_AccessingOwnTeam", func(t *testing.T) {
		rec := makeReq(emp1, bizA, team1A.ID)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d: %s", rec.Code, rec.Body.String())
		}
		t.Log("PASS: Scenario 1 - Employee accessing own team rejected with 403")
	})

	// 2. Employee accessing another team -> 403 Forbidden
	t.Run("Scenario2_Employee_AccessingAnotherTeam", func(t *testing.T) {
		rec := makeReq(emp1, bizA, team2A.ID)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d: %s", rec.Code, rec.Body.String())
		}
		t.Log("PASS: Scenario 2 - Employee accessing another team rejected with 403")
	})

	// 3. Team Lead accessing their assigned team -> 200 OK
	t.Run("Scenario3_TeamLead_AccessingAssignedTeam", func(t *testing.T) {
		rec := makeReq(teamLead1, bizA, team1A.ID)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Members []store.OrgMember `json:"members"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if len(body.Members) == 0 {
			t.Fatalf("expected team members in response, got 0")
		}
		t.Logf("PASS: Scenario 3 - Team Lead accessed assigned team with %d member(s)", len(body.Members))
	})

	// 4. Team Lead accessing an unrelated team in the same organization -> 403 Forbidden
	t.Run("Scenario4_TeamLead_AccessingUnrelatedTeamInSameOrg", func(t *testing.T) {
		rec := makeReq(teamLead1, bizA, team2A.ID)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d: %s", rec.Code, rec.Body.String())
		}
		t.Log("PASS: Scenario 4 - Team Lead accessing unrelated team in same org rejected with 403")
	})

	// 5. Team Lead accessing a team in another organization -> 403 Forbidden
	t.Run("Scenario5_TeamLead_AccessingTeamInAnotherOrg", func(t *testing.T) {
		rec := makeReq(teamLead1, bizB, teamB.ID)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d: %s", rec.Code, rec.Body.String())
		}
		t.Log("PASS: Scenario 5 - Team Lead accessing team in another org rejected with 403")
	})

	// 6. HR with the required permissions -> 200 OK
	t.Run("Scenario6_HR_WithRequiredPermissions", func(t *testing.T) {
		rec := makeReq(hrPermitted, bizA, team1A.ID)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		t.Log("PASS: Scenario 6 - HR with can_view_team allowed (200 OK)")
	})

	// 7. HR without the required permissions -> 403 Forbidden
	t.Run("Scenario7_HR_WithoutRequiredPermissions", func(t *testing.T) {
		rec := makeReq(hrUnpermitted, bizA, team1A.ID)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d: %s", rec.Code, rec.Body.String())
		}
		t.Log("PASS: Scenario 7 - HR without permissions rejected with 403")
	})

	// 8. Owner accessing all authorized teams -> 200 OK
	t.Run("Scenario8_Owner_AccessingAllAuthorizedTeams", func(t *testing.T) {
		rec1 := makeReq(ownerA, bizA, team1A.ID)
		if rec1.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for team 1, got %d: %s", rec1.Code, rec1.Body.String())
		}
		rec2 := makeReq(ownerA, bizA, team2A.ID)
		if rec2.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for team 2, got %d: %s", rec2.Code, rec2.Body.String())
		}
		t.Log("PASS: Scenario 8 - Owner accessed all teams in organization (200 OK)")
	})

	// 9. Suspended member attempting access -> 403 Forbidden
	t.Run("Scenario9_SuspendedMember_AttemptingAccess", func(t *testing.T) {
		rec := makeReq(suspendedUser, bizA, team1A.ID)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d: %s", rec.Code, rec.Body.String())
		}
		t.Log("PASS: Scenario 9 - Suspended member rejected with 403 Forbidden")
	})

	// 10. Team ID belonging to a different business -> 404 Not Found (without exposing cross-tenant info)
	t.Run("Scenario10_TeamID_BelongingToDifferentBusiness", func(t *testing.T) {
		rec := makeReq(ownerA, bizA, teamB.ID)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found, got %d: %s", rec.Code, rec.Body.String())
		}
		t.Log("PASS: Scenario 10 - Team belonging to different business returns 404 Not Found")
	})
}

// TestScreenshotAccess_NonEmptyTestData verifies non-empty screenshot metadata and direct binary image streaming
// for all roles, enforcing team hierarchy boundaries, self-monitoring prevention, and delegated permissions.
func TestScreenshotAccess_NonEmptyTestData(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	defer pool.Close()

	st := store.New(pool)
	authMgr := auth.NewManager("test-jwt-secret-xyz-1234567890")
	filestoreRoot := t.TempDir()
	fileSt := filestore.New(filestoreRoot)
	router := setupTestRouter(st, authMgr, fileSt)

	// Clean tables
	_, _ = pool.Exec(ctx, `TRUNCATE businesses, users, screenshots CASCADE`)

	bizA := uuid.New().String()
	bizB := uuid.New().String()

	ownerA := uuid.New().String()
	leadA := uuid.New().String()
	subordinateA := uuid.New().String()
	unrelatedA := uuid.New().String()
	hrPermitted := uuid.New().String()
	hrUnpermitted := uuid.New().String()

	ownerB := uuid.New().String()
	empB := uuid.New().String()

	users := []struct {
		id   string
		name string
	}{
		{ownerA, "Admin Owner A"},
		{leadA, "Team Lead A"},
		{subordinateA, "Subordinate A"},
		{unrelatedA, "Unrelated A"},
		{hrPermitted, "HR Screenshot Permitted"},
		{hrUnpermitted, "HR Screenshot Unpermitted"},
		{ownerB, "Admin Owner B"},
		{empB, "Employee B"},
	}
	for _, u := range users {
		_, err := pool.Exec(ctx,
			`INSERT INTO users (id, email, password_hash, display_name, account_type) 
			 VALUES ($1, $2, 'hash123', $3, 'manager')`,
			u.id, u.id+"@test.com", u.name,
		)
		if err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}

	// Seed Businesses
	_, _ = pool.Exec(ctx, `INSERT INTO businesses (id, name, kind, owner_user_id) VALUES ($1, 'Acme Corp', 'team', $2)`, bizA, ownerA)
	_, _ = pool.Exec(ctx, `INSERT INTO businesses (id, name, kind, owner_user_id) VALUES ($1, 'Beta Corp', 'team', $2)`, bizB, ownerB)

	// Seed Memberships
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'owner', false)`, bizA, ownerA)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'team_lead', false)`, bizA, leadA)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'employee', false)`, bizA, subordinateA)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'employee', false)`, bizA, unrelatedA)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'employee', false)`, bizA, hrPermitted)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'employee', false)`, bizA, hrUnpermitted)

	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'owner', false)`, bizB, ownerB)
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'employee', false)`, bizB, empB)

	// Grant permissions
	_ = st.GrantPermission(ctx, bizA, ownerA, leadA, "can_view_screenshots", true)
	_ = st.GrantPermission(ctx, bizA, ownerA, hrPermitted, "can_view_screenshots", true)
	_ = st.GrantPermission(ctx, bizA, ownerA, hrPermitted, "can_view_reports", true)

	// Create Teams in Biz A
	teamAlpha, err := st.CreateTeam(ctx, bizA, "Alpha Team", nil, &leadA, nil, ownerA)
	if err != nil {
		t.Fatalf("create team alpha: %v", err)
	}
	unrelatedLead := uuid.New().String()
	_, _ = pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, display_name, account_type) VALUES ($1, $2, 'hash123', 'Unrelated Lead', 'manager')`, unrelatedLead, unrelatedLead+"@test.com")
	_, _ = pool.Exec(ctx, `INSERT INTO memberships (business_id, user_id, role, is_suspended) VALUES ($1, $2, 'team_lead', false)`, bizA, unrelatedLead)
	teamBeta, err := st.CreateTeam(ctx, bizA, "Beta Team", nil, &unrelatedLead, nil, ownerA)
	if err != nil {
		t.Fatalf("create team beta: %v", err)
	}

	// Assign subordinateA to Alpha Team, unrelatedA to Beta Team
	_ = st.AssignMemberToTeam(ctx, bizA, teamAlpha.ID, subordinateA, ownerA)
	_ = st.AssignMemberToTeam(ctx, bizA, teamBeta.ID, unrelatedA, ownerA)

	// Write physical test screenshots and insert database records
	now := time.Now().Unix()
	webpData := []byte("RIFF\x1a\x00\x00\x00WEBPVP8 \x0e\x00\x00\x00\xb0\x01\x00\x9d\x01\x2a\x01\x00\x01\x00\x00")

	insertShot := func(bizID, userID string, ts int64) string {
		clientUUID := uuid.New().String()
		relPath, err := fileSt.Write(bizID, userID, ts, clientUUID, webpData)
		if err != nil {
			t.Fatalf("write screenshot file: %v", err)
		}
		deviceID := uuid.New().String()
		_, err = pool.Exec(ctx,
			`INSERT INTO screenshots (client_uuid, user_id, business_id, device_id, ts, file_path, byte_size, width, height, client_updated_at, capture_source)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, 1920, 1080, $5, 'automatic')`,
			clientUUID, userID, bizID, deviceID, ts, relPath, len(webpData),
		)
		if err != nil {
			t.Fatalf("insert screenshot record: %v", err)
		}
		return clientUUID
	}

	shotSubordinate := insertShot(bizA, subordinateA, now-100)
	shotLead := insertShot(bizA, leadA, now-200)
	shotUnrelated := insertShot(bizA, unrelatedA, now-300)
	shotEmpB := insertShot(bizB, empB, now-400)

	// Issue auth tokens
	tokens := make(map[string]string)
	for _, u := range users {
		pair, err := authMgr.Issue(u.id)
		if err != nil {
			t.Fatalf("issue token: %v", err)
		}
		tokens[u.id] = pair.AccessToken
	}

	getMetadata := func(callerID, targetEmployeeID string) *httptest.ResponseRecorder {
		url := fmt.Sprintf("/v1/reports/employees/%s/screenshots?from=%d&to=%d", targetEmployeeID, now-1000, now+100)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+tokens[callerID])
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	getImage := func(callerID, clientUUID string) *httptest.ResponseRecorder {
		url := fmt.Sprintf("/v1/screenshots/%s", clientUUID)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+tokens[callerID])
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// 1. Employee cannot access another employee's screenshots
	t.Run("Employee_AccessOtherEmployeeScreenshots_Blocked", func(t *testing.T) {
		recMeta := getMetadata(subordinateA, unrelatedA)
		if recMeta.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for metadata, got %d", recMeta.Code)
		}
		recImg := getImage(subordinateA, shotUnrelated)
		if recImg.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for direct image stream, got %d", recImg.Code)
		}
		t.Log("PASS: Employee blocked from other employee's screenshot metadata and image stream")
	})

	// 2. Team Lead can access authorized subordinates' screenshots
	t.Run("TeamLead_AccessAuthorizedSubordinate_Allowed", func(t *testing.T) {
		recMeta := getMetadata(leadA, subordinateA)
		if recMeta.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for metadata, got %d: %s", recMeta.Code, recMeta.Body.String())
		}
		var body struct {
			Screenshots []any `json:"screenshots"`
		}
		_ = json.Unmarshal(recMeta.Body.Bytes(), &body)
		if len(body.Screenshots) == 0 {
			t.Fatalf("expected non-empty screenshot metadata list, got 0")
		}
		recImg := getImage(leadA, shotSubordinate)
		if recImg.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for image stream, got %d", recImg.Code)
		}
		if recImg.Header().Get("Content-Type") != "image/webp" {
			t.Fatalf("expected image/webp content-type, got %s", recImg.Header().Get("Content-Type"))
		}
		if recImg.Body.Len() == 0 {
			t.Fatalf("expected non-empty image stream body")
		}
		t.Logf("PASS: Team Lead successfully accessed subordinate screenshot metadata and streamed %d bytes", recImg.Body.Len())
	})

	// 3. Team Lead CANNOT view their own screenshots through team monitoring
	t.Run("TeamLead_ViewOwnScreenshots_Blocked", func(t *testing.T) {
		recMeta := getMetadata(leadA, leadA)
		if recMeta.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for self-monitoring metadata, got %d", recMeta.Code)
		}
		recImg := getImage(leadA, shotLead)
		if recImg.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for direct self image stream, got %d", recImg.Code)
		}
		t.Log("PASS: Team Lead self-monitoring blocked on both metadata (403) and binary stream (404)")
	})

	// 4. Team Lead CANNOT view screenshots of unrelated team members
	t.Run("TeamLead_ViewUnrelatedTeamMember_Blocked", func(t *testing.T) {
		recMeta := getMetadata(leadA, unrelatedA)
		if recMeta.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d", recMeta.Code)
		}
		recImg := getImage(leadA, shotUnrelated)
		if recImg.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found, got %d", recImg.Code)
		}
		t.Log("PASS: Team Lead blocked from unrelated team member's screenshots (403/404)")
	})

	// 5. HR access follows delegated screenshot permissions
	t.Run("HR_Access_FollowsDelegatedPermissions", func(t *testing.T) {
		// HR with can_view_screenshots -> Allowed
		recMetaPerm := getMetadata(hrPermitted, subordinateA)
		if recMetaPerm.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for permitted HR metadata, got %d: %s", recMetaPerm.Code, recMetaPerm.Body.String())
		}
		recImgPerm := getImage(hrPermitted, shotSubordinate)
		if recImgPerm.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for permitted HR image stream, got %d", recImgPerm.Code)
		}

		// HR without can_view_screenshots -> Blocked
		recMetaNoPerm := getMetadata(hrUnpermitted, subordinateA)
		if recMetaNoPerm.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for unpermitted HR metadata, got %d", recMetaNoPerm.Code)
		}
		recImgNoPerm := getImage(hrUnpermitted, shotSubordinate)
		if recImgNoPerm.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for unpermitted HR image stream, got %d", recImgNoPerm.Code)
		}
		t.Log("PASS: HR screenshot access strictly follows delegated permissions (can_view_screenshots)")
	})

	// 6. Admin access follows organization ownership
	t.Run("Admin_Access_FollowsOrganizationOwnership", func(t *testing.T) {
		recMeta := getMetadata(ownerA, subordinateA)
		if recMeta.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for owner metadata, got %d: %s", recMeta.Code, recMeta.Body.String())
		}
		recImg := getImage(ownerA, shotSubordinate)
		if recImg.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for owner image stream, got %d", recImg.Code)
		}

		// Owner A accessing Emp B from different org -> 403 / 404
		recCrossMeta := getMetadata(ownerA, empB)
		if recCrossMeta.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for cross-org metadata, got %d", recCrossMeta.Code)
		}
		recCrossImg := getImage(ownerA, shotEmpB)
		if recCrossImg.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for cross-org image stream, got %d", recCrossImg.Code)
		}
		t.Log("PASS: Admin access strictly confined to organization ownership with tenant isolation")
	})

	// 7. Direct UUID manipulation does not bypass authorization
	t.Run("Direct_UUID_Manipulation_Blocked", func(t *testing.T) {
		fakeUUID := uuid.New().String()
		rec := getImage(leadA, fakeUUID)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for forged UUID, got %d", rec.Code)
		}
		recEmp := getImage(subordinateA, fakeUUID)
		if recEmp.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for forged UUID by employee, got %d", recEmp.Code)
		}
		t.Log("PASS: Direct UUID manipulation returns 404 Not Found without leaking existence or files")
	})
}
