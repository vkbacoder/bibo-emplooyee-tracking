package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ctracking/backend/internal/auth"
	"ctracking/backend/internal/handlers"
	"ctracking/backend/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testDSN = "postgres://postgres@127.0.0.1:54339/ctracking_test?sslmode=disable"

func setupTestRouter(st *store.Store, authMgr *auth.Manager) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())

	orgH := handlers.NewOrgHandler(st)
	policyH := handlers.NewScreenshotPolicyHandler(st)

	authed := r.Group("/v1", authMgr.Required())
	{
		authed.GET("/screenshot-policy", policyH.GetScreenshotPolicy)
		authed.POST("/screenshot-policy/ack", policyH.AckScreenshotPolicy)

		org := authed.Group("/org/businesses/:id")
		{
			org.GET("/access", orgH.MyAccess)
			org.GET("/members", orgH.ListMembers)
			org.GET("/teams", orgH.ListTeams)
			org.POST("/teams", orgH.CreateTeam)
			org.GET("/teams/:team_id", orgH.GetTeam)
			org.PUT("/teams/:team_id", orgH.UpdateTeam)
			org.DELETE("/teams/:team_id", orgH.DeleteTeam)
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
}
