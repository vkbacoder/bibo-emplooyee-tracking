import { useEffect, useState, type ReactNode } from "react";
import { listOrgAuditLogs } from "../api/endpoints";
import type { OrgAuditLog } from "../api/types";
import { Empty, Notice, Spinner } from "../components/ui";
import { useBusinesses } from "../useBusinesses";
import { useMyAccess } from "../useMyAccess";

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

const IconShield = svg(<><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10" /></>);
const IconChevronLeft = svg(<path d="m15 18-6-6 6-6" />);
const IconChevronRight = svg(<path d="m9 18 6-6-6-6" />);

function getActionBadge(action: string) {
  let badgeClass = "badge--subtle";
  if (action.includes("created") || action.includes("promoted") || action.includes("granted") || action.includes("reactivated")) {
    badgeClass = "badge--positive";
  } else if (action.includes("demoted") || action.includes("suspended") || action.includes("revoked")) {
    badgeClass = "badge--warn";
  } else if (action.includes("deleted") || action.includes("removed")) {
    badgeClass = "badge--danger";
  }

  const label = action.replace(".", " • ").toUpperCase();
  return <span className={`badge ${badgeClass}`}>{label}</span>;
}

export function AuditLogs() {
  const { selectedId } = useBusinesses();
  const { isOwner, can } = useMyAccess();

  const [logs, setLogs] = useState<OrgAuditLog[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(0);
  const [search, setSearch] = useState("");
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const PAGE_SIZE = 50;
  const canViewLogs = isOwner || can("can_view_audit_logs");

  const loadLogs = async (bizId: string, pageNum: number) => {
    setLoading(true);
    setError(null);
    try {
      const res = await listOrgAuditLogs(bizId, PAGE_SIZE, pageNum * PAGE_SIZE);
      setLogs(res.logs || []);
    } catch (err: any) {
      setError(err?.message || "Failed to load audit logs");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (selectedId && canViewLogs) {
      loadLogs(selectedId, page);
    } else {
      setLogs([]);
    }
  }, [selectedId, page, canViewLogs]);

  const filteredLogs = logs.filter((log) => {
    if (!search) return true;
    const q = search.toLowerCase();
    return (
      log.action.toLowerCase().includes(q) ||
      (log.actor_display_name && log.actor_display_name.toLowerCase().includes(q)) ||
      (log.actor_email && log.actor_email.toLowerCase().includes(q)) ||
      (log.target_display_name && log.target_display_name.toLowerCase().includes(q)) ||
      (log.target_email && log.target_email.toLowerCase().includes(q)) ||
      (log.target_team_name && log.target_team_name.toLowerCase().includes(q))
    );
  });

  if (!canViewLogs) {
    return (
      <div className="page-audit-logs">
        <Notice kind="info">
          You do not have permission to view organizational audit logs.
        </Notice>
      </div>
    );
  }

  return (
    <div className="page-audit-logs">
      {/* Page Header */}
      <div className="admin-page-header">
        <div className="admin-page-header__main">
          <div className="title-with-badge">
            <h1 className="admin-page-title">Organizational Audit Logs</h1>
            <span className="badge badge--primary">
              <span className="badge-icon">{IconShield}</span>
              <span>Append-Only Immutability</span>
            </span>
          </div>
          <p className="admin-page-subtitle">
            Cryptographically and trigger-enforced immutable record of team restructurings, role changes, and delegations.
          </p>
        </div>
      </div>

      {error && <Notice kind="danger">{error}</Notice>}

      {/* Filter Toolbar */}
      <div className="audit-toolbar bibo-card bibo-card--default admin-cardpad">
        <div className="audit-toolbar__search">
          <input
            type="text"
            className="input input--sm"
            placeholder="Filter by actor, target, team or action..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <div className="audit-toolbar__pagination">
          <button
            type="button"
            className="btn btn--subtle btn--sm"
            onClick={() => setPage((p) => Math.max(0, p - 1))}
            disabled={page === 0 || loading}
          >
            {IconChevronLeft}
            <span>Previous</span>
          </button>
          <span className="audit-page-indicator">Page {page + 1}</span>
          <button
            type="button"
            className="btn btn--subtle btn--sm"
            onClick={() => setPage((p) => p + 1)}
            disabled={logs.length < PAGE_SIZE || loading}
          >
            <span>Next</span>
            {IconChevronRight}
          </button>
        </div>
      </div>

      {/* Logs Table */}
      {loading ? (
        <div className="admin-loading-box">
          <Spinner />
          <p>Loading audit log trail...</p>
        </div>
      ) : filteredLogs.length === 0 ? (
        <Empty>
          <h3>No audit entries found</h3>
          <p className="muted">Organizational lifecycle changes will appear here as teams and permissions are managed.</p>
        </Empty>
      ) : (
        <div className="bibo-card bibo-card--default audit-table-wrapper">
          <table className="admin-table">
            <thead>
              <tr>
                <th>Timestamp</th>
                <th>Action</th>
                <th>Actor</th>
                <th>Target</th>
                <th>Details</th>
              </tr>
            </thead>
            <tbody>
              {filteredLogs.map((log) => {
                const isExpanded = expandedId === log.id;
                const formattedDate = new Date(log.created_at).toLocaleString();

                return (
                  <tr key={log.id} className="audit-log-row">
                    <td className="audit-log-time">
                      <span>{formattedDate}</span>
                    </td>
                    <td>{getActionBadge(log.action)}</td>
                    <td>
                      <div className="audit-person">
                        <span className="audit-person__name">
                          {log.actor_display_name || "System"}
                        </span>
                        {log.actor_email && (
                          <span className="audit-person__sub">{log.actor_email}</span>
                        )}
                      </div>
                    </td>
                    <td>
                      <div className="audit-person">
                        {log.target_team_name ? (
                          <span className="badge badge--subtle">{log.target_team_name}</span>
                        ) : log.target_display_name ? (
                          <>
                            <span className="audit-person__name">{log.target_display_name}</span>
                            {log.target_email && (
                              <span className="audit-person__sub">{log.target_email}</span>
                            )}
                          </>
                        ) : (
                          <span className="text-muted">—</span>
                        )}
                      </div>
                    </td>
                    <td>
                      <button
                        type="button"
                        className="btn btn--subtle btn--xs"
                        onClick={() => setExpandedId(isExpanded ? null : log.id)}
                      >
                        {isExpanded ? "Hide" : "Inspect"}
                      </button>
                      {isExpanded && (
                        <div className="audit-details-popup">
                          <pre>{JSON.stringify(log.details, null, 2)}</pre>
                        </div>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
