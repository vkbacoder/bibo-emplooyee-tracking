// Shapes mirror the backend contract in docs/11-backend-and-sync.md.

/** Persona of a self-signup owner. Personal users have no account at all. */
export type AccountType = "manager" | "parent";

export interface User {
  id: string;
  email: string;
  username?: string;
  display_name: string;
  account_type: AccountType;
  /** Listed in the backend's SUPER_ADMINS — unlocks the internal Messages area. */
  is_super_admin?: boolean;
}

export interface Tokens {
  access_token: string;
  refresh_token: string;
  expires_in: number;
}

export interface AuthResponse {
  user: User;
  tokens: Tokens;
}

/** A business is a team or a family; `kind` drives member wording (employees/kids). */
export type BusinessKind = "team" | "family";

/**
 * How employee screens are captured. "privacy" (the default) captures only the
 * frontmost window; "normal" captures every display in full. The backend may
 * still return pre-rename values ("full_screen"/"active_window") — normalize
 * before comparing.
 */
export type ScreenshotMode = "privacy" | "normal";

/** One category of the backend's curated sensitive-app list. */
export interface PrivacyAppCategory {
  key: string;
  apps: string[];
}

export interface Business {
  id: string;
  name: string;
  kind: BusinessKind;
  owner_user_id: string;
  screenshot_retention_days: number | null;
  screenshot_interval_s: number;
  idle_threshold_s: number;
  allow_employee_override: boolean;
  screenshot_mode: string; // normalize to ScreenshotMode before comparing
  screenshot_skip_apps: string[];
  screenshots_enabled?: boolean;
  continuous_capture_enabled?: boolean;
  manual_capture_enabled?: boolean;
  policy_version?: number;
  policy_updated_at?: string;
}

export interface BusinessSettingsPatch {
  screenshot_retention_days?: number | null;
  screenshot_interval_s?: number;
  idle_threshold_s?: number;
  allow_employee_override?: boolean;
  screenshot_mode?: ScreenshotMode;
  screenshot_skip_apps?: string[];
  screenshots_enabled?: boolean;
  continuous_capture_enabled?: boolean;
  manual_capture_enabled?: boolean;
}

export interface EffectivePolicy {
  employee_id: string;
  business_id: string;
  capture_enabled: boolean;
  continuous_capture_enabled: boolean;
  capture_interval_minutes: number;
  capture_interval_seconds: number;
  manual_capture_enabled: boolean;
  policy_version: number;
  policy_updated_at: string;
  last_ack_version?: number;
  last_ack_at?: string;
}

export interface EmployeeOverrides {
  screenshots_enabled: boolean | null;
  continuous_capture_enabled: boolean | null;
  screenshot_interval_s: number | null;
  manual_capture_enabled: boolean | null;
  policy_version: number;
  policy_updated_at: string;
  last_ack_version: number | null;
  last_ack_at: string | null;
  last_ack_device_id: string | null;
}

export interface BusinessScreenshotDefaults {
  screenshots_enabled: boolean;
  continuous_capture_enabled: boolean;
  screenshot_interval_s: number;
  manual_capture_enabled: boolean;
  allow_employee_override: boolean;
  screenshot_mode: string;
  policy_version: number;
  policy_updated_at: string;
}

export interface EmployeePolicyDetail {
  employee_id: string;
  business_id: string;
  effective: EffectivePolicy;
  overrides: EmployeeOverrides;
  business_defaults: BusinessScreenshotDefaults;
}

export interface EmployeePolicyPatch {
  screenshots_enabled?: boolean | null;
  continuous_capture_enabled?: boolean | null;
  screenshot_interval_s?: number | null;
  manual_capture_enabled?: boolean | null;
}

export interface ScreenshotRequest {
  id: string;
  business_id: string;
  employee_id: string;
  requested_by_user_id: string;
  requested_by_name: string;
  status: string;
  policy_version: number;
  created_at: string;
  expires_at: string;
  acknowledged_at?: string;
  completed_at?: string;
  screenshot_client_uuid?: string;
  error_message?: string;
}

export interface Employee {
  id: string;
  email: string;
  username?: string;
  display_name: string;
}

export interface CreateEmployeeResponse {
  employee: Employee;
  business: Business;
}

export interface ReportEmployee {
  id: string;
  email: string;
  username?: string;
  display_name: string;
  role?: "owner" | "employee";
  last_seen: number | null;
  active_today_s: number;
  active_yesterday_s: number;
  screenshots_today: number;
  screenshots_yesterday: number;
  /** 0–100 share of active time with keyboard input; null when no activity today. */
  focus_pct_today: number | null;
}

export interface ActivitySample {
  ts: number;
  app_name: string;
  window_title: string;
  duration_s: number;
}

export interface AppBreakdown {
  app_name: string;
  duration_s: number;
}

export interface ActivityResponse {
  samples: ActivitySample[];
  breakdown: AppBreakdown[];
}

export interface KeystrokeBucket {
  ts_bucket: number;
  count: number;
}

export interface BrowserVisit {
  ts: number;
  url: string;
  page_title: string;
  browser: string;
  duration_s: number;
}

export interface ScreenshotMeta {
  client_uuid: string;
  ts: number;
  byte_size: number;
  width: number | null;
  height: number | null;
  display_id: number | null;
  capture_source?: "automatic" | "manual" | "unknown";
  request_id?: string | null;
  employee_name?: string;
  device_id?: string;
}

export interface ScreenshotsResponse {
  screenshots: ScreenshotMeta[];
  limit: number;
  offset: number;
}

// Thrown by the client for non-2xx responses so UIs can show inline errors.
export class ApiError extends Error {
  status: number;
  body: unknown;
  constructor(status: number, message: string, body: unknown) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
  }
}

// ---------- in-app messages (super admin, ticket 145) ----------
export type MessageKind = "announcement" | "survey" | "promo";
export type MessageFieldType = "text" | "textarea" | "radio" | "checkbox" | "select" | "rating" | "nps";
export type MessageEvent = "delivered" | "shown" | "dismissed" | "later" | "cta" | "submitted";

export interface MessageContent {
  title: string;
  body: string;
  image?: string;
  cta_label?: string;
  cta_url?: string;
}
export interface MessageFieldDef {
  id: string;
  type: MessageFieldType;
  required: boolean;
  label: Record<string, string>;
  options?: { value: string; label: Record<string, string> }[];
}
export interface MessageAudience {
  roles?: string[];
  kinds?: string[];
  platforms?: string[];
  locales?: string[];
  min_version?: string;
  max_version?: string;
}
export interface MessageDef {
  id: string;
  kind: MessageKind;
  active: boolean;
  start?: string | null;
  end?: string | null;
  audience: MessageAudience;
  repeat_days: number;
  anonymous?: boolean;
  content: Record<string, MessageContent>;
  fields?: MessageFieldDef[];
  created_at?: string;
  updated_at?: string;
  /** Survey answers so far — once > 0 the questions can only be reworded. */
  response_count?: number;
}
export type EventCounts = Partial<Record<MessageEvent, { total: number; unique: number }>>;
export interface AdminMessageRow extends MessageDef {
  stats: EventCounts;
}
export interface MessageFieldSummary {
  id: string;
  type: MessageFieldType;
  label: string;
  answered: number;
  options?: { value: string; label: string; count: number }[];
  scale?: Record<string, number>;
  average?: number;
  nps?: number;
  texts?: string[];
}
export interface MessageResponseRow {
  answers: Record<string, string | string[] | number>;
  user_id: string | null;
  user_name: string | null; // named surveys only
  user_login: string | null;
  app_version: string;
  platform: string;
  locale: string;
  created_at: string;
}
export interface MessageStats {
  id: string;
  totals: EventCounts;
  daily: { day: string; event: MessageEvent; unique: number }[];
  breakdown: { dim: "locale" | "platform" | "version" | "role"; value: string; event: MessageEvent; unique: number }[];
  message?: MessageDef;
  responses?: MessageResponseRow[];
  summary?: MessageFieldSummary[];
}

// ─────────────────────────────────────────────────────────────────────
// Organizational Hierarchy & Role-Based Access Control (RBAC)
// ─────────────────────────────────────────────────────────────────────

export type OrgRole = "owner" | "team_lead" | "employee";

export type RbacPermission =
  | "can_view_team"
  | "can_manage_employees"
  | "can_create_team_leads"
  | "can_assign_employees"
  | "can_manage_screenshot_policy"
  | "can_view_screenshots"
  | "can_view_reports"
  | "can_manage_team"
  | "can_view_audit_logs";

export interface Team {
  id: string;
  business_id: string;
  name: string;
  description: string | null;
  lead_user_id: string | null;
  parent_team_id: string | null;
  created_at: string;
  updated_at: string;
}

export interface TeamWithDetails extends Team {
  lead_display_name?: string | null;
  lead_email?: string | null;
  parent_team_name?: string | null;
  member_count: number;
  subteam_count: number;
}

export interface TeamMember {
  id: string;
  business_id: string;
  team_id: string;
  user_id: string;
  created_at: string;
}

export interface OrgMember {
  user_id: string;
  email: string;
  username: string;
  display_name: string;
  role: OrgRole;
  is_suspended: boolean;
  team_id?: string | null;
  team_name?: string | null;
  is_team_lead: boolean;
  led_team_ids?: string[];
  created_at: string;
}

export interface UserPermission {
  id: string;
  business_id: string;
  user_id: string;
  permission: RbacPermission;
  delegable: boolean;
  granted_by_user_id?: string | null;
  created_at: string;
}

export interface MyAccess {
  role: OrgRole;
  is_suspended: boolean;
  is_org_wide: boolean;
  authorized_team_ids: string[];
  permissions: RbacPermission[];
  user_permissions: UserPermission[];
}

export interface OrgAuditLog {
  id: string;
  business_id: string;
  actor_user_id?: string | null;
  actor_email?: string | null;
  actor_display_name?: string | null;
  target_user_id?: string | null;
  target_email?: string | null;
  target_display_name?: string | null;
  target_team_id?: string | null;
  target_team_name?: string | null;
  action: string;
  details: Record<string, any>;
  created_at: string;
}

export interface CreateTeamReq {
  name: string;
  description?: string;
  lead_user_id?: string;
  parent_team_id?: string;
}

export interface UpdateTeamReq {
  name: string;
  description?: string;
  lead_user_id?: string;
  parent_team_id?: string;
}

export interface PromoteMemberReq {
  team_id?: string;
  permissions?: RbacPermission[];
}

export interface DemoteMemberReq {
  replacement_lead_id?: string;
}

export interface SuspendMemberReq {
  replacement_lead_id?: string;
}

export interface RemoveMemberReq {
  replacement_lead_id?: string;
  allow_unassigned_teams?: boolean;
}

export interface GrantPermissionReq {
  permission: RbacPermission;
  delegable?: boolean;
}
