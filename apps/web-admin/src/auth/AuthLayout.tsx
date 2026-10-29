import type { ReactNode } from "react";
import { LanguageSwitcher } from "../components/LanguageSwitcher";

/**
 * AuthLayout — the shared welcome surface: centered card on a clean
 * enterprise surface with the Finjulia brand lockup above it.
 */
export function AuthLayout({
  children,
  wide,
  bare,
  hideLockup,
  footer,
}: {
  children: ReactNode;
  wide?: boolean;
  /** Skip the centered card wrapper — used by the two-column rail layout. */
  bare?: boolean;
  /** Hide the brand lockup above the card (sign-in supplies its own inline mark). */
  hideLockup?: boolean;
  footer?: ReactNode;
}) {
  return (
    <div className="auth-wrap welcome">
      <div className="auth-lang">
        <LanguageSwitcher />
      </div>
      {!hideLockup && (
        <div className="auth-lockup" style={{ display: "flex", alignItems: "center", justifyContent: "center", gap: 10, marginBottom: 20 }}>
          <span
            className="brand-logo"
            aria-hidden
            style={{
              width: 36,
              height: 36,
              borderRadius: 9,
              display: "grid",
              placeItems: "center",
              background: "var(--brand-gradient)",
              boxShadow: "0 4px 12px var(--accent-glow)",
            }}
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{ width: 20, height: 20 }}>
              <circle cx="12" cy="12" r="9" />
              <path d="M4.5 12 h3.2 l1.8 -4.4 l2.4 8.8 l1.8 -4.4 h4.5" />
            </svg>
          </span>
          <span style={{ fontSize: 22, fontWeight: 800, letterSpacing: "-0.02em", color: "var(--text)" }}>Finjulia</span>
        </div>
      )}
      {bare ? children : <div className={wide ? "auth-card wide" : "auth-card"}>{children}</div>}
      {footer && <div className="auth-foot">{footer}</div>}
    </div>
  );
}

