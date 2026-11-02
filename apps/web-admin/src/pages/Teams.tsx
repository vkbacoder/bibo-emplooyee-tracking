import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  assignTeamMember,
  createTeam,
  deleteTeam,
  listOrgMembers,
  listTeams,
  removeTeamMember,
  updateTeam,
} from "../api/endpoints";
import type { OrgMember, TeamWithDetails } from "../api/types";
import { Empty, Modal, Notice, Spinner } from "../components/ui";
import { useBusinesses } from "../useBusinesses";
import { useMyAccess } from "../useMyAccess";

// ── Icons ─────────────────────────────────────────────────────────────
const svg = (children: ReactNode, size = 18) => (
  <svg
    viewBox="0 0 24 24"
    width={size}
    height={size}
    fill="none"
    stroke="currentColor"
    strokeWidth={2}
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden
  >
    {children}
  </svg>
);

const IconPlus = svg(<><path d="M5 12h14" /><path d="M12 5v14" /></>);
const IconEdit = svg(<><path d="M17 3a2.85 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z" /></>);
const IconTrash = svg(<><path d="M3 6h18" /><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6" /><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2" /></>);
const IconUsers = svg(<><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" /><circle cx="9" cy="7" r="4" /><path d="M22 21v-2a4 4 0 0 0-3-3.87" /><path d="M16 3.13a4 4 0 0 1 0 7.75" /></>);
const IconCrown = svg(<><path d="m2 4 3 12h14l3-12-6 7-4-7-4 7-6-7zm3 16h14" /></>);
const IconFolderTree = svg(<><path d="M20 10c0-4.4-3.6-8-8-8s-8 3.6-8 8" /><path d="M4 10v10" /><path d="M20 10v10" /><path d="M4 14h16" /><path d="M4 18h16" /></>);
const IconChevronDown = svg(<path d="m6 9 6 6 6-6" />, 14);
const IconChevronRight = svg(<path d="m9 18 6-6-6-6" />, 14);
const IconCornerDownRight = svg(<><path d="m15 10 5 5-5 5" /><path d="M4 4v7a4 4 0 0 0 4 4h12" /></>, 14);

export function Teams() {
  const { selectedId } = useBusinesses();
  const { isOwner, can } = useMyAccess();

  const [teams, setTeams] = useState<TeamWithDetails[]>([]);
  const [members, setMembers] = useState<OrgMember[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Modals state
  const [createOpen, setCreateOpen] = useState(false);
  const [editingTeam, setEditingTeam] = useState<TeamWithDetails | null>(null);
  const [deletingTeam, setDeletingTeam] = useState<TeamWithDetails | null>(null);
  const [managingMembersTeam, setManagingMembersTeam] = useState<TeamWithDetails | null>(null);

  // Form states
  const [teamName, setTeamName] = useState("");
  const [teamDesc, setTeamDesc] = useState("");
  const [teamLeadId, setTeamLeadId] = useState("");
  const [parentTeamId, setParentTeamId] = useState("");
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  // Assign member form
  const [selectedUserToAssign, setSelectedUserToAssign] = useState("");
  const [assigning, setAssigning] = useState(false);

  // View mode: 'tree' | 'grid'
  const [viewMode, setViewMode] = useState<"tree" | "grid">("tree");
  const [collapsedNodes, setCollapsedNodes] = useState<Record<string, boolean>>({});

  const canManageTeams = isOwner || can("can_manage_team");
  const canAssignEmployees = isOwner || can("can_assign_employees");

  const loadData = async (bizId: string) => {
    setLoading(true);
    setError(null);
    try {
      const [teamsRes, membersRes] = await Promise.all([
        listTeams(bizId),
        listOrgMembers(bizId).catch(() => ({ members: [] })),
      ]);
      setTeams(teamsRes.teams);
      setMembers(membersRes.members);
    } catch (err: any) {
      setError(err?.message || "Failed to load teams");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (selectedId) {
      loadData(selectedId);
    } else {
      setTeams([]);
      setMembers([]);
    }
  }, [selectedId]);

  // Derived metrics
  const totalTeams = teams.length;
  const totalLeads = useMemo(() => {
    const set = new Set<string>();
    teams.forEach((t) => {
      if (t.lead_user_id) set.add(t.lead_user_id);
    });
    return set.size;
  }, [teams]);
  const totalAssignedMembers = useMemo(() => {
    return members.filter((m) => m.team_id).length;
  }, [members]);
  const rootTeams = useMemo(() => teams.filter((t) => !t.parent_team_id), [teams]);

  // Open Create Modal
  const openCreateModal = () => {
    setTeamName("");
    setTeamDesc("");
    setTeamLeadId("");
    setParentTeamId("");
    setFormError(null);
    setCreateOpen(true);
  };

  // Open Edit Modal
  const openEditModal = (t: TeamWithDetails) => {
    setEditingTeam(t);
    setTeamName(t.name);
    setTeamDesc(t.description || "");
    setTeamLeadId(t.lead_user_id || "");
    setParentTeamId(t.parent_team_id || "");
    setFormError(null);
  };

  const handleSaveTeam = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedId) return;
    if (!teamName.trim()) {
      setFormError("Team name is required");
      return;
    }
    setSaving(true);
    setFormError(null);
    try {
      if (editingTeam) {
        await updateTeam(selectedId, editingTeam.id, {
          name: teamName.trim(),
          description: teamDesc.trim() || undefined,
          lead_user_id: teamLeadId || undefined,
          parent_team_id: parentTeamId || undefined,
        });
        setEditingTeam(null);
      } else {
        await createTeam(selectedId, {
          name: teamName.trim(),
          description: teamDesc.trim() || undefined,
          lead_user_id: teamLeadId || undefined,
          parent_team_id: parentTeamId || undefined,
        });
        setCreateOpen(false);
      }
      await loadData(selectedId);
    } catch (err: any) {
      setFormError(err?.message || "Failed to save team");
    } finally {
      setSaving(false);
    }
  };

  const handleDeleteTeam = async () => {
    if (!selectedId || !deletingTeam) return;
    setSaving(true);
    try {
      await deleteTeam(selectedId, deletingTeam.id);
      setDeletingTeam(null);
      await loadData(selectedId);
    } catch (err: any) {
      alert(err?.message || "Failed to delete team");
    } finally {
      setSaving(false);
    }
  };

  const handleAssignMember = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedId || !managingMembersTeam || !selectedUserToAssign) return;
    setAssigning(true);
    try {
      await assignTeamMember(selectedId, managingMembersTeam.id, selectedUserToAssign);
      setSelectedUserToAssign("");
      await loadData(selectedId);
    } catch (err: any) {
      alert(err?.message || "Failed to assign member");
    } finally {
      setAssigning(false);
    }
  };

  const handleRemoveMember = async (userId: string) => {
    if (!selectedId || !managingMembersTeam) return;
    try {
      await removeTeamMember(selectedId, managingMembersTeam.id, userId);
      await loadData(selectedId);
    } catch (err: any) {
      alert(err?.message || "Failed to remove member");
    }
  };

  const toggleCollapse = (id: string) => {
    setCollapsedNodes((prev) => ({ ...prev, [id]: !prev[id] }));
  };

  // Build Hierarchy Tree
  const childrenMap = useMemo(() => {
    const map = new Map<string, TeamWithDetails[]>();
    teams.forEach((t) => {
      const pid = t.parent_team_id || "root";
      const list = map.get(pid) || [];
      list.push(t);
      map.set(pid, list);
    });
    return map;
  }, [teams]);

  // Candidates for team lead: active members who are owners or team_leads
  const leadCandidates = useMemo(() => {
    return members.filter((m) => !m.is_suspended && (m.role === "owner" || m.role === "team_lead"));
  }, [members]);

  // Available parent teams (avoid setting self as parent)
  const availableParents = useMemo(() => {
    if (!editingTeam) return teams;
    return teams.filter((t) => t.id !== editingTeam.id);
  }, [teams, editingTeam]);

  // Current team's assigned members
  const teamMembers = useMemo(() => {
    if (!managingMembersTeam) return [];
    return members.filter((m) => m.team_id === managingMembersTeam.id);
  }, [members, managingMembersTeam]);

  // Unassigned active members
  const unassignedMembers = useMemo(() => {
    return members.filter((m) => !m.is_suspended && !m.team_id);
  }, [members]);

  // Recursive Tree Node Renderer
  const renderTreeNode = (team: TeamWithDetails, level = 0) => {
    const children = childrenMap.get(team.id) || [];
    const hasChildren = children.length > 0;
    const isCollapsed = !!collapsedNodes[team.id];

    return (
      <div key={team.id} className="hierarchy-node" style={{ marginLeft: level > 0 ? "24px" : "0" }}>
        <div className="hierarchy-card bibo-card bibo-card--default">
          <div className="hierarchy-card__header">
            <div className="hierarchy-card__title-row">
              {hasChildren && (
                <button
                  type="button"
                  className="hierarchy-collapse-btn"
                  onClick={() => toggleCollapse(team.id)}
                  title={isCollapsed ? "Expand" : "Collapse"}
                >
                  {isCollapsed ? IconChevronRight : IconChevronDown}
                </button>
              )}
              {!hasChildren && level > 0 && (
                <span className="hierarchy-connector-dot">{IconCornerDownRight}</span>
              )}
              <h3 className="hierarchy-team-name">{team.name}</h3>
              {team.parent_team_name && (
                <span className="badge badge--subtle">Part of {team.parent_team_name}</span>
              )}
            </div>

            <div className="hierarchy-card__actions">
              <button
                type="button"
                className="btn btn--subtle btn--sm"
                onClick={() => setManagingMembersTeam(team)}
                title="Manage Members"
              >
                {IconUsers}
                <span>{team.member_count} Members</span>
              </button>
              {canManageTeams && (
                <>
                  <button
                    type="button"
                    className="btn btn--subtle btn--sm"
                    onClick={() => openEditModal(team)}
                    title="Edit Team"
                  >
                    {IconEdit}
                  </button>
                  <button
                    type="button"
                    className="btn btn--danger-subtle btn--sm"
                    onClick={() => setDeletingTeam(team)}
                    title="Delete Team"
                  >
                    {IconTrash}
                  </button>
                </>
              )}
            </div>
          </div>

          {team.description && (
            <p className="hierarchy-card__desc">{team.description}</p>
          )}

          <div className="hierarchy-card__footer">
            <div className="hierarchy-lead-info">
              {team.lead_user_id ? (
                <div className="badge badge--primary">
                  <span className="badge-icon">{IconCrown}</span>
                  <span>Lead: <strong>{team.lead_display_name || team.lead_email}</strong></span>
                </div>
              ) : (
                <span className="badge badge--neutral">No Lead Assigned</span>
              )}
            </div>
            {hasChildren && (
              <span className="hierarchy-subteam-count">
                {children.length} {children.length === 1 ? "subteam" : "subteams"}
              </span>
            )}
          </div>
        </div>

        {hasChildren && !isCollapsed && (
          <div className="hierarchy-children">
            {children.map((child) => renderTreeNode(child, level + 1))}
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="page-teams">
      {/* Page Header */}
      <div className="admin-page-header">
        <div className="admin-page-header__main">
          <h1 className="admin-page-title">Teams & Hierarchy</h1>
          <p className="admin-page-subtitle">
            Structure departments, assign Team Leads, and organize employee reporting chains.
          </p>
        </div>
        <div className="admin-page-header__actions">
          <div className="view-mode-toggle">
            <button
              type="button"
              className={`view-mode-btn ${viewMode === "tree" ? "active" : ""}`}
              onClick={() => setViewMode("tree")}
            >
              Tree
            </button>
            <button
              type="button"
              className={`view-mode-btn ${viewMode === "grid" ? "active" : ""}`}
              onClick={() => setViewMode("grid")}
            >
              Grid
            </button>
          </div>
          {canManageTeams && (
            <button type="button" className="btn btn--primary" onClick={openCreateModal}>
              {IconPlus}
              <span>Create Team</span>
            </button>
          )}
        </div>
      </div>

      {error && <Notice kind="danger">{error}</Notice>}

      {/* Metrics Row */}
      <div className="admin-stats-grid">
        <div className="bibo-card bibo-card--default admin-cardpad">
          <div className="bibo-stat">
            <div className="bibo-stat__top">
              <div className="bibo-stat__icon">{IconFolderTree}</div>
              <div className="bibo-stat__label">Total Teams</div>
            </div>
            <div className="bibo-stat__value">{totalTeams}</div>
            <div className="bibo-stat__foot">
              <span className="bibo-stat__sub">{rootTeams.length} top-level departments</span>
            </div>
          </div>
        </div>

        <div className="bibo-card bibo-card--default admin-cardpad">
          <div className="bibo-stat">
            <div className="bibo-stat__top">
              <div className="bibo-stat__icon">{IconCrown}</div>
              <div className="bibo-stat__label">Active Team Leads</div>
            </div>
            <div className="bibo-stat__value">{totalLeads}</div>
            <div className="bibo-stat__foot">
              <span className="bibo-stat__sub">Empowered managers</span>
            </div>
          </div>
        </div>

        <div className="bibo-card bibo-card--default admin-cardpad">
          <div className="bibo-stat">
            <div className="bibo-stat__top">
              <div className="bibo-stat__icon">{IconUsers}</div>
              <div className="bibo-stat__label">Assigned Members</div>
            </div>
            <div className="bibo-stat__value">{totalAssignedMembers}</div>
            <div className="bibo-stat__foot">
              <span className="bibo-stat__sub">
                {members.length - totalAssignedMembers} unassigned
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* Main Content */}
      {loading ? (
        <div className="admin-loading-box">
          <Spinner />
          <p>Loading organization hierarchy...</p>
        </div>
      ) : teams.length === 0 ? (
        <Empty>
          <h3>No teams established yet</h3>
          <p className="muted">Create your first team to organize employees into departments with dedicated Team Leads.</p>
          {canManageTeams && (
            <div style={{ marginTop: 16 }}>
              <button type="button" className="btn btn--primary" onClick={openCreateModal}>
                {IconPlus}
                <span>Create Team</span>
              </button>
            </div>
          )}
        </Empty>
      ) : viewMode === "tree" ? (
        <div className="hierarchy-tree-container">
          {rootTeams.map((root) => renderTreeNode(root, 0))}
        </div>
      ) : (
        <div className="teams-grid">
          {teams.map((t) => (
            <div key={t.id} className="bibo-card bibo-card--default team-grid-card">
              <div className="team-grid-card__header">
                <div>
                  <h3 className="team-grid-card__name">{t.name}</h3>
                  {t.parent_team_name && (
                    <span className="badge badge--subtle">Subteam of {t.parent_team_name}</span>
                  )}
                </div>
                {canManageTeams && (
                  <div className="team-grid-card__actions">
                    <button
                      type="button"
                      className="btn btn--subtle btn--sm"
                      onClick={() => openEditModal(t)}
                    >
                      {IconEdit}
                    </button>
                    <button
                      type="button"
                      className="btn btn--danger-subtle btn--sm"
                      onClick={() => setDeletingTeam(t)}
                    >
                      {IconTrash}
                    </button>
                  </div>
                )}
              </div>

              {t.description && <p className="team-grid-card__desc">{t.description}</p>}

              <div className="team-grid-card__stats">
                <div className="stat-pill">
                  <span className="stat-pill__label">Members</span>
                  <span className="stat-pill__val">{t.member_count}</span>
                </div>
                <div className="stat-pill">
                  <span className="stat-pill__label">Subteams</span>
                  <span className="stat-pill__val">{t.subteam_count}</span>
                </div>
              </div>

              <div className="team-grid-card__footer">
                <div className="hierarchy-lead-info">
                  {t.lead_user_id ? (
                    <span className="badge badge--primary">
                      {IconCrown}
                      <span>{t.lead_display_name || t.lead_email}</span>
                    </span>
                  ) : (
                    <span className="badge badge--neutral">Unassigned Lead</span>
                  )}
                </div>
                <button
                  type="button"
                  className="btn btn--subtle btn--sm"
                  onClick={() => setManagingMembersTeam(t)}
                >
                  {IconUsers}
                  <span>Manage</span>
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Create / Edit Team Modal */}
      {(createOpen || editingTeam) && (
        <Modal
          title={editingTeam ? "Edit Team" : "Create New Team"}
          onClose={() => {
            setCreateOpen(false);
            setEditingTeam(null);
          }}
        >
          <form onSubmit={handleSaveTeam} className="admin-form">
            {formError && <Notice kind="danger">{formError}</Notice>}

            <div className="form-group">
              <label htmlFor="teamName" className="form-label">
                Team Name <span className="req">*</span>
              </label>
              <input
                id="teamName"
                type="text"
                className="input"
                placeholder="e.g. Engineering, Sales, QA"
                value={teamName}
                onChange={(e) => setTeamName(e.target.value)}
                autoFocus
                required
              />
            </div>

            <div className="form-group">
              <label htmlFor="teamDesc" className="form-label">
                Description
              </label>
              <textarea
                id="teamDesc"
                className="input input--textarea"
                rows={3}
                placeholder="Brief summary of department goals or scope..."
                value={teamDesc}
                onChange={(e) => setTeamDesc(e.target.value)}
              />
            </div>

            <div className="form-group">
              <label htmlFor="parentTeam" className="form-label">
                Parent Team (Optional)
              </label>
              <select
                id="parentTeam"
                className="input"
                value={parentTeamId}
                onChange={(e) => setParentTeamId(e.target.value)}
              >
                <option value="">None (Top-Level Department)</option>
                {availableParents.map((pt) => (
                  <option key={pt.id} value={pt.id}>
                    {pt.name}
                  </option>
                ))}
              </select>
              <span className="form-hint">
                Hierarchical cycle prevention guarantees safe nesting without loops.
              </span>
            </div>

            <div className="form-group">
              <label htmlFor="teamLead" className="form-label">
                Team Lead (Optional)
              </label>
              <select
                id="teamLead"
                className="input"
                value={teamLeadId}
                onChange={(e) => setTeamLeadId(e.target.value)}
              >
                <option value="">None (Unassigned Lead)</option>
                {leadCandidates.map((lc) => (
                  <option key={lc.user_id} value={lc.user_id}>
                    {lc.display_name} ({lc.email || lc.username}) — {lc.role}
                  </option>
                ))}
              </select>
              <span className="form-hint">
                Only active Team Leads and Owners may be designated as leaders.
              </span>
            </div>

            <div className="modal-actions">
              <button
                type="button"
                className="btn btn--subtle"
                onClick={() => {
                  setCreateOpen(false);
                  setEditingTeam(null);
                }}
                disabled={saving}
              >
                Cancel
              </button>
              <button type="submit" className="btn btn--primary" disabled={saving}>
                {saving ? "Saving..." : editingTeam ? "Save Changes" : "Create Team"}
              </button>
            </div>
          </form>
        </Modal>
      )}

      {/* Delete Confirmation Modal */}
      {deletingTeam && (
        <Modal
          title="Delete Team"
          onClose={() => setDeletingTeam(null)}
        >
          <div className="delete-confirm-box">
            <p>
              Are you sure you want to delete <strong>{deletingTeam.name}</strong>?
            </p>
            <p className="delete-confirm-note">
              Members will become unassigned. No user accounts or tracked activity will be lost.
            </p>
            <div className="modal-actions">
              <button
                type="button"
                className="btn btn--subtle"
                onClick={() => setDeletingTeam(null)}
                disabled={saving}
              >
                Cancel
              </button>
              <button
                type="button"
                className="btn btn--danger"
                onClick={handleDeleteTeam}
                disabled={saving}
              >
                {saving ? "Deleting..." : "Delete Team"}
              </button>
            </div>
          </div>
        </Modal>
      )}

      {/* Manage Team Members Modal */}
      {managingMembersTeam && (
        <Modal
          title={`Members in ${managingMembersTeam.name}`}
          onClose={() => setManagingMembersTeam(null)}
        >
          <div className="team-members-manager">
            {canAssignEmployees && unassignedMembers.length > 0 && (
              <form onSubmit={handleAssignMember} className="assign-member-bar">
                <select
                  className="input input--sm"
                  value={selectedUserToAssign}
                  onChange={(e) => setSelectedUserToAssign(e.target.value)}
                >
                  <option value="">Select unassigned member to add...</option>
                  {unassignedMembers.map((um) => (
                    <option key={um.user_id} value={um.user_id}>
                      {um.display_name} ({um.email || um.username})
                    </option>
                  ))}
                </select>
                <button
                  type="submit"
                  className="btn btn--primary btn--sm"
                  disabled={!selectedUserToAssign || assigning}
                >
                  {assigning ? "Adding..." : "Add to Team"}
                </button>
              </form>
            )}

            <div className="team-members-list">
              {teamMembers.length === 0 ? (
                <p className="team-members-empty">No employees assigned to this team yet.</p>
              ) : (
                teamMembers.map((m) => (
                  <div key={m.user_id} className="team-member-row">
                    <div className="team-member-info">
                      <span className="team-member-name">{m.display_name}</span>
                      <span className="team-member-email">{m.email || m.username}</span>
                      {m.role === "team_lead" && (
                        <span className="badge badge--primary badge--xs">Team Lead</span>
                      )}
                      {m.user_id === managingMembersTeam.lead_user_id && (
                        <span className="badge badge--gold badge--xs">Lead</span>
                      )}
                    </div>
                    {canAssignEmployees && (
                      <button
                        type="button"
                        className="btn btn--subtle btn--xs"
                        onClick={() => handleRemoveMember(m.user_id)}
                        title="Remove from team"
                      >
                        Remove
                      </button>
                    )}
                  </div>
                ))
              )}
            </div>

            <div className="modal-actions">
              <button
                type="button"
                className="btn btn--subtle"
                onClick={() => setManagingMembersTeam(null)}
              >
                Close
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
