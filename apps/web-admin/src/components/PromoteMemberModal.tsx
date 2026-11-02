import { useState } from "react";
import { promoteMember } from "../api/endpoints";
import type { OrgMember, RbacPermission, TeamWithDetails } from "../api/types";
import { Modal, Notice } from "./ui";

const ALL_PERMISSIONS: { key: RbacPermission; label: string; desc: string }[] = [
  { key: "can_view_team", label: "View Team & Members", desc: "Access assigned team roster and employee profiles" },
  { key: "can_view_reports", label: "View Team Reports", desc: "Access activity, keystroke and browser usage analytics" },
  { key: "can_view_screenshots", label: "View Screenshots", desc: "Access screenshot galleries for authorized team members" },
  { key: "can_assign_employees", label: "Assign Employees", desc: "Place employees into authorized teams" },
  { key: "can_manage_team", label: "Manage Teams", desc: "Create and configure subordinate team structures" },
  { key: "can_manage_screenshot_policy", label: "Configure Capture Policy", desc: "Adjust capture intervals and tracking overrides" },
  { key: "can_manage_employees", label: "Manage Employees", desc: "Manage assigned employee profiles and details" },
  { key: "can_create_team_leads", label: "Create Subordinate Leads", desc: "Promote employees to subordinate team leads" },
  { key: "can_view_audit_logs", label: "View Audit Logs", desc: "Access the organizational audit trail" },
];

export function PromoteMemberModal({
  businessId,
  member,
  teams,
  onClose,
  onSuccess,
}: {
  businessId: string;
  member: OrgMember;
  teams: TeamWithDetails[];
  onClose: () => void;
  onSuccess: () => void;
}) {
  const [teamId, setTeamId] = useState(member.team_id || "");
  const [selectedPerms, setSelectedPerms] = useState<RbacPermission[]>([
    "can_view_team",
    "can_view_reports",
    "can_view_screenshots",
  ]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const togglePerm = (perm: RbacPermission) => {
    setSelectedPerms((prev) =>
      prev.includes(perm) ? prev.filter((p) => p !== perm) : [...prev, perm],
    );
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await promoteMember(businessId, member.user_id, {
        team_id: teamId || undefined,
        permissions: selectedPerms,
      });
      onSuccess();
    } catch (err: any) {
      setError(err?.message || "Failed to promote member");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title={`Promote ${member.display_name} to Team Lead`} wide onClose={onClose}>
      <form onSubmit={handleSubmit} className="admin-form">
        {error && <Notice kind="danger">{error}</Notice>}

        <p className="form-info-banner">
          <strong>Identity Preserved:</strong> The user&apos;s existing login credentials, desktop app pairing,
          and historical tracking data are completely preserved.
        </p>

        <div className="form-group">
          <label htmlFor="leadTeam" className="form-label">
            Assign Team Leadership (Optional)
          </label>
          <select
            id="leadTeam"
            className="input"
            value={teamId}
            onChange={(e) => setTeamId(e.target.value)}
          >
            <option value="">Do not assign as team lead right now</option>
            {teams.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name} {t.lead_display_name ? `(Current lead: ${t.lead_display_name})` : "(No lead)"}
              </option>
            ))}
          </select>
          <span className="form-hint">
            The member will immediately be designated as the authorized leader of this team.
          </span>
        </div>

        <div className="form-group">
          <label className="form-label">Grant Delegated Permissions</label>
          <div className="permissions-checklist">
            {ALL_PERMISSIONS.map((p) => {
              const checked = selectedPerms.includes(p.key);
              return (
                <label key={p.key} className={`permission-item ${checked ? "active" : ""}`}>
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={() => togglePerm(p.key)}
                  />
                  <div className="permission-item__text">
                    <span className="permission-item__title">{p.label}</span>
                    <span className="permission-item__desc">{p.desc}</span>
                  </div>
                </label>
              );
            })}
          </div>
        </div>

        <div className="modal-actions">
          <button type="button" className="btn btn--subtle" onClick={onClose} disabled={busy}>
            Cancel
          </button>
          <button type="submit" className="btn btn--primary" disabled={busy}>
            {busy ? "Promoting..." : "Promote to Team Lead"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
