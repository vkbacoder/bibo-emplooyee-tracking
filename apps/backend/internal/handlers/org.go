package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"ctracking/backend/internal/auth"
	"ctracking/backend/internal/store"

	"github.com/gin-gonic/gin"
)

// OrgHandler handles organizational hierarchy, teams, roles, delegation and audit logging.
type OrgHandler struct {
	store *store.Store
}

// NewOrgHandler wires the organizational handler.
func NewOrgHandler(s *store.Store) *OrgHandler {
	return &OrgHandler{store: s}
}

// requireRole checks if caller is in the business with allowed roles and is not suspended.
func (h *OrgHandler) requireRole(c *gin.Context, businessID string, allowedRoles ...string) (string, bool) {
	callerID, _ := auth.UserID(c)
	role, isSuspended, err := h.store.GetMemberRole(c.Request.Context(), businessID, callerID)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a member of this business"})
		return "", false
	}
	if err != nil {
		serverError(c, err)
		return "", false
	}
	if isSuspended {
		c.JSON(http.StatusForbidden, gin.H{"error": "business membership suspended"})
		return "", false
	}

	for _, r := range allowedRoles {
		if role == r {
			return role, true
		}
	}

	c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
	return "", false
}

// requirePerm checks if caller is owner or has the required granular permission.
func (h *OrgHandler) requirePerm(c *gin.Context, businessID, perm string) bool {
	callerID, _ := auth.UserID(c)
	role, isSuspended, err := h.store.GetMemberRole(c.Request.Context(), businessID, callerID)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a member of this business"})
		return false
	}
	if err != nil {
		serverError(c, err)
		return false
	}
	if isSuspended {
		c.JSON(http.StatusForbidden, gin.H{"error": "business membership suspended"})
		return false
	}
	if role == "owner" {
		return true
	}

	hasPerm, err := h.store.HasPermission(c.Request.Context(), businessID, callerID, perm)
	if err != nil || !hasPerm {
		c.JSON(http.StatusForbidden, gin.H{"error": fmt.Sprintf("missing required permission: %s", perm)})
		return false
	}
	return true
}

// MyAccess returns the caller's role, suspension status, permissions, and team scoping.
func (h *OrgHandler) MyAccess(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	businessID := c.Param("id")

	role, isSuspended, err := h.store.GetMemberRole(c.Request.Context(), businessID, callerID)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a member of this business"})
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	if isSuspended {
		c.JSON(http.StatusForbidden, gin.H{"error": "business membership suspended"})
		return
	}

	teamIDs, isOrgWide, err := h.store.GetAuthorizedTeamIDs(c.Request.Context(), businessID, callerID)
	if err != nil && !errors.Is(err, store.ErrSuspended) {
		serverError(c, err)
		return
	}

	perms, err := h.store.ListUserPermissions(c.Request.Context(), businessID, callerID)
	if err != nil {
		serverError(c, err)
		return
	}

	permNames := make([]string, 0, len(perms))
	for _, p := range perms {
		permNames = append(permNames, p.Permission)
	}

	c.JSON(http.StatusOK, gin.H{
		"role":                role,
		"is_suspended":        isSuspended,
		"is_org_wide":         isOrgWide,
		"authorized_team_ids": teamIDs,
		"permissions":         permNames,
		"user_permissions":    perms,
	})
}

// ListMembers returns all business members within the caller's organizational scope.
func (h *OrgHandler) ListMembers(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	businessID := c.Param("id")

	if _, ok := h.requireRole(c, businessID, "owner", "team_lead"); !ok {
		return
	}

	members, err := h.store.ListOrgMembers(c.Request.Context(), businessID, callerID)
	if err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"members": members})
}

// ListTeams returns the teams within the caller's organizational scope.
func (h *OrgHandler) ListTeams(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	businessID := c.Param("id")

	if _, ok := h.requireRole(c, businessID, "owner", "team_lead"); !ok {
		return
	}

	teams, err := h.store.ListTeams(c.Request.Context(), businessID, callerID)
	if err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"teams": teams})
}

type createTeamReq struct {
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	LeadUserID   *string `json:"lead_user_id"`
	ParentTeamID *string `json:"parent_team_id"`
}

// CreateTeam creates a new organizational team.
func (h *OrgHandler) CreateTeam(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	businessID := c.Param("id")

	if !h.requirePerm(c, businessID, "can_manage_team") {
		return
	}

	var req createTeamReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		badRequest(c, "team name is required")
		return
	}

	team, err := h.store.CreateTeam(c.Request.Context(), businessID, req.Name, req.Description, req.LeadUserID, req.ParentTeamID, callerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, team)
}

// GetTeam returns a single team.
func (h *OrgHandler) GetTeam(c *gin.Context) {
	businessID := c.Param("id")
	teamID := c.Param("team_id")

	if _, ok := h.requireRole(c, businessID, "owner", "team_lead"); !ok {
		return
	}

	team, err := h.store.GetTeam(c.Request.Context(), businessID, teamID)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "team not found"})
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, team)
}

// UpdateTeam updates an existing team.
func (h *OrgHandler) UpdateTeam(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	businessID := c.Param("id")
	teamID := c.Param("team_id")

	if !h.requirePerm(c, businessID, "can_manage_team") {
		return
	}

	var req createTeamReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		badRequest(c, "team name is required")
		return
	}

	team, err := h.store.UpdateTeam(c.Request.Context(), businessID, teamID, req.Name, req.Description, req.LeadUserID, req.ParentTeamID, callerID)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "team not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, team)
}

// DeleteTeam deletes a team.
func (h *OrgHandler) DeleteTeam(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	businessID := c.Param("id")
	teamID := c.Param("team_id")

	if _, ok := h.requireRole(c, businessID, "owner"); !ok {
		return
	}

	err := h.store.DeleteTeam(c.Request.Context(), businessID, teamID, callerID)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "team not found"})
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ListTeamMembers returns all members belonging to the specified team within caller's scope.
func (h *OrgHandler) ListTeamMembers(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	businessID := c.Param("id")
	teamID := c.Param("team_id")

	if _, ok := h.requireRole(c, businessID, "owner", "team_lead"); !ok {
		return
	}

	members, err := h.store.ListOrgMembers(c.Request.Context(), businessID, callerID)
	if err != nil {
		serverError(c, err)
		return
	}

	teamMembers := make([]store.OrgMember, 0)
	for _, m := range members {
		if m.TeamID != nil && *m.TeamID == teamID {
			teamMembers = append(teamMembers, m)
		}
	}

	c.JSON(http.StatusOK, gin.H{"members": teamMembers})
}

type assignMemberReq struct {
	UserID string `json:"user_id"`
}

// AssignTeamMember assigns an employee to a team.
func (h *OrgHandler) AssignTeamMember(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	businessID := c.Param("id")
	teamID := c.Param("team_id")

	if !h.requirePerm(c, businessID, "can_assign_employees") {
		return
	}

	var req assignMemberReq
	if err := c.ShouldBindJSON(&req); err != nil || req.UserID == "" {
		badRequest(c, "user_id is required")
		return
	}

	err := h.store.AssignMemberToTeam(c.Request.Context(), businessID, teamID, req.UserID, callerID)
	if errors.Is(err, store.ErrSuspended) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot assign a suspended member to a team"})
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// RemoveTeamMember unassigns an employee from their team.
func (h *OrgHandler) RemoveTeamMember(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	businessID := c.Param("id")
	userID := c.Param("user_id")

	if !h.requirePerm(c, businessID, "can_assign_employees") {
		return
	}

	err := h.store.RemoveMemberFromTeam(c.Request.Context(), businessID, userID, callerID)
	if err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type promoteReq struct {
	TeamID      *string  `json:"team_id"`
	Permissions []string `json:"permissions"`
}

// PromoteMember upgrades an employee to Team Lead while preserving their identity.
func (h *OrgHandler) PromoteMember(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	businessID := c.Param("id")
	targetUserID := c.Param("user_id")

	if !h.requirePerm(c, businessID, "can_create_team_leads") {
		return
	}

	var req promoteReq
	_ = c.ShouldBindJSON(&req)

	err := h.store.PromoteToTeamLead(c.Request.Context(), businessID, callerID, targetUserID, req.TeamID, req.Permissions)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}
	if errors.Is(err, store.ErrSuspended) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot promote a suspended member"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type demoteReq struct {
	ReplacementLeadID *string `json:"replacement_lead_id"`
}

// DemoteMember demotes a Team Lead to an employee.
func (h *OrgHandler) DemoteMember(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	businessID := c.Param("id")
	targetUserID := c.Param("user_id")

	if _, ok := h.requireRole(c, businessID, "owner"); !ok {
		return
	}

	var req demoteReq
	_ = c.ShouldBindJSON(&req)

	err := h.store.DemoteToEmployee(c.Request.Context(), businessID, callerID, targetUserID, req.ReplacementLeadID)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}
	if errors.Is(err, store.ErrCannotRemoveOwner) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot demote the business owner"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type suspendReq struct {
	ReplacementLeadID *string `json:"replacement_lead_id"`
}

// SuspendMember suspends a user's membership in this business.
func (h *OrgHandler) SuspendMember(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	businessID := c.Param("id")
	targetUserID := c.Param("user_id")

	if _, ok := h.requireRole(c, businessID, "owner"); !ok {
		return
	}

	var req suspendReq
	_ = c.ShouldBindJSON(&req)

	err := h.store.SetMemberSuspended(c.Request.Context(), businessID, callerID, targetUserID, true, req.ReplacementLeadID)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}
	if errors.Is(err, store.ErrCannotSuspendOwner) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot suspend the business owner"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "is_suspended": true})
}

// ReactivateMember un-suspends a user's membership in this business.
func (h *OrgHandler) ReactivateMember(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	businessID := c.Param("id")
	targetUserID := c.Param("user_id")

	if _, ok := h.requireRole(c, businessID, "owner"); !ok {
		return
	}

	err := h.store.SetMemberSuspended(c.Request.Context(), businessID, callerID, targetUserID, false, nil)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "is_suspended": false})
}

type removeMemberReq struct {
	ReplacementLeadID    *string `json:"replacement_lead_id"`
	AllowUnassignedTeams bool    `json:"allow_unassigned_teams"`
}

// RemoveMember removes an employee from the business using the verified 8-step workflow.
func (h *OrgHandler) RemoveMember(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	businessID := c.Param("id")
	targetUserID := c.Param("user_id")

	if _, ok := h.requireRole(c, businessID, "owner"); !ok {
		return
	}

	var req removeMemberReq
	_ = c.ShouldBindJSON(&req)

	err := h.store.RemoveMember(c.Request.Context(), businessID, callerID, targetUserID, req.ReplacementLeadID, req.AllowUnassignedTeams)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}
	if errors.Is(err, store.ErrCannotRemoveOwner) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot remove the business owner"})
		return
	}
	if errors.Is(err, store.ErrReplacementRequired) {
		c.JSON(http.StatusConflict, gin.H{
			"error": "departing member leads one or more teams; specify a replacement_lead_id or set allow_unassigned_teams to true",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ListMemberPermissions lists granular permissions for a member.
func (h *OrgHandler) ListMemberPermissions(c *gin.Context) {
	businessID := c.Param("id")
	targetUserID := c.Param("user_id")

	if _, ok := h.requireRole(c, businessID, "owner", "team_lead"); !ok {
		return
	}

	perms, err := h.store.ListUserPermissions(c.Request.Context(), businessID, targetUserID)
	if err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"permissions": perms})
}

type grantPermReq struct {
	Permission string `json:"permission"`
	Delegable  bool   `json:"delegable"`
}

// GrantPermission grants a granular permission to a member.
func (h *OrgHandler) GrantPermission(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	businessID := c.Param("id")
	targetUserID := c.Param("user_id")

	var req grantPermReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Permission == "" {
		badRequest(c, "permission is required")
		return
	}

	err := h.store.GrantPermission(c.Request.Context(), businessID, callerID, targetUserID, req.Permission, req.Delegable)
	if errors.Is(err, store.ErrDelegationDisallowed) {
		c.JSON(http.StatusForbidden, gin.H{"error": "you do not have authority to delegate this permission"})
		return
	}
	if errors.Is(err, store.ErrInvalidPermission) {
		badRequest(c, "invalid permission name")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// RevokePermission revokes a granular permission and cascades downstream delegations.
func (h *OrgHandler) RevokePermission(c *gin.Context) {
	callerID, _ := auth.UserID(c)
	businessID := c.Param("id")
	targetUserID := c.Param("user_id")
	permission := c.Param("permission")

	if _, ok := h.requireRole(c, businessID, "owner", "team_lead"); !ok {
		return
	}

	err := h.store.RevokePermission(c.Request.Context(), businessID, callerID, targetUserID, permission)
	if err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ListAuditLogs returns recent organization audit log entries.
func (h *OrgHandler) ListAuditLogs(c *gin.Context) {
	businessID := c.Param("id")

	if !h.requirePerm(c, businessID, "can_view_audit_logs") {
		return
	}

	limit := clampInt(c.Query("limit"), 50, 1, 100)
	offset := clampInt(c.Query("offset"), 0, 0, 1<<31)

	logs, err := h.store.ListAuditLogs(c.Request.Context(), businessID, limit, offset)
	if err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": logs})
}
