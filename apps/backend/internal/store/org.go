package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Org-related error constants
var (
	ErrSuspended            = errors.New("business membership is suspended")
	ErrCycleDetected        = errors.New("circular team hierarchy detected")
	ErrReplacementRequired  = errors.New("replacement team lead required")
	ErrCannotRemoveOwner    = errors.New("cannot remove the business owner")
	ErrCannotSuspendOwner   = errors.New("cannot suspend the business owner")
	ErrInvalidPermission    = errors.New("invalid permission")
	ErrDelegationDisallowed = errors.New("permission cannot be delegated")
)

// Organizational membership roles
const (
	RoleOwner    = "owner"
	RoleTeamLead = "team_lead"
	RoleEmployee = "employee"
)

// ValidRoles contains all allowed membership roles.
var ValidRoles = map[string]bool{
	RoleOwner:    true,
	RoleTeamLead: true,
	RoleEmployee: true,
}

// Canonical RBAC permission keys
const (
	PermCanViewTeam               = "can_view_team"
	PermCanManageEmployees        = "can_manage_employees"
	PermCanCreateTeamLeads        = "can_create_team_leads"
	PermCanAssignEmployees        = "can_assign_employees"
	PermCanManageScreenshotPolicy = "can_manage_screenshot_policy"
	PermCanViewScreenshots        = "can_view_screenshots"
	PermCanViewReports            = "can_view_reports"
	PermCanManageTeam             = "can_manage_team"
	PermCanViewAuditLogs          = "can_view_audit_logs"
)

// ValidPermissions contains the 9 granular RBAC permissions.
var ValidPermissions = map[string]bool{
	PermCanViewTeam:               true,
	PermCanManageEmployees:        true,
	PermCanCreateTeamLeads:        true,
	PermCanAssignEmployees:        true,
	PermCanManageScreenshotPolicy: true,
	PermCanViewScreenshots:        true,
	PermCanViewReports:            true,
	PermCanManageTeam:             true,
	PermCanViewAuditLogs:          true,
}


// Team represents a organizational department or group.
type Team struct {
	ID           string    `json:"id"`
	BusinessID   string    `json:"business_id"`
	Name         string    `json:"name"`
	Description  *string   `json:"description"`
	LeadUserID   *string   `json:"lead_user_id"`
	ParentTeamID *string   `json:"parent_team_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TeamWithDetails enriches Team with human-readable lead info and member counts.
type TeamWithDetails struct {
	Team
	LeadDisplayName *string `json:"lead_display_name,omitempty"`
	LeadEmail       *string `json:"lead_email,omitempty"`
	ParentTeamName  *string `json:"parent_team_name,omitempty"`
	MemberCount     int     `json:"member_count"`
	SubteamCount    int     `json:"subteam_count"`
}

// TeamMember represents an employee's primary team assignment.
type TeamMember struct {
	ID         string    `json:"id"`
	BusinessID string    `json:"business_id"`
	TeamID     string    `json:"team_id"`
	UserID     string    `json:"user_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// UserPermission represents a granular permission delegated to a member.
type UserPermission struct {
	ID              string    `json:"id"`
	BusinessID      string    `json:"business_id"`
	UserID          string    `json:"user_id"`
	Permission      string    `json:"permission"`
	Delegable       bool      `json:"delegable"`
	GrantedByUserID *string   `json:"granted_by_user_id"`
	CreatedAt       time.Time `json:"created_at"`
}

// OrgMember represents a member of a business with their role, suspension status, and team.
type OrgMember struct {
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"` // 'owner' | 'team_lead' | 'employee'
	IsSuspended bool      `json:"is_suspended"`
	TeamID      *string   `json:"team_id,omitempty"`
	TeamName    *string   `json:"team_name,omitempty"`
	IsTeamLead  bool      `json:"is_team_lead"`
	LedTeamIDs  []string  `json:"led_team_ids,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// OrgAuditLog represents an immutable audit record for organizational changes.
type OrgAuditLog struct {
	ID                string          `json:"id"`
	BusinessID        string          `json:"business_id"`
	ActorUserID       *string         `json:"actor_user_id"`
	ActorEmail        *string         `json:"actor_email"`
	ActorDisplayName  *string         `json:"actor_display_name"`
	TargetUserID      *string         `json:"target_user_id"`
	TargetEmail       *string         `json:"target_email"`
	TargetDisplayName *string         `json:"target_display_name"`
	TargetTeamID      *string         `json:"target_team_id"`
	TargetTeamName    *string         `json:"target_team_name"`
	Action            string          `json:"action"`
	Details           json.RawMessage `json:"details"`
	CreatedAt         time.Time       `json:"created_at"`
}

// ---------------------------------------------------------------------
// Membership & Authorization helpers
// ---------------------------------------------------------------------

// GetMemberRole fetches the role and suspension status of a user in a business.
func (s *Store) GetMemberRole(ctx context.Context, businessID, userID string) (string, bool, error) {
	var role string
	var isSuspended bool
	err := s.pool.QueryRow(ctx,
		`SELECT role, is_suspended FROM memberships WHERE business_id = $1 AND user_id = $2`,
		businessID, userID,
	).Scan(&role, &isSuspended)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, ErrNotFound
	}
	if err != nil {
		return "", false, err
	}
	return role, isSuspended, nil
}

// HasPermission checks if the user has a specific permission in the business.
// Owners always possess all permissions (unless suspended).
func (s *Store) HasPermission(ctx context.Context, businessID, userID, permission string) (bool, error) {
	if !ValidPermissions[permission] {
		return false, ErrInvalidPermission
	}
	role, isSuspended, err := s.GetMemberRole(ctx, businessID, userID)
	if err != nil {
		return false, err
	}
	if isSuspended {
		return false, nil
	}
	if role == "owner" {
		return true, nil
	}
	var exists bool
	err = s.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM user_permissions 
			 WHERE business_id = $1 AND user_id = $2 AND permission = $3
		)`,
		businessID, userID, permission,
	).Scan(&exists)
	return exists, err
}

// GetAuthorizedTeamIDs returns the team IDs an actor is authorized to manage or view.
// If isOrgWide is true, the actor has org-wide authority (e.g. owner) and teamIDs will be nil.
func (s *Store) GetAuthorizedTeamIDs(ctx context.Context, businessID, userID string) ([]string, bool, error) {
	role, isSuspended, err := s.GetMemberRole(ctx, businessID, userID)
	if err != nil {
		return nil, false, err
	}
	if isSuspended {
		return nil, false, ErrSuspended
	}
	if role == "owner" {
		return nil, true, nil
	}
	if role != "team_lead" {
		return []string{}, false, nil
	}

	// For Team Leads, collect all teams they lead plus all subordinate/descendant teams
	rows, err := s.pool.Query(ctx,
		`WITH RECURSIVE team_tree AS (
			SELECT id FROM teams WHERE business_id = $1 AND lead_user_id = $2
			UNION ALL
			SELECT t.id FROM teams t
			JOIN team_tree tt ON t.parent_team_id = tt.id
			WHERE t.business_id = $1
		)
		SELECT DISTINCT id FROM team_tree`,
		businessID, userID,
	)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var teamIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, false, err
		}
		teamIDs = append(teamIDs, id)
	}
	return teamIDs, false, rows.Err()
}

// CanAccessEmployee verifies whether actorUserID has authority to view/manage targetUserID in businessID.
func (s *Store) CanAccessEmployee(ctx context.Context, businessID, actorUserID, targetUserID string, requiredPermission string) (bool, error) {
	if actorUserID == targetUserID {
		return true, nil
	}
	role, isSuspended, err := s.GetMemberRole(ctx, businessID, actorUserID)
	if err != nil {
		return false, err
	}
	if isSuspended {
		return false, nil
	}
	if role == "owner" {
		return true, nil
	}

	// If a permission is required, verify actor holds it
	if requiredPermission != "" {
		hasPerm, err := s.HasPermission(ctx, businessID, actorUserID, requiredPermission)
		if err != nil || !hasPerm {
			return false, err
		}
	}

	// Verify target user is inside actor's authorized team hierarchy
	teamIDs, isOrgWide, err := s.GetAuthorizedTeamIDs(ctx, businessID, actorUserID)
	if err != nil {
		return false, err
	}
	if isOrgWide {
		return true, nil
	}
	if len(teamIDs) == 0 {
		return false, nil
	}

	var inScope bool
	err = s.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM team_members 
			 WHERE business_id = $1 AND user_id = $2 AND team_id = ANY($3)
			 UNION
			 SELECT 1 FROM teams
			  WHERE business_id = $1 AND lead_user_id = $2 AND id = ANY($3)
		)`,
		businessID, targetUserID, teamIDs,
	).Scan(&inScope)
	return inScope, err
}

// ---------------------------------------------------------------------
// Teams CRUD
// ---------------------------------------------------------------------

// CreateTeam inserts a new team.
func (s *Store) CreateTeam(ctx context.Context, businessID, name string, description, leadUserID, parentTeamID *string, actorUserID string) (Team, error) {
	var t Team
	err := s.pool.QueryRow(ctx,
		`INSERT INTO teams (business_id, name, description, lead_user_id, parent_team_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, business_id, name, description, lead_user_id, parent_team_id, created_at, updated_at`,
		businessID, name, description, leadUserID, parentTeamID,
	).Scan(&t.ID, &t.BusinessID, &t.Name, &t.Description, &t.LeadUserID, &t.ParentTeamID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return Team{}, err
	}

	// Log audit event
	_ = s.AppendAuditLog(ctx, businessID, &actorUserID, leadUserID, &t.ID, "team.created", map[string]any{
		"team_name":      name,
		"parent_team_id": parentTeamID,
	})

	return t, nil
}

// GetTeam retrieves a single team by ID.
func (s *Store) GetTeam(ctx context.Context, businessID, teamID string) (Team, error) {
	var t Team
	err := s.pool.QueryRow(ctx,
		`SELECT id, business_id, name, description, lead_user_id, parent_team_id, created_at, updated_at
		   FROM teams WHERE business_id = $1 AND id = $2`,
		businessID, teamID,
	).Scan(&t.ID, &t.BusinessID, &t.Name, &t.Description, &t.LeadUserID, &t.ParentTeamID, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Team{}, ErrNotFound
	}
	return t, err
}

// ListTeams returns all teams within a business with lead details and member counts.
func (s *Store) ListTeams(ctx context.Context, businessID string, callerUserID string) ([]TeamWithDetails, error) {
	teamIDs, isOrgWide, err := s.GetAuthorizedTeamIDs(ctx, businessID, callerUserID)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT t.id, t.business_id, t.name, t.description, t.lead_user_id, t.parent_team_id, t.created_at, t.updated_at,
		       u.display_name AS lead_display_name,
		       COALESCE(u.email, u.username) AS lead_email,
		       pt.name AS parent_team_name,
		       (SELECT COUNT(*) FROM team_members tm WHERE tm.team_id = t.id) AS member_count,
		       (SELECT COUNT(*) FROM teams sub WHERE sub.parent_team_id = t.id) AS subteam_count
		  FROM teams t
		  LEFT JOIN users u ON u.id = t.lead_user_id
		  LEFT JOIN teams pt ON pt.id = t.parent_team_id
		 WHERE t.business_id = $1
	`
	var rows pgx.Rows
	if isOrgWide {
		query += ` ORDER BY t.name`
		rows, err = s.pool.Query(ctx, query, businessID)
	} else {
		if len(teamIDs) == 0 {
			return []TeamWithDetails{}, nil
		}
		query += ` AND t.id = ANY($2) ORDER BY t.name`
		rows, err = s.pool.Query(ctx, query, businessID, teamIDs)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []TeamWithDetails{}
	for rows.Next() {
		var item TeamWithDetails
		if err := rows.Scan(
			&item.ID, &item.BusinessID, &item.Name, &item.Description, &item.LeadUserID, &item.ParentTeamID,
			&item.CreatedAt, &item.UpdatedAt,
			&item.LeadDisplayName, &item.LeadEmail, &item.ParentTeamName,
			&item.MemberCount, &item.SubteamCount,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// UpdateTeam updates a team's attributes.
func (s *Store) UpdateTeam(ctx context.Context, businessID, teamID, name string, description, leadUserID, parentTeamID *string, actorUserID string) (Team, error) {
	var t Team
	err := s.pool.QueryRow(ctx,
		`UPDATE teams
		    SET name = $3,
		        description = $4,
		        lead_user_id = $5,
		        parent_team_id = $6,
		        updated_at = now()
		  WHERE business_id = $1 AND id = $2
		  RETURNING id, business_id, name, description, lead_user_id, parent_team_id, created_at, updated_at`,
		businessID, teamID, name, description, leadUserID, parentTeamID,
	).Scan(&t.ID, &t.BusinessID, &t.Name, &t.Description, &t.LeadUserID, &t.ParentTeamID, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Team{}, ErrNotFound
	}
	if err != nil {
		return Team{}, err
	}

	_ = s.AppendAuditLog(ctx, businessID, &actorUserID, leadUserID, &t.ID, "team.updated", map[string]any{
		"team_name":      name,
		"parent_team_id": parentTeamID,
	})

	return t, nil
}

// DeleteTeam removes a team and reassigns its members.
func (s *Store) DeleteTeam(ctx context.Context, businessID, teamID string, actorUserID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var teamName string
	err = tx.QueryRow(ctx, `SELECT name FROM teams WHERE business_id = $1 AND id = $2`, businessID, teamID).Scan(&teamName)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	// Delete team (FK constraints will cascade delete team_members and nullify parent_team_ids)
	_, err = tx.Exec(ctx, `DELETE FROM teams WHERE business_id = $1 AND id = $2`, businessID, teamID)
	if err != nil {
		return err
	}

	err = s.AppendAuditLogTx(ctx, tx, businessID, &actorUserID, nil, &teamID, "team.deleted", map[string]any{
		"deleted_team_name": teamName,
	})
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------
// Team Members Assignment
// ---------------------------------------------------------------------

// AssignMemberToTeam places an employee in a team (enforcing 1 primary team per employee).
func (s *Store) AssignMemberToTeam(ctx context.Context, businessID, teamID, userID, actorUserID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Check if user is suspended
	var isSuspended bool
	err = tx.QueryRow(ctx,
		`SELECT is_suspended FROM memberships WHERE business_id = $1 AND user_id = $2`,
		businessID, userID,
	).Scan(&isSuspended)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if isSuspended {
		return ErrSuspended
	}

	// Upsert membership into team_members
	_, err = tx.Exec(ctx,
		`INSERT INTO team_members (business_id, team_id, user_id)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (business_id, user_id)
		 DO UPDATE SET team_id = EXCLUDED.team_id`,
		businessID, teamID, userID,
	)
	if err != nil {
		return err
	}

	err = s.AppendAuditLogTx(ctx, tx, businessID, &actorUserID, &userID, &teamID, "team.member_assigned", map[string]any{
		"team_id": teamID,
	})
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// RemoveMemberFromTeam removes an employee from their assigned team.
func (s *Store) RemoveMemberFromTeam(ctx context.Context, businessID, userID, actorUserID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var teamID string
	err = tx.QueryRow(ctx,
		`DELETE FROM team_members WHERE business_id = $1 AND user_id = $2 RETURNING team_id`,
		businessID, userID,
	).Scan(&teamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // already not in a team
	}
	if err != nil {
		return err
	}

	err = s.AppendAuditLogTx(ctx, tx, businessID, &actorUserID, &userID, &teamID, "team.member_removed", nil)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------
// Roster & Organizational Members Query
// ---------------------------------------------------------------------

// ListOrgMembers returns the roster with team assignment, role, and suspension status.
func (s *Store) ListOrgMembers(ctx context.Context, businessID, callerUserID string) ([]OrgMember, error) {
	teamIDs, isOrgWide, err := s.GetAuthorizedTeamIDs(ctx, businessID, callerUserID)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT u.id, COALESCE(u.email, ''), COALESCE(u.username, ''), u.display_name,
		       m.role, m.is_suspended, m.created_at,
		       tm.team_id, t.name AS team_name,
		       EXISTS(SELECT 1 FROM teams lt WHERE lt.business_id = $1 AND lt.lead_user_id = u.id) AS is_team_lead,
		       COALESCE((SELECT array_agg(lt.id::text) FROM teams lt WHERE lt.business_id = $1 AND lt.lead_user_id = u.id), '{}') AS led_team_ids
		  FROM memberships m
		  JOIN users u ON u.id = m.user_id
		  LEFT JOIN team_members tm ON tm.business_id = m.business_id AND tm.user_id = m.user_id
		  LEFT JOIN teams t ON t.id = tm.team_id
		 WHERE m.business_id = $1
	`

	var rows pgx.Rows
	if isOrgWide {
		query += ` ORDER BY u.display_name`
		rows, err = s.pool.Query(ctx, query, businessID)
	} else {
		if len(teamIDs) == 0 {
			// Team lead has no assigned teams yet
			return []OrgMember{}, nil
		}
		query += ` AND (tm.team_id = ANY($2) OR m.user_id = $3) ORDER BY u.display_name`
		rows, err = s.pool.Query(ctx, query, businessID, teamIDs, callerUserID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []OrgMember{}
	for rows.Next() {
		var m OrgMember
		if err := rows.Scan(
			&m.UserID, &m.Email, &m.Username, &m.DisplayName,
			&m.Role, &m.IsSuspended, &m.CreatedAt,
			&m.TeamID, &m.TeamName, &m.IsTeamLead, &m.LedTeamIDs,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------
// Promotion, Demotion & Suspension
// ---------------------------------------------------------------------

// PromoteToTeamLead promotes an employee to Team Lead while preserving all identity and credentials.
func (s *Store) PromoteToTeamLead(ctx context.Context, businessID, actorUserID, targetUserID string, teamID *string, permissions []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Step 1: Verify target exists and is not suspended
	var currentRole string
	var isSuspended bool
	err = tx.QueryRow(ctx,
		`SELECT role, is_suspended FROM memberships WHERE business_id = $1 AND user_id = $2`,
		businessID, targetUserID,
	).Scan(&currentRole, &isSuspended)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if isSuspended {
		return ErrSuspended
	}
	if currentRole == "owner" {
		return fmt.Errorf("user is already the business owner")
	}

	// Step 2: Update role to team_lead
	_, err = tx.Exec(ctx,
		`UPDATE memberships SET role = 'team_lead' WHERE business_id = $1 AND user_id = $2`,
		businessID, targetUserID,
	)
	if err != nil {
		return err
	}

	// Step 3: If teamID specified, set target as lead of that team
	if teamID != nil && *teamID != "" {
		_, err = tx.Exec(ctx,
			`UPDATE teams SET lead_user_id = $3, updated_at = now() WHERE business_id = $1 AND id = $2`,
			businessID, *teamID, targetUserID,
		)
		if err != nil {
			return err
		}
	}

	// Step 4: Grant delegated permissions
	for _, perm := range permissions {
		if !ValidPermissions[perm] {
			continue
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO user_permissions (business_id, user_id, permission, delegable, granted_by_user_id)
			 VALUES ($1, $2, $3, false, $4)
			 ON CONFLICT (business_id, user_id, permission) DO NOTHING`,
			businessID, targetUserID, perm, actorUserID,
		)
		if err != nil {
			return err
		}
	}

	// Step 5: Audit log
	err = s.AppendAuditLogTx(ctx, tx, businessID, &actorUserID, &targetUserID, teamID, "member.promoted_to_team_lead", map[string]any{
		"assigned_team_id": teamID,
		"permissions":      permissions,
	})
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// DemoteToEmployee demotes a Team Lead to an employee.
func (s *Store) DemoteToEmployee(ctx context.Context, businessID, actorUserID, targetUserID string, replacementLeadID *string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Step 1: Check target role
	var currentRole string
	err = tx.QueryRow(ctx,
		`SELECT role FROM memberships WHERE business_id = $1 AND user_id = $2`,
		businessID, targetUserID,
	).Scan(&currentRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if currentRole == "owner" {
		return ErrCannotRemoveOwner
	}
	if currentRole == "employee" {
		return nil // already an employee
	}

	// Step 2: Handle teams led by target
	var ledTeamIDs []string
	rows, err := tx.Query(ctx,
		`SELECT id FROM teams WHERE business_id = $1 AND lead_user_id = $2 ORDER BY id FOR UPDATE`,
		businessID, targetUserID,
	)
	if err != nil {
		return err
	}
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err != nil {
			rows.Close()
			return err
		}
		ledTeamIDs = append(ledTeamIDs, tid)
	}
	rows.Close()

	if len(ledTeamIDs) > 0 {
		if replacementLeadID != nil && *replacementLeadID != "" {
			// Verify replacement lead is active member with owner or team_lead role
			var repRole string
			var repSuspended bool
			err = tx.QueryRow(ctx,
				`SELECT role, is_suspended FROM memberships WHERE business_id = $1 AND user_id = $2`,
				businessID, *replacementLeadID,
			).Scan(&repRole, &repSuspended)
			if err != nil || repSuspended || (repRole != "owner" && repRole != "team_lead") {
				return fmt.Errorf("invalid replacement team lead")
			}
			_, err = tx.Exec(ctx,
				`UPDATE teams SET lead_user_id = $3, updated_at = now() WHERE business_id = $1 AND lead_user_id = $2`,
				businessID, targetUserID, *replacementLeadID,
			)
			if err != nil {
				return err
			}
		} else {
			// Clear team leadership
			_, err = tx.Exec(ctx,
				`UPDATE teams SET lead_user_id = NULL, updated_at = now() WHERE business_id = $1 AND lead_user_id = $2`,
				businessID, targetUserID,
			)
			if err != nil {
				return err
			}
		}
	}

	// Step 3: Cascade-revoke permissions granted by target
	_, err = tx.Exec(ctx,
		`WITH RECURSIVE delegation_tree AS (
			SELECT id, user_id, permission
			  FROM user_permissions
			 WHERE business_id = $1 AND granted_by_user_id = $2
			UNION ALL
			SELECT c.id, c.user_id, c.permission
			  FROM user_permissions c
			  JOIN delegation_tree p 
			    ON c.granted_by_user_id = p.user_id 
			   AND c.permission = p.permission
			 WHERE c.business_id = $1
		)
		DELETE FROM user_permissions WHERE id IN (SELECT id FROM delegation_tree)`,
		businessID, targetUserID,
	)
	if err != nil {
		return err
	}

	// Step 4: Revoke permissions held by target
	_, err = tx.Exec(ctx,
		`DELETE FROM user_permissions WHERE business_id = $1 AND user_id = $2`,
		businessID, targetUserID,
	)
	if err != nil {
		return err
	}

	// Step 5: Update role to employee
	_, err = tx.Exec(ctx,
		`UPDATE memberships SET role = 'employee' WHERE business_id = $1 AND user_id = $2`,
		businessID, targetUserID,
	)
	if err != nil {
		return err
	}

	// Step 6: Audit log
	err = s.AppendAuditLogTx(ctx, tx, businessID, &actorUserID, &targetUserID, nil, "member.demoted_to_employee", map[string]any{
		"replacement_lead_id": replacementLeadID,
		"cleared_teams":       ledTeamIDs,
	})
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// SetMemberSuspended sets business-specific suspension status.
func (s *Store) SetMemberSuspended(ctx context.Context, businessID, actorUserID, targetUserID string, suspended bool, replacementLeadID *string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var currentRole string
	err = tx.QueryRow(ctx,
		`SELECT role FROM memberships WHERE business_id = $1 AND user_id = $2`,
		businessID, targetUserID,
	).Scan(&currentRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if currentRole == "owner" {
		return ErrCannotSuspendOwner
	}

	if suspended {
		// If suspending, reassign or unassign teams led by this member
		if replacementLeadID != nil && *replacementLeadID != "" {
			_, err = tx.Exec(ctx,
				`UPDATE teams SET lead_user_id = $3, updated_at = now() WHERE business_id = $1 AND lead_user_id = $2`,
				businessID, targetUserID, *replacementLeadID,
			)
			if err != nil {
				return err
			}
		} else {
			_, err = tx.Exec(ctx,
				`UPDATE teams SET lead_user_id = NULL, updated_at = now() WHERE business_id = $1 AND lead_user_id = $2`,
				businessID, targetUserID,
			)
			if err != nil {
				return err
			}
		}
	}

	// Update is_suspended flag
	_, err = tx.Exec(ctx,
		`UPDATE memberships SET is_suspended = $3 WHERE business_id = $1 AND user_id = $2`,
		businessID, targetUserID, suspended,
	)
	if err != nil {
		return err
	}

	action := "member.suspended"
	if !suspended {
		action = "member.reactivated"
	}
	err = s.AppendAuditLogTx(ctx, tx, businessID, &actorUserID, &targetUserID, nil, action, map[string]any{
		"is_suspended": suspended,
	})
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------
// 8-Step Transactional Member Removal Workflow
// ---------------------------------------------------------------------

// RemoveMember executes the verified 8-step transactional workflow to remove a membership.
func (s *Store) RemoveMember(ctx context.Context, businessID, actorUserID, departingUserID string, replacementLeadID *string, allowUnassignedTeams bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Step 1: Verify actor authorization and that departing user is not the owner
	var actorRole string
	err = tx.QueryRow(ctx,
		`SELECT role FROM memberships WHERE business_id = $1 AND user_id = $2`,
		businessID, actorUserID,
	).Scan(&actorRole)
	if err != nil || actorRole != "owner" {
		return ErrForbidden
	}

	var departingRole string
	err = tx.QueryRow(ctx,
		`SELECT role FROM memberships WHERE business_id = $1 AND user_id = $2`,
		businessID, departingUserID,
	).Scan(&departingRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if departingRole == "owner" {
		return ErrCannotRemoveOwner
	}

	// Step 2: Lock the business with advisory lock and lock affected teams
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('finjulia_biz_' || $1))`, businessID)
	if err != nil {
		return err
	}

	// Step 3: Find every team led by the departing member in this business
	var ledTeamIDs []string
	rows, err := tx.Query(ctx,
		`SELECT id FROM teams WHERE business_id = $1 AND lead_user_id = $2 ORDER BY id FOR UPDATE`,
		businessID, departingUserID,
	)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ledTeamIDs = append(ledTeamIDs, id)
	}
	rows.Close()

	// Step 4: Require explicit replacement leads or explicit restructuring
	if len(ledTeamIDs) > 0 {
		if replacementLeadID != nil && *replacementLeadID != "" {
			// Verify replacement lead is valid member
			var repRole string
			var repSuspended bool
			err = tx.QueryRow(ctx,
				`SELECT role, is_suspended FROM memberships WHERE business_id = $1 AND user_id = $2`,
				businessID, *replacementLeadID,
			).Scan(&repRole, &repSuspended)
			if err != nil || repSuspended || (repRole != "owner" && repRole != "team_lead") {
				return fmt.Errorf("invalid replacement team lead")
			}
			_, err = tx.Exec(ctx,
				`UPDATE teams SET lead_user_id = $3, updated_at = now() WHERE business_id = $1 AND lead_user_id = $2`,
				businessID, departingUserID, *replacementLeadID,
			)
			if err != nil {
				return err
			}
		} else if allowUnassignedTeams {
			_, err = tx.Exec(ctx,
				`UPDATE teams SET lead_user_id = NULL, updated_at = now() WHERE business_id = $1 AND lead_user_id = $2`,
				businessID, departingUserID,
			)
			if err != nil {
				return err
			}
		} else {
			return ErrReplacementRequired
		}
	}

	// Step 5: Remove team membership assignments
	_, err = tx.Exec(ctx,
		`DELETE FROM team_members WHERE business_id = $1 AND user_id = $2`,
		businessID, departingUserID,
	)
	if err != nil {
		return err
	}

	// Step 6: Revoke delegated permissions and downstream permissions derived from their grants
	_, err = tx.Exec(ctx,
		`WITH RECURSIVE delegation_tree AS (
			SELECT id, user_id, permission
			  FROM user_permissions
			 WHERE business_id = $1 AND granted_by_user_id = $2
			UNION ALL
			SELECT c.id, c.user_id, c.permission
			  FROM user_permissions c
			  JOIN delegation_tree p 
			    ON c.granted_by_user_id = p.user_id 
			   AND c.permission = p.permission
			 WHERE c.business_id = $1
		)
		DELETE FROM user_permissions WHERE id IN (SELECT id FROM delegation_tree)`,
		businessID, departingUserID,
	)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`DELETE FROM user_permissions WHERE business_id = $1 AND user_id = $2`,
		businessID, departingUserID,
	)
	if err != nil {
		return err
	}

	// Step 7: Record all changes in organization_audit_logs
	err = s.AppendAuditLogTx(ctx, tx, businessID, &actorUserID, &departingUserID, nil, "member.removed", map[string]any{
		"departing_role":         departingRole,
		"reassigned_teams":       ledTeamIDs,
		"replacement_lead_id":    replacementLeadID,
		"allow_unassigned_teams": allowUnassignedTeams,
	})
	if err != nil {
		return err
	}

	// Step 8: Delete the business membership only after all dependent operations succeed
	_, err = tx.Exec(ctx,
		`DELETE FROM memberships WHERE business_id = $1 AND user_id = $2`,
		businessID, departingUserID,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------
// Permissions CRUD
// ---------------------------------------------------------------------

// ListUserPermissions returns all granular permissions held by a user in a business.
func (s *Store) ListUserPermissions(ctx context.Context, businessID, userID string) ([]UserPermission, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, business_id, user_id, permission, delegable, granted_by_user_id, created_at
		   FROM user_permissions
		  WHERE business_id = $1 AND user_id = $2
		  ORDER BY permission`,
		businessID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []UserPermission{}
	for rows.Next() {
		var p UserPermission
		if err := rows.Scan(&p.ID, &p.BusinessID, &p.UserID, &p.Permission, &p.Delegable, &p.GrantedByUserID, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GrantPermission grants or updates a permission on a user.
func (s *Store) GrantPermission(ctx context.Context, businessID, granterUserID, targetUserID, permission string, delegable bool) error {
	if !ValidPermissions[permission] {
		return ErrInvalidPermission
	}

	granterRole, granterSuspended, err := s.GetMemberRole(ctx, businessID, granterUserID)
	if err != nil {
		return err
	}
	if granterSuspended {
		return ErrSuspended
	}

	// Check granter authority: owner has full delegation authority; team_lead must have this permission and delegable = true
	if granterRole != "owner" {
		var granterCanDelegate bool
		err = s.pool.QueryRow(ctx,
			`SELECT delegable FROM user_permissions WHERE business_id = $1 AND user_id = $2 AND permission = $3`,
			businessID, granterUserID, permission,
		).Scan(&granterCanDelegate)
		if err != nil || !granterCanDelegate {
			return ErrDelegationDisallowed
		}
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO user_permissions (business_id, user_id, permission, delegable, granted_by_user_id)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (business_id, user_id, permission)
		 DO UPDATE SET delegable = EXCLUDED.delegable, granted_by_user_id = EXCLUDED.granted_by_user_id`,
		businessID, targetUserID, permission, delegable, granterUserID,
	)
	if err != nil {
		return err
	}

	_ = s.AppendAuditLog(ctx, businessID, &granterUserID, &targetUserID, nil, "permission.granted", map[string]any{
		"permission": permission,
		"delegable":  delegable,
	})

	return nil
}

// RevokePermission revokes a permission and cascades down any sub-delegations originatng from this user.
func (s *Store) RevokePermission(ctx context.Context, businessID, revokerUserID, targetUserID, permission string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Step 1: Cascade-revoke downstream sub-delegations
	_, err = tx.Exec(ctx,
		`WITH RECURSIVE delegation_tree AS (
			SELECT id, user_id, permission
			  FROM user_permissions
			 WHERE business_id = $1 AND granted_by_user_id = $2 AND permission = $3
			UNION ALL
			SELECT c.id, c.user_id, c.permission
			  FROM user_permissions c
			  JOIN delegation_tree p 
			    ON c.granted_by_user_id = p.user_id 
			   AND c.permission = p.permission
			 WHERE c.business_id = $1
		)
		DELETE FROM user_permissions WHERE id IN (SELECT id FROM delegation_tree)`,
		businessID, targetUserID, permission,
	)
	if err != nil {
		return err
	}

	// Step 2: Delete target's permission
	_, err = tx.Exec(ctx,
		`DELETE FROM user_permissions WHERE business_id = $1 AND user_id = $2 AND permission = $3`,
		businessID, targetUserID, permission,
	)
	if err != nil {
		return err
	}

	err = s.AppendAuditLogTx(ctx, tx, businessID, &revokerUserID, &targetUserID, nil, "permission.revoked", map[string]any{
		"permission": permission,
	})
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------
// Audit Logs
// ---------------------------------------------------------------------

// AppendAuditLogTx appends an immutable audit log row inside an active transaction.
func (s *Store) AppendAuditLogTx(ctx context.Context, tx pgx.Tx, businessID string, actorUserID, targetUserID, targetTeamID *string, action string, details map[string]any) error {
	var actorEmail, actorDisplayName *string
	if actorUserID != nil {
		var email, name string
		if err := tx.QueryRow(ctx, `SELECT COALESCE(email, username, ''), display_name FROM users WHERE id = $1`, *actorUserID).Scan(&email, &name); err == nil {
			actorEmail = &email
			actorDisplayName = &name
		}
	}

	var targetEmail, targetDisplayName *string
	if targetUserID != nil {
		var email, name string
		if err := tx.QueryRow(ctx, `SELECT COALESCE(email, username, ''), display_name FROM users WHERE id = $1`, *targetUserID).Scan(&email, &name); err == nil {
			targetEmail = &email
			targetDisplayName = &name
		}
	}

	var targetTeamName *string
	if targetTeamID != nil {
		var name string
		if err := tx.QueryRow(ctx, `SELECT name FROM teams WHERE id = $1`, *targetTeamID).Scan(&name); err == nil {
			targetTeamName = &name
		}
	}

	detailsJSON, _ := json.Marshal(details)
	if details == nil {
		detailsJSON = []byte("{}")
	}

	_, err := tx.Exec(ctx,
		`INSERT INTO organization_audit_logs (
			business_id, actor_user_id, actor_email, actor_display_name,
			target_user_id, target_email, target_display_name,
			target_team_id, target_team_name, action, details
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		businessID, actorUserID, actorEmail, actorDisplayName,
		targetUserID, targetEmail, targetDisplayName,
		targetTeamID, targetTeamName, action, detailsJSON,
	)
	return err
}

// AppendAuditLog writes an audit log row using the pool.
func (s *Store) AppendAuditLog(ctx context.Context, businessID string, actorUserID, targetUserID, targetTeamID *string, action string, details map[string]any) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.AppendAuditLogTx(ctx, tx, businessID, actorUserID, targetUserID, targetTeamID, action, details); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ListAuditLogs returns recent audit log entries for a business.
func (s *Store) ListAuditLogs(ctx context.Context, businessID string, limit, offset int) ([]OrgAuditLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, business_id, actor_user_id, actor_email, actor_display_name,
		        target_user_id, target_email, target_display_name,
		        target_team_id, target_team_name, action, details, created_at
		   FROM organization_audit_logs
		  WHERE business_id = $1
		  ORDER BY created_at DESC
		  LIMIT $2 OFFSET $3`,
		businessID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []OrgAuditLog{}
	for rows.Next() {
		var log OrgAuditLog
		if err := rows.Scan(
			&log.ID, &log.BusinessID, &log.ActorUserID, &log.ActorEmail, &log.ActorDisplayName,
			&log.TargetUserID, &log.TargetEmail, &log.TargetDisplayName,
			&log.TargetTeamID, &log.TargetTeamName, &log.Action, &log.Details, &log.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, log)
	}
	return out, rows.Err()
}
