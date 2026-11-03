import { useEffect, useState, type ReactNode } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { Trans, useTranslation } from "react-i18next";
import {
  createBusiness,
  createEmployee,
  demoteMember,
  listOrgMembers,
  listTeams,
  reactivateMember,
  reportEmployees,
  suspendMember,
} from "../api/endpoints";
import {
  ApiError,
  type BusinessKind,
  type OrgMember,
  type TeamWithDetails,
} from "../api/types";
import { Empty, Modal, Notice, Spinner } from "../components/ui";
import { useBusinesses } from "../useBusinesses";
import { memberTerms, type MemberTerms } from "../terms";
import { useAuth } from "../auth/AuthContext";
import { useMyAccess } from "../useMyAccess";
import { PromoteMemberModal } from "../components/PromoteMemberModal";
import { MemberPermissionsModal } from "../components/MemberPermissionsModal";
import { RemoveMemberModal } from "../components/RemoveMemberModal";

// ── Icons ─────────────────────────────────────────────────────────────
const svg = (children: ReactNode) => (
  <svg
    viewBox="0 0 24 24"
    width="16"
    height="16"
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
const IconUserPlus = svg(<><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" /><circle cx="9" cy="7" r="4" /><line x1="19" x2="19" y1="8" y2="14" /><line x1="22" x2="16" y1="11" y2="11" /></>);
const IconArrowRight = svg(<><path d="M5 12h14" /><path d="m12 5 7 7-7 7" /></>);
const IconCrown = svg(<><path d="m2 4 3 12h14l3-12-6 7-4-7-4 7-6-7zm3 16h14" /></>);
const IconShield = svg(<><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10" /></>);
const IconClose = svg(<><path d="M18 6 6 18M6 6l12 12" /></>);
const IconDices = svg(<><rect width="12" height="12" x="2" y="10" rx="2" ry="2" /><path d="m17.92 14 3.5-3.5a2.24 2.24 0 0 0 0-3l-5-4.92a2.24 2.24 0 0 0-3 0L10 6" /><path d="M6 18h.01" /><path d="M10 14h.01" /><path d="M15 6h.01" /><path d="M18 9h.01" /></>);

function genTempPassword(): string {
  const chars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789";
  let out = "";
  const rnd = crypto.getRandomValues(new Uint32Array(10));
  for (let i = 0; i < 10; i++) out += chars[rnd[i] % chars.length];
  return out;
}

const AVATAR_PALETTE = [
  { bg: "var(--info-soft)", fg: "var(--info)" },
  { bg: "var(--positive-soft)", fg: "var(--positive)" },
  { bg: "color-mix(in srgb, var(--data-rose) 18%, transparent)", fg: "var(--data-rose)" },
  { bg: "color-mix(in srgb, var(--data-amber) 22%, transparent)", fg: "var(--data-amber)" },
];

const initials = (name: string) =>
  name.split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0]?.toUpperCase()).join("") || "?";

type Status = "active" | "idle" | "offline";
function memberStatus(lastSeen: number | null): Status {
  if (!lastSeen) return "offline";
  const ageS = Date.now() / 1000 - lastSeen;
  if (ageS < 5 * 60) return "active";
  if (ageS < 30 * 60) return "idle";
  return "offline";
}

export function Employees() {
  const { t } = useTranslation("dashboard");
  const { user } = useAuth();
  const {
    businesses,
    selected,
    selectedId,
    setSelectedId,
    loading: bizLoading,
    reload: reloadBiz,
  } = useBusinesses();
  const { isOwner, can } = useMyAccess();

  const [members, setMembers] = useState<OrgMember[]>([]);
  const [teams, setTeams] = useState<TeamWithDetails[]>([]);
  const [lastSeen, setLastSeen] = useState<Record<string, number | null>>({});
  const [loading, setLoading] = useState(false);
  const [listError, setListError] = useState<string | null>(null);

  // Modals state
  const [showBiz, setShowBiz] = useState(false);
  const [showEmp, setShowEmp] = useState(false);
  const [promotingMember, setPromotingMember] = useState<OrgMember | null>(null);
  const [managingPermsMember, setManagingPermsMember] = useState<OrgMember | null>(null);
  const [removingMember, setRemovingMember] = useState<OrgMember | null>(null);
  const [demotingMember, setDemotingMember] = useState<OrgMember | null>(null);
  const [suspendingMember, setSuspendingMember] = useState<OrgMember | null>(null);
  const [replacementLeadId, setReplacementLeadId] = useState("");
  const [autoCreatedNote, setAutoCreatedNote] = useState<string | null>(null);

  const terms = memberTerms(selected?.kind);
  const canManageEmployees = isOwner || can("can_manage_employees");
  const canCreateTeamLeads = isOwner || can("can_create_team_leads");

  function loadData(bizId: string) {
    setLoading(true);
    setListError(null);
    Promise.all([
      listOrgMembers(bizId),
      listTeams(bizId).catch(() => ({ teams: [] })),
      reportEmployees(bizId).catch(() => ({ employees: [] })),
    ])
      .then(([mRes, tRes, rRes]) => {
        setMembers(mRes.members || []);
        setTeams(tRes.teams || []);
        setLastSeen(Object.fromEntries(rRes.employees.map((e) => [e.id, e.last_seen])));
      })
      .catch((err) => {
        setListError(err?.message || t("employees.errorLoadMembers", { members: terms.lowerMany }));
      })
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    if (selectedId) loadData(selectedId);
    else {
      setMembers([]);
      setTeams([]);
    }
  }, [selectedId]);

  const [searchParams, setSearchParams] = useSearchParams();
  useEffect(() => {
    if (searchParams.get("new") === null) return;
    setShowBiz(true);
    searchParams.delete("new");
    setSearchParams(searchParams, { replace: true });
  }, [searchParams, setSearchParams]);

  const hasBusiness = businesses.length > 0;

  const handleDemote = async () => {
    if (!selectedId || !demotingMember) return;
    try {
      await demoteMember(selectedId, demotingMember.user_id, {
        replacement_lead_id: replacementLeadId || undefined,
      });
      setDemotingMember(null);
      setReplacementLeadId("");
      loadData(selectedId);
    } catch (err: any) {
      alert(err?.message || "Failed to demote member");
    }
  };

  const handleSuspend = async () => {
    if (!selectedId || !suspendingMember) return;
    try {
      await suspendMember(selectedId, suspendingMember.user_id, {
        replacement_lead_id: replacementLeadId || undefined,
      });
      setSuspendingMember(null);
      setReplacementLeadId("");
      loadData(selectedId);
    } catch (err: any) {
      alert(err?.message || "Failed to suspend member");
    }
  };

  const handleReactivate = async (member: OrgMember) => {
    if (!selectedId) return;
    try {
      await reactivateMember(selectedId, member.user_id);
      loadData(selectedId);
    } catch (err: any) {
      alert(err?.message || "Failed to reactivate member");
    }
  };

  return (
    <div className="admin-wrap" style={{ paddingBottom: 32 }}>
      {/* Header */}
      <div className="admin-pagehead">
        <div className="admin-pagehead__main">
          <h1 className="admin-h1">{terms.many}</h1>
          {selected && (
            <p className="admin-sub">
              {selected.name} · {members.length} {terms.many}
            </p>
          )}
        </div>
        <div className="admin-pagehead__actions">
          {isOwner && (
            <button className="bibo-btn bibo-btn--secondary" onClick={() => setShowBiz(true)}>
              <span style={{ display: "inline-flex", lineHeight: 0 }}>{IconPlus}</span>
              <span>{t("employees.newOrg", { org: terms.org })}</span>
            </button>
          )}
          {canManageEmployees && (
            <button className="bibo-btn bibo-btn--primary" onClick={() => setShowEmp(true)}>
              <span style={{ display: "inline-flex", lineHeight: 0 }}>{IconUserPlus}</span>
              <span>{terms.addCta}</span>
            </button>
          )}
        </div>
      </div>

      {!hasBusiness && !bizLoading && (
        <div style={{ marginBottom: 16 }}>
          <Notice kind="info">
            <Trans
              i18nKey="employees.noBusinessNotice"
              t={t}
              values={{ member: terms.lowerOne }}
              components={{ 1: <em /> }}
            />
          </Notice>
        </div>
      )}

      {autoCreatedNote && (
        <div style={{ marginBottom: 16 }}>
          <Notice kind="success">{autoCreatedNote}</Notice>
        </div>
      )}

      {bizLoading && <Spinner label={t("employees.loading")} />}
      {listError && <Notice kind="danger">{listError}</Notice>}
      {loading && <Spinner label={t("employees.loadingMembers", { members: terms.lowerMany })} />}

      {!loading && selectedId && members.length === 0 && !listError && (
        <Empty>{t("employees.noMembersYet", { members: terms.lowerMany })}</Empty>
      )}

      {/* Members Table */}
      {members.length > 0 && (
        <div className="bibo-card bibo-card--default admin-tablecard">
          <table className="admin-table admin-table--roster">
            <thead>
              <tr>
                <th>{t("employees.table.name")}</th>
                <th>Role & Scope</th>
                <th>Assigned Department</th>
                <th>Status</th>
                <th className="r">Actions</th>
              </tr>
            </thead>
            <tbody>
              {members.map((m, i) => {
                const pal = AVATAR_PALETTE[i % AVATAR_PALETTE.length];
                const isSelf = m.user_id === user?.id;
                const status = m.user_id in lastSeen ? memberStatus(lastSeen[m.user_id]) : null;

                return (
                  <tr key={m.user_id} className={m.is_suspended ? "row--suspended" : ""}>
                    <td>
                      <div className="admin-name">
                        <span className="bibo-avatar" style={{ ["--_s" as string]: "34px" }}>
                          <span
                            className="bibo-avatar__img"
                            aria-label={m.display_name}
                            style={{ background: pal.bg, color: pal.fg }}
                          >
                            {initials(m.display_name)}
                          </span>
                          {status && !m.is_suspended && (
                            <span className={`bibo-avatar__dot bibo-avatar__dot--${status}`} />
                          )}
                        </span>
                        <div className="admin-name__meta">
                          <span className="admin-name__txt">
                            {m.display_name}
                            {isSelf && <span className="admin-self">{t("dashboard.selfBadge")}</span>}
                          </span>
                          <span className="admin-login-sub">{m.email || m.username}</span>
                        </div>
                      </div>
                    </td>

                    <td>
                      <div className="role-cell">
                        {m.role === "owner" && (
                          <span className="badge badge--gold">
                            <span className="badge-icon">{IconCrown}</span>
                            <span>HR / Owner</span>
                          </span>
                        )}
                        {m.role === "team_lead" && (
                          <span className="badge badge--primary">
                            <span className="badge-icon">{IconShield}</span>
                            <span>Team Lead</span>
                          </span>
                        )}
                        {m.role === "employee" && (
                          <span className="badge badge--neutral">Employee</span>
                        )}
                      </div>
                    </td>

                    <td>
                      {m.team_name ? (
                        <span className="badge badge--subtle">{m.team_name}</span>
                      ) : (
                        <span className="text-muted text-xs">Unassigned</span>
                      )}
                    </td>

                    <td>
                      {m.is_suspended ? (
                        <span className="badge badge--danger">Suspended</span>
                      ) : status ? (
                        <span className={`status-pill status-pill--${status}`}>{status}</span>
                      ) : (
                        <span className="text-muted text-xs">—</span>
                      )}
                    </td>

                    <td className="r">
                      <div className="member-actions-row">
                        <Link
                          className="admin-viewlink"
                          to={`/employees/${m.user_id}?business=${selectedId}`}
                          title="View Reports"
                        >
                          Reports
                          {IconArrowRight}
                        </Link>

                        {/* Promote to Team Lead */}
                        {canCreateTeamLeads && m.role === "employee" && !m.is_suspended && (
                          <button
                            type="button"
                            className="btn btn--subtle btn--xs"
                            onClick={() => setPromotingMember(m)}
                            title="Promote to Team Lead"
                          >
                            Promote
                          </button>
                        )}

                        {/* Manage Permissions */}
                        {(isOwner || can("can_create_team_leads")) && m.role === "team_lead" && (
                          <button
                            type="button"
                            className="btn btn--subtle btn--xs"
                            onClick={() => setManagingPermsMember(m)}
                            title="Manage Delegated Permissions"
                          >
                            Permissions
                          </button>
                        )}

                        {/* Demote Team Lead */}
                        {isOwner && m.role === "team_lead" && (
                          <button
                            type="button"
                            className="btn btn--subtle btn--xs"
                            onClick={() => {
                              setReplacementLeadId("");
                              setDemotingMember(m);
                            }}
                            title="Demote to Employee"
                          >
                            Demote
                          </button>
                        )}

                        {/* Suspend / Reactivate */}
                        {isOwner && m.role !== "owner" && (
                          m.is_suspended ? (
                            <button
                              type="button"
                              className="btn btn--primary-subtle btn--xs"
                              onClick={() => handleReactivate(m)}
                              title="Reactivate Membership"
                            >
                              Reactivate
                            </button>
                          ) : (
                            <button
                              type="button"
                              className="btn btn--warn-subtle btn--xs"
                              onClick={() => {
                                setReplacementLeadId("");
                                setSuspendingMember(m);
                              }}
                              title="Suspend Membership"
                            >
                              Suspend
                            </button>
                          )
                        )}

                        {/* Remove Member */}
                        {isOwner && m.role !== "owner" && (
                          <button
                            type="button"
                            className="btn btn--danger-subtle btn--xs"
                            onClick={() => setRemovingMember(m)}
                            title="Remove from Organization"
                          >
                            Remove
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {/* Modals */}
      {showBiz && (
        <NewBusinessModal
          terms={terms}
          kind={selected?.kind}
          onClose={() => setShowBiz(false)}
          onCreated={async (id) => {
            setShowBiz(false);
            await reloadBiz();
            setSelectedId(id);
          }}
        />
      )}

      {showEmp && (
        <NewEmployeeModal
          businessId={selectedId}
          terms={terms}
          onClose={() => setShowEmp(false)}
          onCreated={async (newBusinessId, wasAutoCreated) => {
            setShowEmp(false);
            if (wasAutoCreated) {
              await reloadBiz();
              setSelectedId(newBusinessId);
              setAutoCreatedNote(t("employees.autoCreatedNote", { member: terms.lowerOne }));
            } else if (selectedId) {
              loadData(selectedId);
            }
          }}
        />
      )}

      {promotingMember && selectedId && (
        <PromoteMemberModal
          businessId={selectedId}
          member={promotingMember}
          teams={teams}
          onClose={() => setPromotingMember(null)}
          onSuccess={() => {
            setPromotingMember(null);
            loadData(selectedId);
          }}
        />
      )}

      {managingPermsMember && selectedId && (
        <MemberPermissionsModal
          businessId={selectedId}
          member={managingPermsMember}
          onClose={() => setManagingPermsMember(null)}
        />
      )}

      {removingMember && selectedId && (
        <RemoveMemberModal
          businessId={selectedId}
          member={removingMember}
          candidates={members}
          onClose={() => setRemovingMember(null)}
          onSuccess={() => {
            setRemovingMember(null);
            loadData(selectedId);
          }}
        />
      )}

      {/* Demote Confirmation Modal */}
      {demotingMember && (
        <Modal title={`Demote ${demotingMember.display_name}`} onClose={() => setDemotingMember(null)}>
          <div className="admin-form">
            <p>
              Are you sure you want to demote <strong>{demotingMember.display_name}</strong> to an Employee?
            </p>
            <p className="form-info-banner">
              Management and Web Admin access will be revoked. Their user account, password credentials,
              and historical desktop tracking data remain completely safe and untouched.
            </p>
            {demotingMember.is_team_lead && (
              <div className="form-group">
                <label htmlFor="repLead" className="form-label">
                  Replacement Team Lead (Optional):
                </label>
                <select
                  id="repLead"
                  className="input"
                  value={replacementLeadId}
                  onChange={(e) => setReplacementLeadId(e.target.value)}
                >
                  <option value="">Leave teams without a lead</option>
                  {members
                    .filter((m) => m.user_id !== demotingMember.user_id && !m.is_suspended && (m.role === "owner" || m.role === "team_lead"))
                    .map((m) => (
                      <option key={m.user_id} value={m.user_id}>
                        {m.display_name} ({m.role})
                      </option>
                    ))}
                </select>
              </div>
            )}
            <div className="modal-actions">
              <button type="button" className="btn btn--subtle" onClick={() => setDemotingMember(null)}>
                Cancel
              </button>
              <button type="button" className="btn btn--warn" onClick={handleDemote}>
                Confirm Demotion
              </button>
            </div>
          </div>
        </Modal>
      )}

      {/* Suspend Confirmation Modal */}
      {suspendingMember && (
        <Modal title={`Suspend ${suspendingMember.display_name}`} onClose={() => setSuspendingMember(null)}>
          <div className="admin-form">
            <p>
              Are you sure you want to suspend <strong>{suspendingMember.display_name}</strong> in this business?
            </p>
            <p className="form-info-banner">
              Desktop sync and Web Admin access for this business will be immediately blocked. Their credentials
              and tracking history remain preserved, and they can be reactivated at any time.
            </p>
            {suspendingMember.is_team_lead && (
              <div className="form-group">
                <label htmlFor="susRepLead" className="form-label">
                  Designate Interim Team Lead (Optional):
                </label>
                <select
                  id="susRepLead"
                  className="input"
                  value={replacementLeadId}
                  onChange={(e) => setReplacementLeadId(e.target.value)}
                >
                  <option value="">Unassign lead on affected teams</option>
                  {members
                    .filter((m) => m.user_id !== suspendingMember.user_id && !m.is_suspended && (m.role === "owner" || m.role === "team_lead"))
                    .map((m) => (
                      <option key={m.user_id} value={m.user_id}>
                        {m.display_name} ({m.role})
                      </option>
                    ))}
                </select>
              </div>
            )}
            <div className="modal-actions">
              <button type="button" className="btn btn--subtle" onClick={() => setSuspendingMember(null)}>
                Cancel
              </button>
              <button type="button" className="btn btn--danger" onClick={handleSuspend}>
                Suspend Member
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}

function NewBusinessModal({
  terms,
  kind,
  onClose,
  onCreated,
}: {
  terms: MemberTerms;
  kind: BusinessKind | undefined;
  onClose: () => void;
  onCreated: (id: string) => void;
}) {
  const { t } = useTranslation("dashboard");
  const isFamily = kind === "family";
  const orgCap = terms.org.charAt(0).toUpperCase() + terms.org.slice(1);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const b = await createBusiness(name);
      onCreated(b.id);
    } catch (err: any) {
      setError(err?.message || t("newBusinessModal.errorCreate"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal wide onClose={onClose}>
      <div className="bibo-dlg" role="dialog" aria-modal="true" style={{ ["--_w" as string]: "420px" }}>
        <div className="bibo-dlg__head">
          <div className="bibo-dlg__title">{t("newBusinessModal.title", { org: orgCap })}</div>
          <button
            type="button"
            className="bibo-dlg__close"
            aria-label={t("newBusinessModal.cancel")}
            onClick={onClose}
          >
            {IconClose}
          </button>
        </div>
        <form onSubmit={submit}>
          <div className="bibo-dlg__body">
            {error && (
              <div style={{ marginBottom: 12 }}>
                <Notice kind="danger">{error}</Notice>
              </div>
            )}
            <label className="bibo-field">
              <span className="bibo-field__lbl">
                {isFamily ? t("newBusinessModal.familyName") : t("newBusinessModal.companyName")}
              </span>
              <span className="bibo-input">
                <input
                  placeholder={isFamily ? t("newBusinessModal.placeholderFamily") : t("newBusinessModal.placeholderCompany")}
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  required
                  autoFocus
                />
              </span>
            </label>
          </div>
          <div className="bibo-dlg__foot">
            <button type="button" className="bibo-btn bibo-btn--ghost" onClick={onClose}>
              {t("newBusinessModal.cancel")}
            </button>
            <button className="bibo-btn bibo-btn--primary" disabled={busy || !name.trim()}>
              {busy ? t("newBusinessModal.creating") : t("newBusinessModal.create")}
            </button>
          </div>
        </form>
      </div>
    </Modal>
  );
}

function NewEmployeeModal({
  businessId,
  terms,
  onClose,
  onCreated,
}: {
  businessId: string | null;
  terms: MemberTerms;
  onClose: () => void;
  onCreated: (businessId: string, wasAutoCreated: boolean) => void;
}) {
  const { t } = useTranslation("dashboard");
  const [login, setLogin] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<{ login?: string; password?: string }>({});

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setFieldErrors({});
    const value = login.trim();
    const isEmail = value.includes("@");
    try {
      const res = await createEmployee({
        email: isEmail ? value : undefined,
        username: isEmail ? undefined : value.toLowerCase(),
        display_name: displayName.trim(),
        password,
        business_id: businessId ?? undefined,
      });
      onCreated(res.business.id, businessId === null);
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 409) {
          setFieldErrors({ login: t("newEmployeeModal.errorTaken") });
        } else if (/password/i.test(err.message)) {
          setFieldErrors({ password: err.message });
        } else if (/username/i.test(err.message)) {
          setFieldErrors({ login: err.message });
        } else {
          setError(err.message);
        }
      } else {
        setError(t("newEmployeeModal.errorCreate", { member: terms.lowerOne }));
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal wide onClose={onClose}>
      <div className="bibo-dlg" role="dialog" aria-modal="true" style={{ ["--_w" as string]: "460px" }}>
        <div className="bibo-dlg__head">
          <div className="bibo-dlg__icon">
            <span style={{ display: "inline-flex", lineHeight: 0 }}>{IconUserPlus}</span>
          </div>
          <div className="bibo-dlg__title">{terms.addCta}</div>
          <button
            type="button"
            className="bibo-dlg__close"
            aria-label={t("newEmployeeModal.cancel")}
            onClick={onClose}
          >
            {IconClose}
          </button>
        </div>
        <form onSubmit={submit}>
          <div className="bibo-dlg__body">
            {!businessId && (
              <div style={{ marginBottom: 12 }}>
                <Notice kind="info">
                  {t("newEmployeeModal.noBusinessSelected", { member: terms.lowerOne })}
                </Notice>
              </div>
            )}
            {error && (
              <div style={{ marginBottom: 12 }}>
                <Notice kind="danger">{error}</Notice>
              </div>
            )}
            <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
              <label className="bibo-field">
                <span className="bibo-field__lbl">{t("newEmployeeModal.displayName")}</span>
                <span className="bibo-input">
                  <input
                    placeholder="Mia"
                    value={displayName}
                    onChange={(e) => setDisplayName(e.target.value)}
                    required
                    autoFocus
                  />
                </span>
              </label>
              <label className="bibo-field">
                <span className="bibo-field__lbl">{t("newEmployeeModal.usernameOrEmail")}</span>
                <span className="bibo-input">
                  <input
                    type="text"
                    placeholder="mia_home"
                    value={login}
                    onChange={(e) => setLogin(e.target.value)}
                    autoComplete="off"
                    required
                  />
                </span>
                {fieldErrors.login && <div className="error-text">{fieldErrors.login}</div>}
              </label>
              <div style={{ display: "flex", gap: 8, alignItems: "flex-end" }}>
                <div style={{ flex: "1 1 0%" }}>
                  <label className="bibo-field">
                    <span className="bibo-field__lbl">{t("newEmployeeModal.temporaryPassword")}</span>
                    <span className="bibo-input bibo-input--dots">
                      <input
                        type="text"
                        placeholder="••••••••"
                        value={password}
                        onChange={(e) => setPassword(e.target.value)}
                        required
                      />
                    </span>
                    {fieldErrors.password && <div className="error-text">{fieldErrors.password}</div>}
                  </label>
                </div>
                <button
                  type="button"
                  className="bibo-btn bibo-btn--secondary"
                  title={t("newEmployeeModal.generate")}
                  onClick={() => setPassword(genTempPassword())}
                >
                  <span style={{ display: "inline-flex", lineHeight: 0 }}>{IconDices}</span>
                  <span>{t("newEmployeeModal.generate")}</span>
                </button>
              </div>
            </div>
          </div>
          <div className="bibo-dlg__foot">
            <button type="button" className="bibo-btn bibo-btn--ghost" onClick={onClose}>
              {t("newEmployeeModal.cancel")}
            </button>
            <button
              className="bibo-btn bibo-btn--primary"
              disabled={busy || !displayName.trim() || !login.trim() || !password}
            >
              {busy ? t("newEmployeeModal.adding") : terms.addCta}
            </button>
          </div>
        </form>
      </div>
    </Modal>
  );
}
