import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import {
  getEmployeePolicy,
  listEmployeeScreenshotRequests,
  resetEmployeePolicy,
  retryScreenshotRequest,
  updateEmployeePolicy,
} from "../api/endpoints";
import {
  ApiError,
  type EmployeePolicyDetail,
  type EmployeePolicyPatch,
  type ScreenshotRequest,
} from "../api/types";
import { Modal, Notice, Spinner } from "./ui";
import { fmtRelative } from "../format";

const svg = (children: ReactNode) => (
  <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
    {children}
  </svg>
);
const IconRotate = svg(<><path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8" /><path d="M21 3v5h-5" /></>);

const INTERVAL_PRESETS = [
  { label: "1 min", value: 60 },
  { label: "5 min", value: 300 },
  { label: "10 min", value: 600 },
  { label: "15 min", value: 900 },
];

export function EmployeePolicyModal({
  businessId,
  employeeId,
  employeeName,
  onClose,
  onSaved,
}: {
  businessId: string;
  employeeId: string;
  employeeName: string;
  onClose: () => void;
  onSaved?: () => void;
}) {
  const { t } = useTranslation("dashboard");
  const [detail, setDetail] = useState<EmployeePolicyDetail | null>(null);
  const [requests, setRequests] = useState<ScreenshotRequest[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [msg, setMsg] = useState<{ kind: "success" | "danger"; text: string } | null>(null);

  // Editable override state
  const [captureEnabled, setCaptureEnabled] = useState<boolean | null>(null);
  const [continuousCapture, setContinuousCapture] = useState<boolean | null>(null);
  const [intervalS, setIntervalS] = useState<number | null>(null);
  const [manualCapture, setManualCapture] = useState<boolean | null>(null);

  async function loadData() {
    setLoading(true);
    setError(null);
    try {
      const [p, r] = await Promise.all([
        getEmployeePolicy(businessId, employeeId),
        listEmployeeScreenshotRequests(businessId, employeeId, 10),
      ]);
      setDetail(p);
      setRequests(r.requests);
      setCaptureEnabled(p.overrides.screenshots_enabled ?? null);
      setContinuousCapture(p.overrides.continuous_capture_enabled ?? null);
      setIntervalS(p.overrides.screenshot_interval_s ?? null);
      setManualCapture(p.overrides.manual_capture_enabled ?? null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load employee policy");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    loadData();
  }, [businessId, employeeId]);

  async function handleSave() {
    setSaving(true);
    setMsg(null);
    const patch: EmployeePolicyPatch = {
      screenshots_enabled: captureEnabled,
      continuous_capture_enabled: continuousCapture,
      screenshot_interval_s: intervalS,
      manual_capture_enabled: manualCapture,
    };
    try {
      const updated = await updateEmployeePolicy(businessId, employeeId, patch);
      setDetail(updated);
      setMsg({ kind: "success", text: t("detail.policyModal.saveSuccess") });
      onSaved?.();
    } catch (err) {
      setMsg({ kind: "danger", text: err instanceof ApiError ? err.message : "Could not save policy" });
    } finally {
      setSaving(false);
    }
  }

  async function handleReset() {
    setSaving(true);
    setMsg(null);
    try {
      const res = await resetEmployeePolicy(businessId, employeeId);
      setDetail(res.policy);
      setCaptureEnabled(null);
      setContinuousCapture(null);
      setIntervalS(null);
      setManualCapture(null);
      setMsg({ kind: "success", text: t("detail.policyModal.resetSuccess") });
      onSaved?.();
    } catch (err) {
      setMsg({ kind: "danger", text: err instanceof ApiError ? err.message : "Could not reset policy" });
    } finally {
      setSaving(false);
    }
  }

  async function handleRetry(requestId: string) {
    try {
      await retryScreenshotRequest(businessId, requestId);
      setMsg({ kind: "success", text: t("detail.policyModal.retrySuccess") });
      const r = await listEmployeeScreenshotRequests(businessId, employeeId, 10);
      setRequests(r.requests);
    } catch (err) {
      setMsg({ kind: "danger", text: err instanceof ApiError ? err.message : "Retry failed" });
    }
  }

  const globalCaptureOff = detail?.business_defaults.screenshots_enabled === false;
  const globalManualOff = detail?.business_defaults.manual_capture_enabled === false;

  return (
    <Modal wide onClose={onClose}>
      <div className="bibo-dlg" style={{ ["--_w" as string]: "580px", maxWidth: "95vw" }}>
        <div className="bibo-dlg__head" style={{ marginBottom: 12 }}>
          <div className="bibo-dlg__title">
            {t("detail.policyModal.title", { name: employeeName })}
          </div>
          <button type="button" className="bibo-dlg__close" onClick={onClose} aria-label="Close">
            ✕
          </button>
        </div>

        <div className="bibo-dlg__body" style={{ maxHeight: "75vh", overflowY: "auto", paddingRight: 4 }}>
          {loading && <Spinner label="Loading policy…" />}

          {error && (
            <div style={{ marginBottom: 12 }}>
              <Notice kind="danger">{error}</Notice>
            </div>
          )}

          {msg && (
            <div style={{ marginBottom: 12 }}>
              <Notice kind={msg.kind}>{msg.text}</Notice>
            </div>
          )}

          {detail && !loading && (
            <>
              {globalCaptureOff && (
                <div style={{ marginBottom: 16 }}>
                  <Notice kind="danger">
                    {t("detail.policyModal.globalOffNotice")}
                  </Notice>
                </div>
              )}

              <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
                {/* 1. Screenshot Capture */}
                <div className="set-row" style={{ padding: "8px 0" }}>
                  <div>
                    <div className="set-title">{t("detail.policyModal.captureEnabled")}</div>
                    <div className="set-desc" style={{ fontSize: 12 }}>
                      Effective: <strong>{detail.effective.capture_enabled ? "ON" : "OFF"}</strong>
                      {captureEnabled === null && " (Inherited)"}
                    </div>
                  </div>
                  <div className="segmented" role="group">
                    <button
                      className={captureEnabled === null ? "active" : ""}
                      disabled={saving || globalCaptureOff}
                      onClick={() => setCaptureEnabled(null)}
                    >
                      {t("detail.policyModal.inherited", {
                        val: detail.business_defaults.screenshots_enabled ? "ON" : "OFF",
                      })}
                    </button>
                    <button
                      className={captureEnabled === true ? "active" : ""}
                      disabled={saving || globalCaptureOff}
                      onClick={() => setCaptureEnabled(true)}
                    >
                      {t("detail.policyModal.overrideOn")}
                    </button>
                    <button
                      className={captureEnabled === false ? "active" : ""}
                      disabled={saving || globalCaptureOff}
                      onClick={() => setCaptureEnabled(false)}
                    >
                      {t("detail.policyModal.overrideOff")}
                    </button>
                  </div>
                </div>

                {/* 2. Continuous Capture */}
                <div className="set-row" style={{ padding: "8px 0" }}>
                  <div>
                    <div className="set-title">{t("detail.policyModal.continuousCapture")}</div>
                    <div className="set-desc" style={{ fontSize: 12 }}>
                      Effective: <strong>{detail.effective.continuous_capture_enabled ? "ON" : "OFF"}</strong>
                      {continuousCapture === null && " (Inherited)"}
                    </div>
                  </div>
                  <div className="segmented" role="group">
                    <button
                      className={continuousCapture === null ? "active" : ""}
                      disabled={saving || globalCaptureOff}
                      onClick={() => setContinuousCapture(null)}
                    >
                      {t("detail.policyModal.inherited", {
                        val: detail.business_defaults.continuous_capture_enabled ? "ON" : "OFF",
                      })}
                    </button>
                    <button
                      className={continuousCapture === true ? "active" : ""}
                      disabled={saving || globalCaptureOff}
                      onClick={() => setContinuousCapture(true)}
                    >
                      {t("detail.policyModal.overrideOn")}
                    </button>
                    <button
                      className={continuousCapture === false ? "active" : ""}
                      disabled={saving || globalCaptureOff}
                      onClick={() => setContinuousCapture(false)}
                    >
                      {t("detail.policyModal.overrideOff")}
                    </button>
                  </div>
                </div>

                {/* 3. Screenshot Interval */}
                <div className="set-row" style={{ padding: "8px 0" }}>
                  <div>
                    <div className="set-title">{t("detail.policyModal.interval")}</div>
                    <div className="set-desc" style={{ fontSize: 12 }}>
                      Effective: <strong>{detail.effective.capture_interval_minutes} min</strong>
                      {intervalS === null && " (Inherited)"}
                    </div>
                  </div>
                  <div className="segmented" role="group">
                    <button
                      className={intervalS === null ? "active" : ""}
                      disabled={saving || globalCaptureOff}
                      onClick={() => setIntervalS(null)}
                    >
                      {t("detail.policyModal.inherited", {
                        val: `${detail.business_defaults.screenshot_interval_s / 60}m`,
                      })}
                    </button>
                    {INTERVAL_PRESETS.map((p) => (
                      <button
                        key={p.value}
                        className={intervalS === p.value ? "active" : ""}
                        disabled={saving || globalCaptureOff}
                        onClick={() => setIntervalS(p.value)}
                      >
                        {p.label}
                      </button>
                    ))}
                  </div>
                </div>

                {/* 4. Manual Capture Requests */}
                <div className="set-row" style={{ padding: "8px 0" }}>
                  <div>
                    <div className="set-title">{t("detail.policyModal.manualRequests")}</div>
                    <div className="set-desc" style={{ fontSize: 12 }}>
                      Effective: <strong>{detail.effective.manual_capture_enabled ? "Allowed" : "Blocked"}</strong>
                      {manualCapture === null && " (Inherited)"}
                    </div>
                  </div>
                  <div className="segmented" role="group">
                    <button
                      className={manualCapture === null ? "active" : ""}
                      disabled={saving || globalCaptureOff || globalManualOff}
                      onClick={() => setManualCapture(null)}
                    >
                      {t("detail.policyModal.inherited", {
                        val: detail.business_defaults.manual_capture_enabled ? "Allowed" : "Blocked",
                      })}
                    </button>
                    <button
                      className={manualCapture === true ? "active" : ""}
                      disabled={saving || globalCaptureOff || globalManualOff}
                      onClick={() => setManualCapture(true)}
                    >
                      {t("detail.policyModal.overrideOn")}
                    </button>
                    <button
                      className={manualCapture === false ? "active" : ""}
                      disabled={saving || globalCaptureOff}
                      onClick={() => setManualCapture(false)}
                    >
                      {t("detail.policyModal.overrideOff")}
                    </button>
                  </div>
                </div>
              </div>

              {/* Reset to inherited defaults button */}
              <div style={{ marginTop: 16, paddingTop: 12, borderTop: "1px solid var(--border)", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <button
                  type="button"
                  className="bibo-btn bibo-btn--ghost bibo-btn--sm"
                  disabled={saving}
                  onClick={handleReset}
                  style={{ color: "var(--text-muted)" }}
                >
                  {t("detail.policyModal.resetDefaults")}
                </button>
                <div style={{ display: "flex", gap: 8 }}>
                  <button type="button" className="bibo-btn bibo-btn--secondary" onClick={onClose}>
                    {t("newBusinessModal.cancel")}
                  </button>
                  <button type="button" className="bibo-btn bibo-btn--primary" disabled={saving} onClick={handleSave}>
                    {saving ? "Saving…" : "Save Policy"}
                  </button>
                </div>
              </div>

              {/* Recent manual requests section */}
              <div style={{ marginTop: 24, paddingTop: 16, borderTop: "1px solid var(--border)" }}>
                <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 8 }}>
                  {t("detail.policyModal.recentRequests")}
                </div>

                {requests.length === 0 ? (
                  <div className="muted" style={{ fontSize: 13 }}>
                    {t("detail.policyModal.noRequests")}
                  </div>
                ) : (
                  <div style={{ maxHeight: 200, overflowY: "auto", display: "flex", flexDirection: "column", gap: 6 }}>
                    {requests.map((r) => {
                      const isAck = r.status === "acknowledged";
                      const isPending = r.status === "pending";
                      const isFailed = r.status.startsWith("rejected_") || r.status === "failed";
                      const isCompleted = r.status === "completed";
                      return (
                        <div
                          key={r.id}
                          style={{
                            display: "flex",
                            alignItems: "center",
                            justifyContent: "space-between",
                            fontSize: 12,
                            padding: "6px 10px",
                            background: "var(--bg-subtle, rgba(0,0,0,0.03))",
                            borderRadius: 6,
                          }}
                        >
                          <div>
                            <span style={{ fontWeight: 600 }}>{fmtRelative(Math.floor(new Date(r.created_at).getTime() / 1000))}</span>
                            {" · "}
                            <span className="muted">by {r.requested_by_name}</span>
                          </div>
                          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                            <span
                              className="pill"
                              style={{
                                fontSize: 11,
                                padding: "2px 8px",
                                background: isCompleted
                                  ? "var(--positive-soft, #e6f9ed)"
                                  : isAck
                                  ? "rgba(99, 102, 241, 0.14)"
                                  : isPending
                                  ? "var(--info-soft, #eef4ff)"
                                  : "var(--negative-soft, #fdf0f0)",
                                color: isCompleted
                                  ? "var(--positive, #10b981)"
                                  : isAck
                                  ? "#6366f1"
                                  : isPending
                                  ? "var(--info, #3b82f6)"
                                  : "var(--negative, #ef4444)",
                                fontWeight: 600,
                              }}
                            >
                              {t(`detail.policyModal.status.${r.status}`, { defaultValue: r.status })}
                            </span>
                            {isFailed && (
                              <button
                                type="button"
                                className="bibo-btn bibo-btn--ghost bibo-btn--sm"
                                title={t("detail.policyModal.retry")}
                                onClick={() => handleRetry(r.id)}
                                style={{ padding: "2px 6px", display: "inline-flex", alignItems: "center" }}
                              >
                                {IconRotate}
                              </button>
                            )}
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      </div>
    </Modal>
  );
}
