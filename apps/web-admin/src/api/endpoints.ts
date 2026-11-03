import { request } from "./client";
import { tokenStore } from "./tokenStore";
import {
  demoActivity,
  demoBrowser,
  demoBusinesses,
  demoEmployees,
  demoKeystrokes,
  demoRoster,
  demoScreenshots,
  isDemo,
} from "./demo";
import type {
  AccountType,
  AdminMessageRow,
  EventCounts,
  MessageDef,
  MessageStats,
  ActivityResponse,
  AuthResponse,
  BrowserVisit,
  Business,
  BusinessSettingsPatch,
  CreateEmployeeResponse,
  Employee,
  KeystrokeBucket,
  PrivacyAppCategory,
  ReportEmployee,
  ScreenshotsResponse,
  Tokens,
  User,
  EmployeePolicyDetail,
  EmployeePolicyPatch,
  ScreenshotRequest,
  CreateTeamReq,
  DemoteMemberReq,
  GrantPermissionReq,
  MyAccess,
  OrgAuditLog,
  OrgMember,
  PromoteMemberReq,
  RemoveMemberReq,
  SuspendMemberReq,
  Team,
  TeamWithDetails,
  UpdateTeamReq,
  UserPermission,
} from "./types";

// ---------- auth ----------
export async function login(identifier: string, password: string, business_id?: string) {
  const res = await request<AuthResponse>("/v1/auth/login", {
    method: "POST",
    auth: false,
    body: { identifier, password, business_id },
  });
  tokenStore.setSession(res.tokens, res.user);
  return res;
}

export async function register(
  identifier: string,
  password: string,
  display_name: string,
  account_type: AccountType = "manager",
) {
  // One field accepts an email or a username; route it to the right body key.
  const isEmail = identifier.includes("@");
  const res = await request<AuthResponse>("/v1/auth/register", {
    method: "POST",
    auth: false,
    body: {
      email: isEmail ? identifier : undefined,
      username: isEmail ? undefined : identifier.toLowerCase(),
      password,
      display_name,
      account_type,
    },
  });
  tokenStore.setSession(res.tokens, res.user);
  return res;
}

export function refresh(refresh_token: string) {
  return request<Tokens>("/v1/auth/refresh", {
    method: "POST",
    auth: false,
    body: { refresh_token },
  });
}

export function getMe() {
  return request<User>("/v1/me");
}

// ---------- businesses ----------
export function createBusiness(name: string) {
  return request<Business>("/v1/businesses", { method: "POST", body: { name } });
}

export function listMyBusinesses() {
  if (isDemo()) return Promise.resolve({ businesses: demoBusinesses });
  return request<{ businesses: Business[] }>("/v1/businesses/mine");
}

export function updateBusinessSettings(id: string, patch: BusinessSettingsPatch) {
  return request<{ status: string }>(`/v1/businesses/${id}/settings`, {
    method: "PATCH",
    body: patch,
  });
}

export function getPrivacyApps() {
  return request<{ categories: PrivacyAppCategory[] }>("/v1/public/screenshot-privacy-apps");
}

export function cleanupScreenshots(id: string, olderThanDays: number) {
  return request<{ deleted_count: number; bytes_freed: number }>(
    `/v1/businesses/${id}/screenshots/cleanup`,
    { method: "POST", query: { older_than_days: olderThanDays } },
  );
}

// ---------- employees ----------
export function createEmployee(input: {
  email?: string;
  username?: string;
  password: string;
  display_name: string;
  business_id?: string;
}) {
  return request<CreateEmployeeResponse>("/v1/employees", {
    method: "POST",
    body: input,
  });
}

export function listBusinessEmployees(businessId: string) {
  if (isDemo()) return Promise.resolve({ employees: demoEmployees() });
  return request<{ employees: Employee[] }>(`/v1/businesses/${businessId}/employees`);
}

// ---------- reports ----------
// "Today" for the roster starts at the viewer's local midnight (not the server's
// UTC day), so it matches the per-employee views, which use local dates.
export function reportEmployees(businessId: string) {
  if (isDemo()) return Promise.resolve({ employees: demoRoster() });
  const midnight = new Date();
  midnight.setHours(0, 0, 0, 0);
  return request<{ employees: ReportEmployee[] }>("/v1/reports/employees", {
    query: { business_id: businessId, day_start: Math.floor(midnight.getTime() / 1000) },
  });
}

export function reportActivity(employeeId: string, from: number, to: number) {
  if (isDemo()) return Promise.resolve(demoActivity(employeeId));
  return request<ActivityResponse>(`/v1/reports/employees/${employeeId}/activity`, {
    query: { from, to },
  });
}

export function reportKeystrokes(employeeId: string, from: number, to: number) {
  if (isDemo()) return Promise.resolve(demoKeystrokes(employeeId));
  return request<{ buckets: KeystrokeBucket[] }>(
    `/v1/reports/employees/${employeeId}/keystrokes`,
    { query: { from, to } },
  );
}

export function reportBrowser(employeeId: string, from: number, to: number) {
  if (isDemo()) return Promise.resolve(demoBrowser(employeeId));
  return request<{ visits: BrowserVisit[] }>(`/v1/reports/employees/${employeeId}/browser`, {
    query: { from, to },
  });
}

export function reportScreenshots(
  employeeId: string,
  from: number,
  to: number,
  limit = 60,
  offset = 0,
  source?: string,
) {
  if (isDemo()) return Promise.resolve(demoScreenshots(employeeId));
  const query: Record<string, string | number> = { from, to, limit, offset };
  if (source) {
    query.source = source;
  }
  return request<ScreenshotsResponse>(`/v1/reports/employees/${employeeId}/screenshots`, {
    query,
  });
}

// ---------- screenshot policy & manual requests (manager) ----------
export function getEmployeePolicy(businessId: string, employeeId: string) {
  return request<EmployeePolicyDetail>(`/v1/businesses/${businessId}/employees/${employeeId}/policy`);
}

export function updateEmployeePolicy(businessId: string, employeeId: string, patch: EmployeePolicyPatch) {
  return request<EmployeePolicyDetail>(`/v1/businesses/${businessId}/employees/${employeeId}/policy`, {
    method: "PATCH",
    body: patch,
  });
}

export function resetEmployeePolicy(businessId: string, employeeId: string) {
  return request<{ status: string; policy: EmployeePolicyDetail }>(
    `/v1/businesses/${businessId}/employees/${employeeId}/policy/reset`,
    { method: "POST" },
  );
}

export function createScreenshotRequest(businessId: string, employeeId: string, expiryMinutes = 15) {
  return request<ScreenshotRequest>(
    `/v1/businesses/${businessId}/employees/${employeeId}/screenshot-requests`,
    {
      method: "POST",
      body: { expiry_minutes: expiryMinutes },
    },
  );
}

export function listEmployeeScreenshotRequests(businessId: string, employeeId: string, limit = 20) {
  return request<{ requests: ScreenshotRequest[] }>(
    `/v1/businesses/${businessId}/employees/${employeeId}/screenshot-requests`,
    { query: { limit } },
  );
}

export function retryScreenshotRequest(businessId: string, requestId: string) {
  return request<ScreenshotRequest>(
    `/v1/businesses/${businessId}/screenshot-requests/${requestId}/retry`,
    { method: "POST" },
  );
}

// ---------- in-app messages (super admin) ----------
export function adminListMessages() {
  return request<{ messages: AdminMessageRow[]; whatsnew: { id: string; stats: EventCounts }[] }>(
    "/v1/admin/messages",
  );
}
export function adminGetMessage(id: string) {
  return request<MessageDef>(`/v1/admin/messages/${encodeURIComponent(id)}`);
}
export function adminCreateMessage(m: MessageDef) {
  return request<{ id: string }>("/v1/admin/messages", { method: "POST", body: m });
}
export function adminUpdateMessage(m: MessageDef) {
  return request<{ id: string }>(`/v1/admin/messages/${encodeURIComponent(m.id)}`, { method: "PUT", body: m });
}
export function adminDeleteMessage(id: string) {
  return request<{ ok: boolean }>(`/v1/admin/messages/${encodeURIComponent(id)}`, { method: "DELETE" });
}
export function adminMessageStats(id: string) {
  return request<MessageStats>(`/v1/admin/messages/${encodeURIComponent(id)}/stats`);
}

// ---------- Organizational Hierarchy & RBAC ----------

export function getMyAccess(businessId: string) {
  return request<MyAccess>(`/v1/businesses/${businessId}/my-access`);
}

export function listOrgMembers(businessId: string) {
  return request<{ members: OrgMember[] }>(`/v1/businesses/${businessId}/members`);
}

export function listTeams(businessId: string) {
  return request<{ teams: TeamWithDetails[] }>(`/v1/businesses/${businessId}/teams`);
}

export function createTeam(businessId: string, req: CreateTeamReq) {
  return request<Team>(`/v1/businesses/${businessId}/teams`, {
    method: "POST",
    body: req,
  });
}

export function getTeam(businessId: string, teamId: string) {
  return request<Team>(`/v1/businesses/${businessId}/teams/${teamId}`);
}

export function updateTeam(businessId: string, teamId: string, req: UpdateTeamReq) {
  return request<Team>(`/v1/businesses/${businessId}/teams/${teamId}`, {
    method: "PATCH",
    body: req,
  });
}

export function deleteTeam(businessId: string, teamId: string) {
  return request<{ status: string }>(`/v1/businesses/${businessId}/teams/${teamId}`, {
    method: "DELETE",
  });
}

export function assignTeamMember(businessId: string, teamId: string, userId: string) {
  return request<{ status: string }>(`/v1/businesses/${businessId}/teams/${teamId}/members`, {
    method: "POST",
    body: { user_id: userId },
  });
}

export function removeTeamMember(businessId: string, teamId: string, userId: string) {
  return request<{ status: string }>(`/v1/businesses/${businessId}/teams/${teamId}/members/${userId}`, {
    method: "DELETE",
  });
}

export function promoteMember(businessId: string, userId: string, req: PromoteMemberReq) {
  return request<{ status: string }>(`/v1/businesses/${businessId}/members/${userId}/promote`, {
    method: "POST",
    body: req,
  });
}

export function demoteMember(businessId: string, userId: string, req: DemoteMemberReq) {
  return request<{ status: string }>(`/v1/businesses/${businessId}/members/${userId}/demote`, {
    method: "POST",
    body: req,
  });
}

export function suspendMember(businessId: string, userId: string, req: SuspendMemberReq) {
  return request<{ status: string; is_suspended: boolean }>(`/v1/businesses/${businessId}/members/${userId}/suspend`, {
    method: "POST",
    body: req,
  });
}

export function reactivateMember(businessId: string, userId: string) {
  return request<{ status: string; is_suspended: boolean }>(`/v1/businesses/${businessId}/members/${userId}/reactivate`, {
    method: "POST",
  });
}

export function removeMember(businessId: string, userId: string, req: RemoveMemberReq) {
  return request<{ status: string }>(`/v1/businesses/${businessId}/members/${userId}`, {
    method: "DELETE",
    body: req,
  });
}

export function listMemberPermissions(businessId: string, userId: string) {
  return request<{ permissions: UserPermission[] }>(`/v1/businesses/${businessId}/members/${userId}/permissions`);
}

export function grantPermission(businessId: string, userId: string, req: GrantPermissionReq) {
  return request<{ status: string }>(`/v1/businesses/${businessId}/members/${userId}/permissions`, {
    method: "POST",
    body: req,
  });
}

export function revokePermission(businessId: string, userId: string, permission: string) {
  return request<{ status: string }>(`/v1/businesses/${businessId}/members/${userId}/permissions/${permission}`, {
    method: "DELETE",
  });
}

export function listOrgAuditLogs(businessId: string, limit = 50, offset = 0) {
  return request<{ logs: OrgAuditLog[] }>(`/v1/businesses/${businessId}/audit-logs`, {
    query: { limit, offset },
  });
}
