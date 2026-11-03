import { useEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { fetchImageObjectUrl } from "../../api/client";
import type { ScreenshotMeta } from "../../api/types";
import { fmtBytes, fmtTime } from "../../format";
import { Empty, Spinner } from "../ui";

// A screenshot may carry the active app (demo/enriched data) for the badge.
type ShotMeta = ScreenshotMeta & { app?: string };

// Per-app placeholder gradient shown until the real thumbnail loads (or if the
// image is unavailable).
const APP_GRADIENTS: Record<string, [string, string]> = {
  "VS Code": ["#6c5ce7", "#3a2f87"],
  Chrome: ["#38bdf8", "#1f6f9c"],
  Figma: ["#34d399", "#0f7a5a"],
  Terminal: ["#2dd4bf", "#136b60"],
  Notion: ["#f472b6", "#9c3a6e"],
  Slack: ["#fbbf24", "#9c7414"],
  Zoom: ["#56ccf2", "#2f6f9c"],
  Spotify: ["#34d399", "#0f7a5a"],
};
const gradientFor = (app?: string) => {
  const [a, b] = (app && APP_GRADIENTS[app]) || APP_GRADIENTS["VS Code"];
  return `linear-gradient(135deg, ${a}, ${b})`;
};

const hhmmss = (ts: number) => {
  const d = new Date(ts * 1000);
  return [d.getHours(), d.getMinutes(), d.getSeconds()]
    .map((x) => String(x).padStart(2, "0"))
    .join(":");
};

const fullDateTime = (ts: number) => {
  return new Date(ts * 1000).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
};

const svg = (children: ReactNode, size = 20) => (
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

const IconX = svg(<path d="M18 6 6 18M6 6l12 12" />, 20);
const IconChevronLeft = svg(<path d="m15 18-6-6 6-6" />, 24);
const IconChevronRight = svg(<path d="m9 18 6-6-6-6" />, 24);
const IconRotate = svg(<><path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8" /><path d="M21 3v5h-5" /></>, 14);
const IconAlert = svg(<><circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="12" /><line x1="12" y1="16" x2="12.01" y2="16" /></>, 28);

// One gallery card: real thumbnail when it loads, gradient placeholder before.
function Shot({
  meta,
  employeeName,
  onOpen,
}: {
  meta: ShotMeta;
  employeeName?: string;
  onOpen: () => void;
}) {
  const [url, setUrl] = useState<string | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    let alive = true;
    let made: string | null = null;
    setError(false);
    fetchImageObjectUrl(meta.client_uuid)
      .then((u) => {
        made = u;
        if (alive) setUrl(u);
        else URL.revokeObjectURL(u);
      })
      .catch(() => {
        if (alive) setError(true);
      });
    return () => {
      alive = false;
      if (made) URL.revokeObjectURL(made);
    };
  }, [meta.client_uuid]);

  const isManual = meta.capture_source === "manual" || !!meta.request_id;

  return (
    <div
      className={`admin-shot${url ? " admin-shot--clickable" : ""}`}
      onClick={url ? onOpen : undefined}
      title={
        employeeName
          ? `${employeeName} · ${hhmmss(meta.ts)} (Click to view full)`
          : `Click to view full screenshot (${hhmmss(meta.ts)})`
      }
    >
      <div
        className="admin-shot__img"
        style={url ? { backgroundImage: `url("${url}")` } : { background: gradientFor(meta.app) }}
      />
      <div className="admin-shot__veil" />

      {/* Source Badge */}
      <span className={`admin-shot__badge ${isManual ? "admin-shot__badge--manual" : "admin-shot__badge--auto"}`}>
        {isManual ? "MANUAL" : "AUTO"}
      </span>

      {/* App Badge */}
      {meta.app && <span className="admin-shot__app">{meta.app}</span>}

      {/* Timestamp */}
      <span className="admin-shot__t">{hhmmss(meta.ts)}</span>

      {/* Request ID snippet if manual */}
      {isManual && meta.request_id && (
        <span className="admin-shot__reqid" title={`Request: ${meta.request_id}`}>
          #{meta.request_id.slice(0, 6)}
        </span>
      )}

      {error && !url && (
        <span
          style={{
            position: "absolute",
            bottom: 10,
            right: 10,
            fontSize: 10,
            background: "rgba(220, 38, 38, 0.8)",
            color: "#fff",
            padding: "2px 6px",
            borderRadius: 4,
          }}
        >
          Image offline
        </span>
      )}
    </div>
  );
}

// Fullscreen lightbox with prev/next/close.
function Lightbox({
  shots,
  index,
  employeeName,
  onIndex,
  onClose,
}: {
  shots: ScreenshotMeta[];
  index: number;
  employeeName?: string;
  onIndex: (i: number) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation("reports");
  const meta = shots[index];
  const [url, setUrl] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const ref = useRef<string | null>(null);

  const hasPrev = index > 0;
  const hasNext = index < shots.length - 1;
  const isManual = meta.capture_source === "manual" || !!meta.request_id;

  const loadImage = () => {
    let alive = true;
    setLoading(true);
    setError(null);
    setUrl(null);

    fetchImageObjectUrl(meta.client_uuid)
      .then((u) => {
        if (ref.current) URL.revokeObjectURL(ref.current);
        ref.current = u;
        if (alive) {
          setUrl(u);
          setLoading(false);
        } else {
          URL.revokeObjectURL(u);
        }
      })
      .catch((err) => {
        if (alive) {
          setLoading(false);
          setError(err instanceof Error ? err.message : "Failed to load screenshot");
        }
      });

    return () => {
      alive = false;
      if (ref.current) {
        URL.revokeObjectURL(ref.current);
        ref.current = null;
      }
    };
  };

  useEffect(() => {
    return loadImage();
  }, [meta.client_uuid]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      else if (e.key === "ArrowLeft" && hasPrev) onIndex(index - 1);
      else if (e.key === "ArrowRight" && hasNext) onIndex(index + 1);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [index, hasPrev, hasNext, onIndex, onClose]);

  return createPortal(
    <div className="admin-lightbox" onClick={onClose} role="dialog" aria-modal="true">
      {/* Top Header */}
      <div className="admin-lightbox__header" onClick={(e) => e.stopPropagation()}>
        <span className="admin-lightbox__title">
          Screenshot {index + 1} of {shots.length}
        </span>
        <span className={`admin-lightbox__badge ${isManual ? "admin-lightbox__badge--manual" : "admin-lightbox__badge--auto"}`}>
          {isManual ? "Manual Request" : "Automatic Continuous"}
        </span>
        {isManual && meta.request_id && (
          <span style={{ fontSize: 12, opacity: 0.8, fontFamily: "monospace" }}>
            ID: {meta.request_id}
          </span>
        )}
      </div>

      <button
        type="button"
        className="admin-lightbox__btn admin-lightbox__close"
        aria-label={t("screenshots.close")}
        onClick={onClose}
      >
        {IconX}
      </button>

      {hasPrev && (
        <button
          type="button"
          className="admin-lightbox__btn admin-lightbox__nav admin-lightbox__nav--prev"
          aria-label={t("screenshots.prev")}
          onClick={(e) => {
            e.stopPropagation();
            onIndex(index - 1);
          }}
        >
          {IconChevronLeft}
        </button>
      )}

      {hasNext && (
        <button
          type="button"
          className="admin-lightbox__btn admin-lightbox__nav admin-lightbox__nav--next"
          aria-label={t("screenshots.next")}
          onClick={(e) => {
            e.stopPropagation();
            onIndex(index + 1);
          }}
        >
          {IconChevronRight}
        </button>
      )}

      {/* Main Image Body */}
      <figure className="admin-lightbox__body" onClick={(e) => e.stopPropagation()}>
        {loading && <Spinner label={t("screenshots.loading")} />}

        {error && !loading && (
          <div className="admin-lightbox__error">
            {IconAlert}
            <div>
              <div style={{ fontWeight: 600, fontSize: 15, marginBottom: 4 }}>
                Unable to display screenshot
              </div>
              <div style={{ fontSize: 13, opacity: 0.8, marginBottom: 12 }}>{error}</div>
              <button
                type="button"
                className="bibo-btn bibo-btn--secondary bibo-btn--sm"
                onClick={loadImage}
              >
                Retry Loading
              </button>
            </div>
          </div>
        )}

        {url && !loading && (
          <img
            className="admin-lightbox__img"
            src={url}
            alt={t("screenshots.alt", { time: fmtTime(meta.ts) })}
          />
        )}

        <figcaption className="admin-lightbox__caption">
          <span>🕒 {fullDateTime(meta.ts)}</span>
          {meta.width && meta.height && (
            <span>📐 {meta.width} × {meta.height}</span>
          )}
          <span>🖥️ {t("screenshots.display", { id: meta.display_id ?? 0 })}</span>
          <span>📦 {fmtBytes(meta.byte_size)}</span>
          {employeeName && <span>👤 {employeeName}</span>}
        </figcaption>
      </figure>
    </div>,
    document.body,
  );
}

export function ScreenshotGallery({
  shots,
  title,
  subtitle,
  sourceType,
  employeeName,
  onRefresh,
  refreshing,
  onLoadMore,
  loadingMore,
  emptyMessage,
}: {
  shots: ScreenshotMeta[];
  title?: string;
  subtitle?: string;
  sourceType?: "automatic" | "manual";
  employeeName?: string;
  onRefresh?: () => void;
  refreshing?: boolean;
  onLoadMore?: () => void;
  loadingMore?: boolean;
  emptyMessage?: string;
}) {
  const { t } = useTranslation("reports");
  const [active, setActive] = useState<number | null>(null);

  return (
    <div className={`admin-gallery-wrap admin-gallery-wrap--${sourceType || "all"}`}>
      <div
        className="admin-panelhead"
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          marginBottom: 16,
        }}
      >
        <div>
          <div className="admin-paneltitle" style={{ fontSize: 16, fontWeight: 700 }}>
            {title || t("screenshots.title")} ({shots.length})
          </div>
          {subtitle && (
            <div style={{ fontSize: 12, color: "var(--text-muted)", marginTop: 2 }}>
              {subtitle}
            </div>
          )}
        </div>
        {onRefresh && (
          <button
            type="button"
            className="bibo-btn bibo-btn--secondary bibo-btn--sm"
            onClick={onRefresh}
            disabled={refreshing}
            style={{ display: "inline-flex", alignItems: "center", gap: 6 }}
          >
            <span style={{ display: "inline-flex", lineHeight: 0 }}>{IconRotate}</span>
            <span>{refreshing ? "Refreshing…" : "Refresh"}</span>
          </button>
        )}
      </div>

      {shots.length === 0 ? (
        <Empty>{emptyMessage || t("screenshots.empty")}</Empty>
      ) : (
        <div className="admin-gallery">
          {shots.map((s, i) => (
            <Shot
              key={s.client_uuid}
              meta={s as ShotMeta}
              employeeName={employeeName}
              onOpen={() => setActive(i)}
            />
          ))}
        </div>
      )}

      {onLoadMore && (
        <div className="admin-gallery__more">
          <button
            type="button"
            className="bibo-btn bibo-btn--secondary"
            onClick={onLoadMore}
            disabled={loadingMore}
          >
            {loadingMore ? t("screenshots.loading") : t("screenshots.loadMore")}
          </button>
        </div>
      )}

      {active != null && shots[active] && (
        <Lightbox
          shots={shots}
          index={active}
          employeeName={employeeName}
          onIndex={setActive}
          onClose={() => setActive(null)}
        />
      )}
    </div>
  );
}
