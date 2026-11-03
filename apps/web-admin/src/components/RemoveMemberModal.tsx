import { useState } from "react";
import { removeMember } from "../api/endpoints";
import type { OrgMember } from "../api/types";
import { Modal, Notice } from "./ui";

export function RemoveMemberModal({
  businessId,
  member,
  candidates,
  onClose,
  onSuccess,
}: {
  businessId: string;
  member: OrgMember;
  candidates: OrgMember[];
  onClose: () => void;
  onSuccess: () => void;
}) {
  const [replacementLeadId, setReplacementLeadId] = useState("");
  const [allowUnassigned, setAllowUnassigned] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const leadsTeams = member.is_team_lead || (member.led_team_ids && member.led_team_ids.length > 0);

  // Eligible replacement leads: active members who are owners or team_leads, excluding departing member
  const eligibleReplacements = candidates.filter(
    (c) => c.user_id !== member.user_id && !c.is_suspended && (c.role === "owner" || c.role === "team_lead"),
  );

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (leadsTeams && !replacementLeadId && !allowUnassigned) {
      setError("Please select a replacement Team Lead or check 'Allow teams to remain unassigned'");
      return;
    }

    setBusy(true);
    setError(null);
    try {
      await removeMember(businessId, member.user_id, {
        replacement_lead_id: replacementLeadId || undefined,
        allow_unassigned_teams: allowUnassigned,
      });
      onSuccess();
    } catch (err: any) {
      setError(err?.message || "Failed to remove member");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title={`Remove ${member.display_name} from Organization`} onClose={onClose}>
      <form onSubmit={handleSubmit} className="admin-form">
        {error && <Notice kind="danger">{error}</Notice>}

        <p className="remove-member-warning">
          Are you sure you want to remove <strong>{member.display_name}</strong> from this business?
        </p>

        <div className="removal-workflow-guarantees">
          <h4>Transactional Removal Guarantees:</h4>
          <ul>
            <li>Global account identity, login credentials, and other businesses remain intact.</li>
            <li>All team assignments in this business will be cleanly unlinked.</li>
            <li>Delegated permissions granted by this member will cascade-revoke down the tree.</li>
            <li>An immutable record will be appended to Organizational Audit Logs.</li>
          </ul>
        </div>

        {leadsTeams && (
          <div className="team-lead-restructure-box">
            <h4 className="restructure-title">⚠️ Departing Member Leads Teams</h4>
            <p className="restructure-desc">
              This member is designated as the Team Lead for one or more departments. You must choose how to handle their leadership:
            </p>

            <div className="form-group">
              <label htmlFor="repLead" className="form-label">
                Designate Replacement Team Lead:
              </label>
              <select
                id="repLead"
                className="input"
                value={replacementLeadId}
                onChange={(e) => {
                  setReplacementLeadId(e.target.value);
                  if (e.target.value) setAllowUnassigned(false);
                }}
                disabled={allowUnassigned}
              >
                <option value="">Select replacement lead...</option>
                {eligibleReplacements.map((r) => (
                  <option key={r.user_id} value={r.user_id}>
                    {r.display_name} ({r.role})
                  </option>
                ))}
              </select>
            </div>

            <div className="form-group checkbox-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  checked={allowUnassigned}
                  onChange={(e) => {
                    setAllowUnassigned(e.target.checked);
                    if (e.target.checked) setReplacementLeadId("");
                  }}
                />
                <span>Allow teams to remain temporarily without a Team Lead</span>
              </label>
            </div>
          </div>
        )}

        <div className="modal-actions">
          <button type="button" className="btn btn--subtle" onClick={onClose} disabled={busy}>
            Cancel
          </button>
          <button type="submit" className="btn btn--danger" disabled={busy}>
            {busy ? "Removing..." : "Confirm & Remove Member"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
