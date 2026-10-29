import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../auth/AuthContext";
import { useTheme, type ThemeMode } from "../theme/ThemeProvider";
import { useBusinesses } from "../useBusinesses";
import { memberTerms } from "../terms";
import { DetailHeaderContext } from "../detailHeader";
import { LanguageSwitcher } from "./LanguageSwitcher";

/** Finjulia circular pulse brand logo (matches desktop). */
function BrandLogo() {
  return (
    <span className="brand-logo" aria-hidden>
      <svg viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <circle cx="12" cy="12" r="9" />
        <path d="M4.5 12 h3.2 l1.8 -4.4 l2.4 8.8 l1.8 -4.4 h4.5" />
      </svg>
    </span>
  );
}

/** Shared lucide-style icon frame (16×16, stroke = currentColor). */
function NavIcon({ children }: { children: ReactNode }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      {children}
    </svg>
  );
}

const DashboardIcon = () => (
  <NavIcon>
    <rect width="7" height="9" x="3" y="3" rx="1" />
    <rect width="7" height="5" x="14" y="3" rx="1" />
    <rect width="7" height="9" x="14" y="12" rx="1" />
    <rect width="7" height="5" x="3" y="16" rx="1" />
  </NavIcon>
);

const MembersIcon = () => (
  <NavIcon>
    <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
    <path d="M16 3.128a4 4 0 0 1 0 7.744" />
    <path d="M22 21v-2a4 4 0 0 0-3-3.87" />
    <circle cx="9" cy="7" r="4" />
  </NavIcon>
);

const SettingsIcon = () => (
  <NavIcon>
    <path d="M9.671 4.136a2.34 2.34 0 0 1 4.659 0 2.34 2.34 0 0 0 3.319 1.915 2.34 2.34 0 0 1 2.33 4.033 2.34 2.34 0 0 0 0 3.831 2.34 2.34 0 0 1-2.33 4.033 2.34 2.34 0 0 0-3.319 1.915 2.34 2.34 0 0 1-4.659 0 2.34 2.34 0 0 0-3.32-1.915 2.34 2.34 0 0 1-2.33-4.033 2.34 2.34 0 0 0 0-3.831A2.34 2.34 0 0 1 6.35 6.051a2.34 2.34 0 0 0 3.319-1.915" />
    <circle cx="12" cy="12" r="3" />
  </NavIcon>
);

const MessagesIcon = () => (
  <NavIcon>
    <path d="m3 11 18-5v12L3 14v-3z" />
    <path d="M11.6 16.8a3 3 0 1 1-5.8-1.6" />
  </NavIcon>
);

const ChevronsUpDownIcon = () => (
  <svg
    xmlns="http://www.w3.org/2000/svg"
    width="15"
    height="15"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden
  >
    <path d="m7 15 5 5 5-5" />
    <path d="m7 9 5-5 5 5" />
  </svg>
);

const CheckIcon = () => (
  <svg
    xmlns="http://www.w3.org/2000/svg"
    width="16"
    height="16"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden
  >
    <path d="M20 6 9 17l-5-5" />
  </svg>
);

const PlusIcon = () => (
  <svg
    xmlns="http://www.w3.org/2000/svg"
    width="15"
    height="15"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden
  >
    <path d="M5 12h14" />
    <path d="M12 5v14" />
  </svg>
);

const LogOutIcon = () => (
  <svg
    xmlns="http://www.w3.org/2000/svg"
    width="16"
    height="16"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden
  >
    <path d="m16 17 5-5-5-5" />
    <path d="M21 12H9" />
    <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
  </svg>
);

const MenuToggleIcon = () => (
  <svg
    xmlns="http://www.w3.org/2000/svg"
    width="20"
    height="20"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden
  >
    <line x1="3" y1="12" x2="21" y2="12" />
    <line x1="3" y1="6" x2="21" y2="6" />
    <line x1="3" y1="18" x2="21" y2="18" />
  </svg>
);

/** Two-letter monogram from a name (falls back to the first two chars). */
function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

/** Closes `open` on outside-click / Escape. */
function useDismiss(open: boolean, close: () => void) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    function onDoc(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) close();
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") close();
    }
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [open, close]);
  return ref;
}

/** Topbar business switcher — a pill showing the current business name. */
function BizPicker() {
  const { t } = useTranslation("dashboard");
  const navigate = useNavigate();
  const { businesses, selected, selectedId, setSelectedId } = useBusinesses();
  const [open, setOpen] = useState(false);
  const ref = useDismiss(open, () => setOpen(false));

  if (!selected) return null;

  return (
    <div className="admin-bizpick" ref={ref}>
      <button
        type="button"
        className="admin-bizpick__btn"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        <span className="admin-bizpick__ic">{initials(selected.name)}</span>
        <span className="admin-bizpick__name">{selected.name}</span>
        <ChevronsUpDownIcon />
      </button>
      {open && (
        <div className="admin-menu admin-menu--left" role="menu">
          {businesses.map((b) => (
            <button
              key={b.id}
              type="button"
              role="menuitemradio"
              aria-checked={b.id === selectedId}
              className={`admin-menu__opt${b.id === selectedId ? " on" : ""}`}
              onClick={() => {
                setSelectedId(b.id);
                setOpen(false);
              }}
            >
              <span className="admin-bizpick__ic">{initials(b.name)}</span>
              <span className="admin-menu__label">{b.name}</span>
              {b.id === selectedId && <CheckIcon />}
            </button>
          ))}
          <div className="admin-menu__sep" />
          <button
            type="button"
            role="menuitem"
            className="admin-menu__opt admin-menu__action"
            onClick={() => {
              setOpen(false);
              navigate("/employees?new=1");
            }}
          >
            <span className="admin-menu__plus">
              <PlusIcon />
            </span>
            <span className="admin-menu__label">{t("dashboard.newTeam")}</span>
          </button>
        </div>
      )}
    </div>
  );
}

/** Topbar account button — avatar that opens a clean dropdown with sign out. */
function AccountMenu() {
  const { t } = useTranslation();
  const { user, logout } = useAuth();
  const [open, setOpen] = useState(false);
  const ref = useDismiss(open, () => setOpen(false));
  const displayName = user?.display_name ?? user?.email ?? "";
  const email = user?.email ?? user?.username ?? "";

  return (
    <div className="admin-acct" ref={ref}>
      <button
        type="button"
        className="admin-acct-btn"
        aria-label={displayName}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        <span className="bibo-avatar bibo-avatar--sm">
          <span className="bibo-avatar__img">{initials(displayName)}</span>
          <span className="bibo-avatar__dot bibo-avatar__dot--active" />
        </span>
      </button>
      {open && (
        <div className="admin-menu admin-menu--right" role="menu" style={{ minWidth: 200 }}>
          <div style={{ padding: "8px 10px 10px" }}>
            <div style={{ fontSize: "13.5px", fontWeight: 700 }} title={displayName}>
              {displayName}
            </div>
            <div style={{ fontSize: "12px", color: "var(--text-muted)", marginTop: 2 }}>{email}</div>
          </div>
          <div className="admin-menu__sep" />
          <button
            className="admin-menu__opt"
            role="menuitem"
            onClick={logout}
            style={{ color: "var(--danger)" }}
          >
            <span style={{ display: "inline-flex", lineHeight: 0 }}>
              <LogOutIcon />
            </span>
            {t("actions.signOut")}
          </button>
        </div>
      )}
    </div>
  );
}

export function AppShell() {
  const { t } = useTranslation();
  const { user } = useAuth();
  const { mode, setMode } = useTheme();
  const { selected } = useBusinesses();
  const terms = memberTerms(selected?.kind);
  const location = useLocation();
  const [mobileOpen, setMobileOpen] = useState(false);

  // Close mobile sidebar on route change
  useEffect(() => {
    setMobileOpen(false);
  }, [location.pathname]);

  const MANAGEMENT_NAV = [
    { to: "/", label: t("nav.dashboard"), end: true, icon: <DashboardIcon /> },
    { to: "/employees", label: terms.many, end: false, icon: <MembersIcon /> },
    { to: "/settings", label: t("nav.settings"), end: false, icon: <SettingsIcon /> },
  ];

  const SUPER_NAV = user?.is_super_admin
    ? [{ to: "/internal/messages", label: t("nav.messages"), end: false, icon: <MessagesIcon /> }]
    : [];

  const allNav = [...MANAGEMENT_NAV, ...SUPER_NAV];
  const activeNav = allNav.find((n) =>
    n.end ? location.pathname === n.to : location.pathname.startsWith(n.to),
  );
  const baseTitle = activeNav?.label ?? t("nav.dashboard");

  const isDetail = location.pathname.startsWith("/employees/");
  const [detailTitle, setDetailTitle] = useState<string | null>(null);
  const detailHeader = useMemo(() => ({ setTitle: setDetailTitle }), []);
  const title = isDetail ? detailTitle ?? baseTitle : baseTitle;

  const displayName = user?.display_name ?? user?.email ?? "";

  return (
    <div className={`app${mobileOpen ? " app--mobile-open" : ""}`}>
      {/* Mobile backdrop */}
      {mobileOpen && (
        <div
          className="sidebar-backdrop"
          onClick={() => setMobileOpen(false)}
          aria-hidden="true"
        />
      )}

      {/* Finjulia Sidebar */}
      <aside className={`sidebar${mobileOpen ? " sidebar--open" : ""}`}>
        <div className="brand">
          <BrandLogo />
          <span className="brand-text">
            <span className="brand-name">Finjulia</span>
            <span className="brand-badge">Admin</span>
          </span>
        </div>

        <nav className="nav">
          <div className="nav-group">
            <div className="nav-group__label">{t("nav.group.management", { defaultValue: "MANAGEMENT" })}</div>
            {MANAGEMENT_NAV.map((n) => (
              <NavLink
                key={n.to}
                to={n.to}
                end={n.end}
                className={({ isActive }) => `nav-item${isActive ? " active" : ""}`}
              >
                <span className="nav-ic">{n.icon}</span>
                <span className="nav-label">{n.label}</span>
              </NavLink>
            ))}
          </div>

          {SUPER_NAV.length > 0 && (
            <div className="nav-group">
              <div className="nav-group__label">{t("nav.group.system", { defaultValue: "SUPER ADMIN" })}</div>
              {SUPER_NAV.map((n) => (
                <NavLink
                  key={n.to}
                  to={n.to}
                  end={n.end}
                  className={({ isActive }) => `nav-item${isActive ? " active" : ""}`}
                >
                  <span className="nav-ic">{n.icon}</span>
                  <span className="nav-label">{n.label}</span>
                </NavLink>
              ))}
            </div>
          )}
        </nav>

        <div className="sidebar-foot">
          <div className="sidebar-user">
            <span className="bibo-avatar bibo-avatar--sm" aria-label={displayName}>
              <span className="bibo-avatar__img">{initials(displayName)}</span>
              <span className="bibo-avatar__dot bibo-avatar__dot--active" />
            </span>
            <div className="sidebar-user__info">
              <div className="sidebar-user__name" title={displayName}>
                {displayName}
              </div>
              <div className="sidebar-user__role">
                {user?.is_super_admin ? "Super Admin" : "Admin"}
              </div>
            </div>
          </div>
        </div>
      </aside>

      {/* Main Content Area */}
      <main className="main">
        <header className="admin-topbar">
          <div className="admin-topbar__left">
            <button
              type="button"
              className="admin-menu-toggle"
              onClick={() => setMobileOpen((o) => !o)}
              aria-label="Toggle navigation"
            >
              <MenuToggleIcon />
            </button>
            <div className="admin-topbar__title">{title}</div>
          </div>

          <div className="admin-topbar__right">
            {!isDetail && <BizPicker />}
            <LanguageSwitcher />
            <div className="bibo-seg bibo-seg--sm" role="tablist" aria-label={t("language")}>
              {(["light", "dark", "system"] as ThemeMode[]).map((m) => (
                <button
                  key={m}
                  type="button"
                  role="tab"
                  aria-selected={m === mode}
                  className={`bibo-seg__opt${m === mode ? " bibo-seg__opt--on" : ""}`}
                  onClick={() => setMode(m)}
                >
                  {m === "light" ? t("theme.light") : m === "dark" ? t("theme.dark") : t("theme.auto")}
                </button>
              ))}
            </div>
            <AccountMenu />
          </div>
        </header>

        <div className={`content${location.pathname === "/" ? " content--flat" : ""}`}>
          <DetailHeaderContext.Provider value={detailHeader}>
            <Outlet />
          </DetailHeaderContext.Provider>
        </div>
      </main>
    </div>
  );
}
