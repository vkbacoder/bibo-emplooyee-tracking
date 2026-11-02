import { useCallback, useEffect, useState, type ReactNode } from "react";
import {
  createScreenshotRequest,
  getEmployeePolicy,
  listEmployeeScreenshotRequests,
  reportScreenshots,
  resetEmployeePolicy,
  updateEmployeePolicy,
} from "../api/endpoints";
import {
  ApiError,
  type EmployeePolicyDetail,
  type EmployeePolicyPatch,
  type ScreenshotMeta,
  type ScreenshotRequest,
} from "../api/types";
import { ScreenshotGallery } from "./reports/ScreenshotGallery";
import { Notice, Spinner } from "./ui";
import { fmtRelative } from "../format";

const INTERVAL_PRESETS = [
  { label: "1 min", value: 60 },
  { label: "5 min", value: 300 },
  { label: "10 min", value: 600 },
  { label: "15 min", value: 900 },
  { label: "30 min", value: 1800 },
];

const svg = (children: ReactNode, size = 16) => (
  <svg
    viewBox="0 0 24 24"
    width={size}
    height={size}
    fill="none"
    stroke="currentColor"
    strokeWidth={2}
    strokeLinecap="round"
    strokeLinejoin="round"
  >
    {children}
  </svg>
);

const IconCamera = svg(
  <>
    <path d="M13.997 4a2 2 0 0 1 1.76 1.05l.486.9A2 2 0 0 0 18.003 7H20a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2h1.997a2 2 0 0 0 1.759-1.048l.489-.904A2 2 0 0 1 10.004 4z" />
    <circle cx="12" cy="13" r="3" />
  </>,
  18
);

type ScreenshotSubTab = "automatic" | "manual";

export function ScreenshotManagement({
  businessId,
  employeeId,
  employeeName,
  from,
  to,
}: {
  businessId: string;
  employeeId: string;
  employeeName: string;
  from: number;
  to: number;
}) {
  const [subTab, setSubTab] = useState<ScreenshotSubTab>("automatic");

  // Policy & overrides state
  const [policy, setPolicy] = useState<EmployeePolicyDetail | null>(null);
  const [loadingPolicy, setLoadingPolicy] = useState(true);
  const [savingPolicy, setSavingPolicy] = useState(false);
  const [policyMsg, setPolicyMsg] = useState<{ kind: "success" | "danger"; text: string } | null>(null);

  // Editable settings (Automatic)
  const [autoOverride, setAutoOverride] = useState(false);
  const [continuousCapture, setContinuousCapture] = useState<boolean>(true);
  const [masterOverride, setMasterOverride] = useState(false);
  const [masterEnabled, setMasterEnabled] = useState<boolean>(true);
  const [intervalOverride, setIntervalOverride] = useState(false);
  const [intervalMinutes, setIntervalMinutes] = useState<number>(5);

  // Editable settings (Manual)
  const [manualOverride, setManualOverride] = useState(false);
  const [manualEnabled, setManualEnabled] = useState<boolean>(true);

  // Capture Now & request status
  const [capturing, setCapturing] = useState(false);
  const [captureMsg, setCaptureMsg] = useState<{ kind: "success" | "danger" | "info"; text: string } | null>(null);
  const [requests, setRequests] = useState<ScreenshotRequest[]>([]);
  const [activeRequest, setActiveRequest] = useState<ScreenshotRequest | null>(null);

  // Galleries state
  const [autoShots, setAutoShots] = useState<ScreenshotMeta[]>([]);
  const [manualShots, setManualShots] = useState<ScreenshotMeta[]>([]);
  const [loadingAuto, setLoadingAuto] = useState(false);
  const [loadingManual, setLoadingManual] = useState(false);
  const [errorAuto, setErrorAuto] = useState<string | null>(null);
  const [errorManual, setErrorManual] = useState<string | null>(null);

  const [autoOffset, setAutoOffset] = useState(0);
  const [manualOffset, setManualOffset] = useState(0);
  const [hasMoreAuto, setHasMoreAuto] = useState(false);
  const [hasMoreManual, setHasMoreManual] = useState(false);

  // Load employee policy
  const loadPolicy = useCallback(async () => {
    if (!businessId || !employeeId) return;
    try {
      const p = await getEmployeePolicy(businessId, employeeId);
      setPolicy(p);

      // Populate automatic controls
      setMasterOverride(p.overrides.screenshots_enabled != null);
      setMasterEnabled(p.overrides.screenshots_enabled ?? p.business_defaults.screenshots_enabled);

      setAutoOverride(p.overrides.continuous_capture_enabled != null);
      setContinuousCapture(p.overrides.continuous_capture_enabled ?? p.business_defaults.continuous_capture_enabled);

      setIntervalOverride(p.overrides.screenshot_interval_s != null);
      setIntervalMinutes(
        Math.round((p.overrides.screenshot_interval_s ?? p.business_defaults.screenshot_interval_s) / 60) || 5
      );

      // Populate manual controls
      setManualOverride(p.overrides.manual_capture_enabled != null);
      setManualEnabled(p.overrides.manual_capture_enabled ?? p.business_defaults.manual_capture_enabled);
    } catch (err) {
      setPolicyMsg({
        kind: "danger",
        text: err instanceof ApiError ? err.message : "Failed to load screenshot policy",
      });
    } finally {
      setLoadingPolicy(false);
    }
  }, [businessId, employeeId]);

  // Load requests
  const loadRequests = useCallback(async () => {
    if (!businessId || !employeeId) return;
    try {
      const resp = await listEmployeeScreenshotRequests(businessId, employeeId, 10);
      setRequests(resp.requests);
      const inFlight = resp.requests.find(
        (r) => r.status === "pending" || r.status === "acknowledged"
      );
      if (inFlight) {
        setActiveRequest(inFlight);
      }
    } catch {
      // quiet
    }
  }, [businessId, employeeId]);

  // Load automatic screenshots
  const loadAutoScreenshots = useCallback(
    async (offset = 0, append = false) => {
      if (!employeeId) return;
      setLoadingAuto(true);
      setErrorAuto(null);
      try {
        const resp = await reportScreenshots(employeeId, from, to, 60, offset, "automatic");
        const list = resp.screenshots || [];
        setAutoShots((prev) => (append ? [...prev, ...list] : list));
        setHasMoreAuto(list.length === 60);
        setAutoOffset(offset);
      } catch (err) {
        setErrorAuto(err instanceof ApiError ? err.message : "Failed to load automatic screenshots");
      } finally {
        setLoadingAuto(false);
      }
    },
    [employeeId, from, to]
  );

  // Load manual screenshots
  const loadManualScreenshots = useCallback(
    async (offset = 0, append = false) => {
      if (!employeeId) return;
      setLoadingManual(true);
      setErrorManual(null);
      try {
        const resp = await reportScreenshots(employeeId, from, to, 60, offset, "manual");
        const list = resp.screenshots || [];
        setManualShots((prev) => (append ? [...prev, ...list] : list));
        setHasMoreManual(list.length === 60);
        setManualOffset(offset);
      } catch (err) {
        setErrorManual(err instanceof ApiError ? err.message : "Failed to load manual screenshots");
      } finally {
        setLoadingManual(false);
      }
    },
    [employeeId, from, to]
  );

  useEffect(() => {
    loadPolicy();
    loadRequests();
    loadAutoScreenshots(0, false);
    loadManualScreenshots(0, false);
  }, [loadPolicy, loadRequests, loadAutoScreenshots, loadManualScreenshots]);

  // Polling active request until terminal (completed, failed, expired, rejected_*)
  useEffect(() => {
    if (!activeRequest || activeRequest.status === "completed" || activeRequest.status.startsWith("rejected") || activeRequest.status === "failed") {
      return;
    }

    const timer = setInterval(async () => {
      try {
        const resp = await listEmployeeScreenshotRequests(businessId, employeeId, 5);
        setRequests(resp.requests);
        const latest = resp.requests.find((r) => r.id === activeRequest.id);
        if (latest) {
          setActiveRequest(latest);
          if (latest.status === "completed") {
            setCaptureMsg({
              kind: "success",
              text: "Capture completed! Screenshot image verified and added to Manual gallery.",
            });
            loadManualScreenshots(0, false);
            clearInterval(timer);
          } else if (latest.status.startsWith("rejected") || latest.status === "failed") {
            setCaptureMsg({
              kind: "danger",
              text: `Capture ${latest.status.replace("_", " ")}: ${latest.error_message || "Desktop rejected request"}`,
            });
            clearInterval(timer);
          }
        }
      } catch {
        // ignore polling err
      }
    }, 2000);

    return () => clearInterval(timer);
  }, [activeRequest, businessId, employeeId, loadManualScreenshots]);

  // Save Automatic Settings
  async function saveAutomaticSettings() {
    if (!businessId || !employeeId) return;
    setSavingPolicy(true);
    setPolicyMsg(null);

    const validatedInterval = Math.max(1, Math.min(120, intervalMinutes));
    const patch: EmployeePolicyPatch = {
      screenshots_enabled: masterOverride ? masterEnabled : null,
      continuous_capture_enabled: autoOverride ? continuousCapture : null,
      screenshot_interval_s: intervalOverride ? validatedInterval * 60 : null,
    };

    try {
      const updated = await updateEmployeePolicy(businessId, employeeId, patch);
      setPolicy(updated);
      setPolicyMsg({
        kind: "success",
        text: "Automatic screenshot settings saved successfully.",
      });
      setTimeout(() => setPolicyMsg(null), 4000);
    } catch (err) {
      setPolicyMsg({
        kind: "danger",
        text: err instanceof ApiError ? err.message : "Failed to save automatic settings",
      });
    } finally {
      setSavingPolicy(false);
    }
  }

  // Save Manual Settings
  async function saveManualSettings() {
    if (!businessId || !employeeId) return;
    setSavingPolicy(true);
    setPolicyMsg(null);

    const patch: EmployeePolicyPatch = {
      manual_capture_enabled: manualOverride ? manualEnabled : null,
    };

    try {
      const updated = await updateEmployeePolicy(businessId, employeeId, patch);
      setPolicy(updated);
      setPolicyMsg({
        kind: "success",
        text: "Manual screenshot settings saved successfully.",
      });
      setTimeout(() => setPolicyMsg(null), 4000);
    } catch (err) {
      setPolicyMsg({
        kind: "danger",
        text: err instanceof ApiError ? err.message : "Failed to save manual settings",
      });
    } finally {
      setSavingPolicy(false);
    }
  }

  // Reset to defaults
  async function handleResetDefaults() {
    if (!businessId || !employeeId) return;
    setSavingPolicy(true);
    setPolicyMsg(null);
    try {
      const res = await resetEmployeePolicy(businessId, employeeId);
      setPolicy(res.policy);
      setMasterOverride(false);
      setAutoOverride(false);
      setIntervalOverride(false);
      setManualOverride(false);
      setMasterEnabled(res.policy.business_defaults.screenshots_enabled);
      setContinuousCapture(res.policy.business_defaults.continuous_capture_enabled);
      setIntervalMinutes(Math.round(res.policy.business_defaults.screenshot_interval_s / 60) || 5);
      setManualEnabled(res.policy.business_defaults.manual_capture_enabled);
      setPolicyMsg({
        kind: "success",
        text: "Reset all employee screenshot settings to business defaults.",
      });
      setTimeout(() => setPolicyMsg(null), 4000);
    } catch (err) {
      setPolicyMsg({
        kind: "danger",
        text: err instanceof ApiError ? err.message : "Failed to reset settings",
      });
    } finally {
      setSavingPolicy(false);
    }
  }

  // Trigger Capture Now
  async function triggerCaptureNow() {
    if (!businessId || !employeeId) return;
    setCapturing(true);
    setCaptureMsg({
      kind: "info",
      text: "Submitting capture request to desktop client...",
    });

    try {
      const req = await createScreenshotRequest(businessId, employeeId);
      setActiveRequest(req);
      setRequests((prev) => [req, ...prev]);
      setCaptureMsg({
        kind: "info",
        text: `Request #${req.id.slice(0, 8)} sent. Waiting for desktop to capture and upload...`,
      });
    } catch (err) {
      setCaptureMsg({
        kind: "danger",
        text: err instanceof ApiError ? err.message : "Capture request failed",
      });
    } finally {
      setCapturing(false);
    }
  }

  const effective = policy?.effective;
  const isMasterDisabled = effective ? !effective.capture_enabled : false;
  const isManualDisabled = effective ? !effective.manual_capture_enabled : false;
  const isCaptureNowAllowed = !isMasterDisabled && !isManualDisabled && !capturing;

  return (
    <div className="admin-screenshot-mgmt" style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      {/* Section Header & Sub-Tabs */}
      <div
        className="admin-mgmt-header"
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          flexWrap: "wrap",
          gap: 12,
          paddingBottom: 12,
          borderBottom: "1px solid var(--border-subtle)",
        }}
      >
        <div>
          <h2 style={{ fontSize: 18, fontWeight: 700, margin: 0, letterSpacing: "-0.02em" }}>
            SCREENSHOT MANAGEMENT
          </h2>
          <div style={{ fontSize: 12, color: "var(--text-muted)", marginTop: 2 }}>
            Manage scheduled automatic captures and on-demand manual captures independently.
          </div>
        </div>

        {/* Two distinct subtabs */}
        <div className="bibo-tabs bibo-tabs--pill" role="tablist">
          <button
            role="tab"
            aria-selected={subTab === "automatic"}
            className={`bibo-tab${subTab === "automatic" ? " bibo-tab--on" : ""}`}
            onClick={() => setSubTab("automatic")}
            style={{ fontWeight: 600, fontSize: 13 }}
          >
            Automatic Screenshots ({autoShots.length})
          </button>
          <button
            role="tab"
            aria-selected={subTab === "manual"}
            className={`bibo-tab${subTab === "manual" ? " bibo-tab--on" : ""}`}
            onClick={() => setSubTab("manual")}
            style={{ fontWeight: 600, fontSize: 13 }}
          >
            Manual Screenshots ({manualShots.length})
          </button>
        </div>
      </div>

      {policyMsg && (
        <Notice kind={policyMsg.kind}>{policyMsg.text}</Notice>
      )}

      {loadingPolicy && !policy ? (
        <Spinner label="Loading screenshot policy & configuration..." />
      ) : (
        <>
          {/* TAB 1: AUTOMATIC SCREENSHOTS */}
          {subTab === "automatic" && (
            <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
              {/* Settings Card */}
              <div
                className="bibo-card bibo-card--default admin-cardpad"
                style={{ padding: 20, borderRadius: 14 }}
              >
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 16 }}>
                  <div>
                    <h3 style={{ fontSize: 15, fontWeight: 700, margin: 0 }}>
                      Automatic Screenshot Settings
                    </h3>
                    <div style={{ fontSize: 12, color: "var(--text-muted)", marginTop: 2 }}>
                      Configure continuous scheduled captures for this employee.
                    </div>
                  </div>
                  <button
                    type="button"
                    className="bibo-btn bibo-btn--secondary bibo-btn--sm"
                    onClick={handleResetDefaults}
                    disabled={savingPolicy}
                  >
                    Reset to Business Defaults
                  </button>
                </div>

                <div
                  style={{
                    display: "grid",
                    gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))",
                    gap: 16,
                    marginBottom: 20,
                  }}
                >
                  {/* Master Screenshot switch */}
                  <div
                    style={{
                      padding: 14,
                      borderRadius: 10,
                      background: "var(--bg-inset)",
                      border: "1px solid var(--border-subtle)",
                    }}
                  >
                    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 8 }}>
                      <span style={{ fontWeight: 600, fontSize: 13 }}>Screenshot Collection</span>
                      <span
                        className="bibo-pill"
                        style={{
                          fontSize: 11,
                          padding: "2px 8px",
                          borderRadius: 999,
                          fontWeight: 700,
                          background: masterOverride ? "rgba(168, 85, 247, 0.15)" : "var(--bg-subtle)",
                          color: masterOverride ? "#c084fc" : "var(--text-muted)",
                        }}
                      >
                        {masterOverride ? "Overridden" : "Inherited"}
                      </span>
                    </div>
                    <div style={{ fontSize: 12, color: "var(--text-muted)", marginBottom: 12 }}>
                      Master permission allowing screenshot collection for this employee.
                    </div>
                    <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
                      <label style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 13, cursor: "pointer" }}>
                        <input
                          type="checkbox"
                          checked={masterOverride}
                          onChange={(e) => setMasterOverride(e.target.checked)}
                        />
                        <span>Override</span>
                      </label>
                      {masterOverride && (
                        <div className="bibo-seg bibo-seg--sm">
                          <button
                            type="button"
                            className={`bibo-seg__opt${masterEnabled ? " bibo-seg__opt--on" : ""}`}
                            onClick={() => setMasterEnabled(true)}
                          >
                            Enabled
                          </button>
                          <button
                            type="button"
                            className={`bibo-seg__opt${!masterEnabled ? " bibo-seg__opt--on" : ""}`}
                            onClick={() => setMasterEnabled(false)}
                          >
                            Disabled
                          </button>
                        </div>
                      )}
                    </div>
                  </div>

                  {/* Automatic Capture switch */}
                  <div
                    style={{
                      padding: 14,
                      borderRadius: 10,
                      background: "var(--bg-inset)",
                      border: "1px solid var(--border-subtle)",
                    }}
                  >
                    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 8 }}>
                      <span style={{ fontWeight: 600, fontSize: 13 }}>Automatic Capture</span>
                      <span
                        className="bibo-pill"
                        style={{
                          fontSize: 11,
                          padding: "2px 8px",
                          borderRadius: 999,
                          fontWeight: 700,
                          background: autoOverride ? "rgba(168, 85, 247, 0.15)" : "var(--bg-subtle)",
                          color: autoOverride ? "#c084fc" : "var(--text-muted)",
                        }}
                      >
                        {autoOverride ? "Overridden" : "Inherited"}
                      </span>
                    </div>
                    <div style={{ fontSize: 12, color: "var(--text-muted)", marginBottom: 12 }}>
                      Scheduled background capture while the employee is working.
                    </div>
                    <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
                      <label style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 13, cursor: "pointer" }}>
                        <input
                          type="checkbox"
                          checked={autoOverride}
                          onChange={(e) => setAutoOverride(e.target.checked)}
                        />
                        <span>Override</span>
                      </label>
                      {autoOverride && (
                        <div className="bibo-seg bibo-seg--sm">
                          <button
                            type="button"
                            className={`bibo-seg__opt${continuousCapture ? " bibo-seg__opt--on" : ""}`}
                            onClick={() => setContinuousCapture(true)}
                          >
                            Enabled
                          </button>
                          <button
                            type="button"
                            className={`bibo-seg__opt${!continuousCapture ? " bibo-seg__opt--on" : ""}`}
                            onClick={() => setContinuousCapture(false)}
                          >
                            Disabled
                          </button>
                        </div>
                      )}
                    </div>
                  </div>

                  {/* Capture Interval */}
                  <div
                    style={{
                      padding: 14,
                      borderRadius: 10,
                      background: "var(--bg-inset)",
                      border: "1px solid var(--border-subtle)",
                    }}
                  >
                    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 8 }}>
                      <span style={{ fontWeight: 600, fontSize: 13 }}>Screenshot Interval</span>
                      <span
                        className="bibo-pill"
                        style={{
                          fontSize: 11,
                          padding: "2px 8px",
                          borderRadius: 999,
                          fontWeight: 700,
                          background: intervalOverride ? "rgba(168, 85, 247, 0.15)" : "var(--bg-subtle)",
                          color: intervalOverride ? "#c084fc" : "var(--text-muted)",
                        }}
                      >
                        {intervalOverride ? "Overridden" : "Inherited"}
                      </span>
                    </div>
                    <div style={{ fontSize: 12, color: "var(--text-muted)", marginBottom: 12 }}>
                      Frequency in minutes (Currently effective: {effective?.capture_interval_minutes ?? 5} min).
                    </div>
                    <div style={{ display: "flex", gap: 10, alignItems: "center", flexWrap: "wrap" }}>
                      <label style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 13, cursor: "pointer" }}>
                        <input
                          type="checkbox"
                          checked={intervalOverride}
                          onChange={(e) => setIntervalOverride(e.target.checked)}
                        />
                        <span>Override</span>
                      </label>
                      {intervalOverride && (
                        <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
                          {INTERVAL_PRESETS.map((p) => {
                            const mins = Math.round(p.value / 60);
                            return (
                              <button
                                key={p.value}
                                type="button"
                                className={`bibo-btn bibo-btn--sm ${intervalMinutes === mins ? "bibo-btn--primary" : "bibo-btn--secondary"}`}
                                onClick={() => setIntervalMinutes(mins)}
                              >
                                {p.label}
                              </button>
                            );
                          })}
                          <input
                            type="number"
                            min={1}
                            max={60}
                            value={intervalMinutes}
                            onChange={(e) => setIntervalMinutes(Math.max(1, parseInt(e.target.value, 10) || 1))}
                            style={{ width: 60, padding: "4px 8px", borderRadius: 6, border: "1px solid var(--border)" }}
                          />
                          <span style={{ fontSize: 12, color: "var(--text-muted)" }}>min</span>
                        </div>
                      )}
                    </div>
                  </div>
                </div>

                <div style={{ display: "flex", justifyContent: "flex-end" }}>
                  <button
                    type="button"
                    className="bibo-btn bibo-btn--primary"
                    onClick={saveAutomaticSettings}
                    disabled={savingPolicy}
                  >
                    {savingPolicy ? "Saving…" : "Save Automatic Settings"}
                  </button>
                </div>
              </div>

              {/* Automatic Gallery */}
              <div
                className="bibo-card bibo-card--default admin-cardpad"
                style={{ padding: 20, borderRadius: 14 }}
              >
                {errorAuto && <Notice kind="danger">{errorAuto}</Notice>}

                {loadingAuto && autoShots.length === 0 ? (
                  <Spinner label="Loading automatic screenshots…" />
                ) : (
                  <ScreenshotGallery
                    shots={autoShots}
                    title="Automatic Captured Screenshots"
                    subtitle="Only screenshots captured by scheduled/continuous capture process"
                    sourceType="automatic"
                    employeeName={employeeName}
                    onRefresh={() => loadAutoScreenshots(0, false)}
                    refreshing={loadingAuto}
                    onLoadMore={hasMoreAuto ? () => loadAutoScreenshots(autoOffset + 60, true) : undefined}
                    loadingMore={loadingAuto && autoShots.length > 0}
                    emptyMessage="No automatic screenshots recorded for this employee in the selected time window."
                  />
                )}
              </div>
            </div>
          )}

          {/* TAB 2: MANUAL SCREENSHOTS */}
          {subTab === "manual" && (
            <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
              {/* Settings Card */}
              <div
                className="bibo-card bibo-card--default admin-cardpad"
                style={{ padding: 20, borderRadius: 14 }}
              >
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 16 }}>
                  <div>
                    <h3 style={{ fontSize: 15, fontWeight: 700, margin: 0 }}>
                      Manual Screenshot Settings
                    </h3>
                    <div style={{ fontSize: 12, color: "var(--text-muted)", marginTop: 2 }}>
                      Configure permissions for on-demand screenshot requests.
                    </div>
                  </div>
                  <button
                    type="button"
                    className="bibo-btn bibo-btn--secondary bibo-btn--sm"
                    onClick={handleResetDefaults}
                    disabled={savingPolicy}
                  >
                    Reset to Business Defaults
                  </button>
                </div>

                {isMasterDisabled && (
                  <div style={{ marginBottom: 16 }}>
                    <Notice kind="info">
                      Master screenshot collection is disabled for this employee or business. Manual captures are currently blocked by master policy.
                    </Notice>
                  </div>
                )}

                <div
                  style={{
                    display: "grid",
                    gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))",
                    gap: 16,
                    marginBottom: 20,
                  }}
                >
                  {/* Manual Capture Permission switch */}
                  <div
                    style={{
                      padding: 14,
                      borderRadius: 10,
                      background: "var(--bg-inset)",
                      border: "1px solid var(--border-subtle)",
                    }}
                  >
                    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 8 }}>
                      <span style={{ fontWeight: 600, fontSize: 13 }}>Manual Capture Permission</span>
                      <span
                        className="bibo-pill"
                        style={{
                          fontSize: 11,
                          padding: "2px 8px",
                          borderRadius: 999,
                          fontWeight: 700,
                          background: manualOverride ? "rgba(168, 85, 247, 0.15)" : "var(--bg-subtle)",
                          color: manualOverride ? "#c084fc" : "var(--text-muted)",
                        }}
                      >
                        {manualOverride ? "Overridden" : "Inherited"}
                      </span>
                    </div>
                    <div style={{ fontSize: 12, color: "var(--text-muted)", marginBottom: 12 }}>
                      Allows managers to send immediate on-demand screenshot requests.
                    </div>
                    <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
                      <label style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 13, cursor: "pointer" }}>
                        <input
                          type="checkbox"
                          checked={manualOverride}
                          onChange={(e) => setManualOverride(e.target.checked)}
                        />
                        <span>Override</span>
                      </label>
                      {manualOverride && (
                        <div className="bibo-seg bibo-seg--sm">
                          <button
                            type="button"
                            className={`bibo-seg__opt${manualEnabled ? " bibo-seg__opt--on" : ""}`}
                            onClick={() => setManualEnabled(true)}
                          >
                            Enabled
                          </button>
                          <button
                            type="button"
                            className={`bibo-seg__opt${!manualEnabled ? " bibo-seg__opt--on" : ""}`}
                            onClick={() => setManualEnabled(false)}
                          >
                            Disabled
                          </button>
                        </div>
                      )}
                    </div>
                  </div>
                </div>

                <div style={{ display: "flex", justifyContent: "flex-end" }}>
                  <button
                    type="button"
                    className="bibo-btn bibo-btn--primary"
                    onClick={saveManualSettings}
                    disabled={savingPolicy}
                  >
                    {savingPolicy ? "Saving…" : "Save Manual Settings"}
                  </button>
                </div>
              </div>

              {/* Capture Now & Live Status Card */}
              <div
                className="bibo-card bibo-card--default admin-cardpad"
                style={{
                  padding: 20,
                  borderRadius: 14,
                  border: "1px solid var(--border)",
                  background: "linear-gradient(180deg, var(--bg-card) 0%, var(--bg-inset) 100%)",
                }}
              >
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 16 }}>
                  <div>
                    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                      <span style={{ color: "var(--primary)", display: "inline-flex" }}>{IconCamera}</span>
                      <h3 style={{ fontSize: 16, fontWeight: 700, margin: 0 }}>On-Demand Capture</h3>
                    </div>
                    <div style={{ fontSize: 12, color: "var(--text-muted)", marginTop: 4 }}>
                      Request an immediate screenshot from the employee's active desktop session.
                    </div>
                  </div>

                  <button
                    type="button"
                    className="bibo-btn bibo-btn--primary"
                    disabled={!isCaptureNowAllowed}
                    onClick={triggerCaptureNow}
                    style={{
                      display: "inline-flex",
                      alignItems: "center",
                      gap: 8,
                      padding: "10px 18px",
                      fontSize: 14,
                      fontWeight: 600,
                    }}
                    title={
                      isMasterDisabled
                        ? "Screenshot collection is disabled"
                        : isManualDisabled
                        ? "Manual capture is disabled"
                        : "Take immediate screenshot"
                    }
                  >
                    <span style={{ display: "inline-flex", lineHeight: 0 }}>{IconCamera}</span>
                    <span>{capturing ? "Submitting Request…" : "Capture Now"}</span>
                  </button>
                </div>

                {captureMsg && (
                  <div style={{ marginTop: 14 }}>
                    <Notice kind={captureMsg.kind}>{captureMsg.text}</Notice>
                  </div>
                )}

                {/* Live Request Tracker */}
                {activeRequest && (
                  <div
                    style={{
                      marginTop: 16,
                      padding: 14,
                      borderRadius: 10,
                      background: "rgba(0,0,0,0.2)",
                      border: "1px solid var(--border-subtle)",
                      display: "flex",
                      justifyContent: "space-between",
                      alignItems: "center",
                      flexWrap: "wrap",
                      gap: 12,
                    }}
                  >
                    <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
                      <span style={{ fontSize: 13, fontWeight: 600 }}>
                        Active Request: <span style={{ fontFamily: "monospace" }}>#{activeRequest.id.slice(0, 8)}</span>
                      </span>
                      <span style={{ fontSize: 12, color: "var(--text-muted)" }}>
                        Initiated {fmtRelative(Math.floor(Date.parse(activeRequest.created_at) / 1000))}
                      </span>
                    </div>

                    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                      <span
                        className={`admin-status-badge admin-status-badge--${activeRequest.status}`}
                        style={{
                          fontSize: 12,
                          fontWeight: 700,
                          padding: "4px 10px",
                          borderRadius: 999,
                          textTransform: "capitalize",
                          background:
                            activeRequest.status === "completed"
                              ? "rgba(34, 197, 94, 0.15)"
                              : activeRequest.status === "acknowledged"
                              ? "rgba(59, 130, 246, 0.15)"
                              : activeRequest.status === "pending"
                              ? "rgba(234, 179, 8, 0.15)"
                              : "rgba(239, 68, 68, 0.15)",
                          color:
                            activeRequest.status === "completed"
                              ? "#4ade80"
                              : activeRequest.status === "acknowledged"
                              ? "#60a5fa"
                              : activeRequest.status === "pending"
                              ? "#facc15"
                              : "#f87171",
                          border: "1px solid currentColor",
                        }}
                      >
                        {activeRequest.status === "acknowledged"
                          ? "Capturing & Uploading…"
                          : activeRequest.status === "pending"
                          ? "Pending Desktop…"
                          : activeRequest.status}
                      </span>
                    </div>
                  </div>
                )}

                {/* Recent Requests History */}
                {requests.length > 0 && (
                  <div style={{ marginTop: 16 }}>
                    <div style={{ fontSize: 12, fontWeight: 600, marginBottom: 8, color: "var(--text-muted)" }}>
                      Recent Capture Requests ({requests.length})
                    </div>
                    <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                      {requests.slice(0, 4).map((r) => (
                        <div
                          key={r.id}
                          style={{
                            display: "flex",
                            justifyContent: "space-between",
                            alignItems: "center",
                            padding: "6px 12px",
                            background: "var(--bg-inset)",
                            borderRadius: 6,
                            fontSize: 12,
                          }}
                        >
                          <span style={{ fontFamily: "monospace" }}>#{r.id.slice(0, 8)}</span>
                          <span style={{ color: "var(--text-muted)" }}>
                            {fmtRelative(Math.floor(Date.parse(r.created_at) / 1000))}
                          </span>
                          <span style={{ textTransform: "capitalize", fontWeight: 600 }}>
                            {r.status === "completed" ? "✓ Completed" : r.status.replace("_", " ")}
                          </span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>

              {/* Manual Gallery */}
              <div
                className="bibo-card bibo-card--default admin-cardpad"
                style={{ padding: 20, borderRadius: 14 }}
              >
                {errorManual && <Notice kind="danger">{errorManual}</Notice>}

                {loadingManual && manualShots.length === 0 ? (
                  <Spinner label="Loading manual screenshots…" />
                ) : (
                  <ScreenshotGallery
                    shots={manualShots}
                    title="Manually Captured Screenshots"
                    subtitle="Only screenshots associated with manual capture requests"
                    sourceType="manual"
                    employeeName={employeeName}
                    onRefresh={() => loadManualScreenshots(0, false)}
                    refreshing={loadingManual}
                    onLoadMore={hasMoreManual ? () => loadManualScreenshots(manualOffset + 60, true) : undefined}
                    loadingMore={loadingManual && manualShots.length > 0}
                    emptyMessage="No manual screenshots found. Click 'Capture Now' above to request an immediate capture."
                  />
                )}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
