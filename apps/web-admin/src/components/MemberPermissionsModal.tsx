import { useEffect, useState } from "react";
import { grantPermission, listMemberPermissions, revokePermission } from "../api/endpoints";
import type { OrgMember, RbacPermission, UserPermission } from "../api/types";
import { Modal, Notice, Spinner } from "./ui";

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

export function MemberPermissionsModal({
  businessId,
  member,
  onClose,
}: {
  businessId: string;
  member: OrgMember;
  onClose: () => void;
}) {
  const [permissions, setPermissions] = useState<UserPermission[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [toggling, setToggling] = useState<string | null>(null);

  const loadPermissions = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await listMemberPermissions(businessId, member.user_id);
      setPermissions(res.permissions || []);
    } catch (err: any) {
      setError(err?.message || "Failed to load permissions");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadPermissions();
  }, [businessId, member.user_id]);

  const permMap = new Map<string, UserPermission>();
  permissions.forEach((p) => permMap.set(p.permission, p));

  const handleToggle = async (permKey: RbacPermission) => {
    setToggling(permKey);
    setError(null);
    const existing = permMap.get(permKey);
    try {
      if (existing) {
        await revokePermission(businessId, member.user_id, permKey);
      } else {
        await grantPermission(businessId, member.user_id, {
          permission: permKey,
          delegable: false,
        });
      }
      await loadPermissions();
    } catch (err: any) {
      setError(err?.message || "Failed to update permission");
    } finally {
      setToggling(null);
    }
  };

  const handleToggleDelegable = async (perm: UserPermission) => {
    setToggling(perm.permission);
    setError(null);
    try {
      await grantPermission(businessId, member.user_id, {
        permission: perm.permission as RbacPermission,
        delegable: !perm.delegable,
      });
      await loadPermissions();
    } catch (err: any) {
      setError(err?.message || "Failed to update delegability");
    } finally {
      setToggling(null);
    }
  };

  return (
    <Modal title={`Permissions · ${member.display_name}`} wide onClose={onClose}>
      <div className="permissions-manager">
        {error && <Notice kind="danger">{error}</Notice>}

        <p className="permissions-manager__intro">
          Configure granular RBAC permissions for this {member.role === "team_lead" ? "Team Lead" : "member"}.
          Only permissions marked as <strong>Delegable</strong> can be granted downstream by this user.
        </p>

        {loading ? (
          <div className="admin-loading-box">
            <Spinner />
            <p>Loading permissions...</p>
          </div>
        ) : (
          <div className="permissions-list">
            {ALL_PERMISSIONS.map((p) => {
              const active = permMap.get(p.key);
              const isBusy = toggling === p.key;

              return (
                <div key={p.key} className={`permission-card ${active ? "active" : ""}`}>
                  <div className="permission-card__left">
                    <span className="permission-card__title">{p.label}</span>
                    <span className="permission-card__desc">{p.desc}</span>
                  </div>

                  <div className="permission-card__controls">
                    {active && (
                      <button
                        type="button"
                        className={`badge-toggle ${active.delegable ? "active" : ""}`}
                        onClick={() => handleToggleDelegable(active)}
                        disabled={isBusy}
                        title="Allow user to delegate this permission to subordinates"
                      >
                        {active.delegable ? "✓ Delegable" : "Not Delegable"}
                      </button>
                    )}
                    <button
                      type="button"
                      className={`btn btn--sm ${active ? "btn--danger-subtle" : "btn--primary-subtle"}`}
                      onClick={() => handleToggle(p.key)}
                      disabled={isBusy}
                    >
                      {isBusy ? "Updating..." : active ? "Revoke" : "Grant"}
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        )}

        <div className="modal-actions">
          <button type="button" className="btn btn--subtle" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </Modal>
  );
}
