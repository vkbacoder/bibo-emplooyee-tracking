import { useCallback, useEffect, useState } from "react";
import { getMyAccess } from "./api/endpoints";
import type { MyAccess, OrgRole, RbacPermission } from "./api/types";
import { useBusinesses } from "./useBusinesses";
import { isDemo } from "./api/demo";

export function useMyAccess() {
  const { selectedId } = useBusinesses();
  const [access, setAccess] = useState<MyAccess | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (bizId: string) => {
    if (isDemo()) {
      setAccess({
        role: "owner",
        is_suspended: false,
        is_org_wide: true,
        authorized_team_ids: [],
        permissions: [
          "can_view_team",
          "can_manage_employees",
          "can_create_team_leads",
          "can_assign_employees",
          "can_manage_screenshot_policy",
          "can_view_screenshots",
          "can_view_reports",
          "can_manage_team",
          "can_view_audit_logs",
        ],
        user_permissions: [],
      });
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const res = await getMyAccess(bizId);
      setAccess(res);
    } catch (err: any) {
      setError(err?.message || "Failed to load access profile");
      setAccess(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (selectedId) {
      load(selectedId);
    } else {
      setAccess(null);
    }
  }, [selectedId, load]);

  const can = useCallback(
    (permission: RbacPermission): boolean => {
      if (!access) return false;
      if (access.is_suspended) return false;
      if (access.role === "owner") return true;
      return access.permissions.includes(permission);
    },
    [access],
  );

  const role: OrgRole | null = access?.role ?? null;
  const isOwner = role === "owner";
  const isTeamLead = role === "team_lead";
  const isEmployee = role === "employee";
  const isSuspended = access?.is_suspended ?? false;

  return {
    access,
    loading,
    error,
    reload: () => selectedId && load(selectedId),
    role,
    isOwner,
    isTeamLead,
    isEmployee,
    isSuspended,
    can,
  };
}
