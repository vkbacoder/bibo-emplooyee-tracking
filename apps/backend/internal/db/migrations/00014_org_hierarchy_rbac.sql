-- +goose Up

-- 1. Extend membership roles constraint to include 'team_lead'
ALTER TABLE memberships DROP CONSTRAINT IF EXISTS memberships_role_check;
ALTER TABLE memberships ADD CONSTRAINT memberships_role_check 
    CHECK (role IN ('owner', 'team_lead', 'employee'));

-- 2. Business-scoped suspension on memberships
ALTER TABLE memberships ADD COLUMN IF NOT EXISTS is_suspended boolean NOT NULL DEFAULT false;

-- 3. Teams table
CREATE TABLE IF NOT EXISTS teams (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id      uuid NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
    name             text NOT NULL,
    description      text,
    lead_user_id     uuid REFERENCES users(id) ON DELETE SET NULL,
    parent_team_id   uuid REFERENCES teams(id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_teams_biz_id UNIQUE (business_id, id),
    CONSTRAINT chk_team_no_self_parent CHECK (parent_team_id IS NULL OR parent_team_id != id)
);
CREATE INDEX IF NOT EXISTS idx_teams_biz ON teams(business_id);
CREATE INDEX IF NOT EXISTS idx_teams_lead ON teams(lead_user_id);
CREATE INDEX IF NOT EXISTS idx_teams_parent ON teams(parent_team_id);

-- 4. Constraint trigger: Enforce same-business integrity for parent team and lead
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION check_team_business_integrity() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.parent_team_id IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1 FROM teams WHERE id = NEW.parent_team_id AND business_id = NEW.business_id
        ) THEN
            RAISE EXCEPTION 'Parent team % does not belong to business %', NEW.parent_team_id, NEW.business_id;
        END IF;
    END IF;

    IF NEW.lead_user_id IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1 FROM memberships 
             WHERE user_id = NEW.lead_user_id 
               AND business_id = NEW.business_id 
               AND role IN ('owner', 'team_lead')
        ) THEN
            RAISE EXCEPTION 'User % is not an authorized team lead in business %', NEW.lead_user_id, NEW.business_id;
        END IF;

        IF EXISTS (
            SELECT 1 FROM memberships 
             WHERE user_id = NEW.lead_user_id 
               AND business_id = NEW.business_id 
               AND is_suspended = true
        ) THEN
            RAISE EXCEPTION 'Cannot assign suspended member % as team lead in business %', NEW.lead_user_id, NEW.business_id;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_check_team_business_integrity ON teams;
CREATE TRIGGER trg_check_team_business_integrity
    BEFORE INSERT OR UPDATE OF business_id, parent_team_id, lead_user_id ON teams
    FOR EACH ROW EXECUTE FUNCTION check_team_business_integrity();

-- 5. Concurrency-safe hierarchy cycle prevention trigger using transactional advisory locks
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION check_team_hierarchy_cycle() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.parent_team_id IS NOT NULL THEN
        -- Advisory lock serialized per business; holds no physical table locks
        PERFORM pg_advisory_xact_lock(hashtext('finjulia_team_hierarchy_' || NEW.business_id::text));

        IF NEW.parent_team_id = NEW.id THEN
            RAISE EXCEPTION 'A team cannot be its own parent (team: %)', NEW.id;
        END IF;

        IF EXISTS (
            WITH RECURSIVE descendants AS (
                SELECT id FROM teams WHERE parent_team_id = NEW.id AND business_id = NEW.business_id
                UNION ALL
                SELECT t.id FROM teams t 
                JOIN descendants d ON t.parent_team_id = d.id 
                WHERE t.business_id = NEW.business_id
            )
            SELECT 1 FROM descendants WHERE id = NEW.parent_team_id
        ) THEN
            RAISE EXCEPTION 'Circular parent-child team relationship detected: team % is an ancestor of proposed parent %', NEW.id, NEW.parent_team_id;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_check_team_hierarchy_cycle ON teams;
CREATE TRIGGER trg_check_team_hierarchy_cycle
    BEFORE INSERT OR UPDATE OF parent_team_id ON teams
    FOR EACH ROW EXECUTE FUNCTION check_team_hierarchy_cycle();

-- 6. Team Members join table (enforcing 1 primary team per employee per business)
CREATE TABLE IF NOT EXISTS team_members (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id uuid NOT NULL,
    team_id     uuid NOT NULL,
    user_id     uuid NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_team_members_user_biz UNIQUE (business_id, user_id),
    CONSTRAINT fk_team_members_team FOREIGN KEY (business_id, team_id)
        REFERENCES teams(business_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_team_members_membership FOREIGN KEY (user_id, business_id)
        REFERENCES memberships(user_id, business_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_team_members_team ON team_members(team_id);
CREATE INDEX IF NOT EXISTS idx_team_members_user ON team_members(user_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION check_team_member_not_suspended() RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM memberships 
         WHERE user_id = NEW.user_id 
           AND business_id = NEW.business_id 
           AND is_suspended = true
    ) THEN
        RAISE EXCEPTION 'Cannot assign suspended member % to team %', NEW.user_id, NEW.team_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_check_team_member_not_suspended ON team_members;
CREATE TRIGGER trg_check_team_member_not_suspended
    BEFORE INSERT OR UPDATE OF user_id, business_id, team_id ON team_members
    FOR EACH ROW EXECUTE FUNCTION check_team_member_not_suspended();

-- 7. Granular Delegated Permissions table
CREATE TABLE IF NOT EXISTS user_permissions (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id         uuid NOT NULL,
    user_id             uuid NOT NULL,
    permission          text NOT NULL CHECK (permission IN (
        'can_view_team',
        'can_manage_employees',
        'can_create_team_leads',
        'can_assign_employees',
        'can_manage_screenshot_policy',
        'can_view_screenshots',
        'can_view_reports',
        'can_manage_team',
        'can_view_audit_logs'
    )),
    delegable           boolean NOT NULL DEFAULT false,
    granted_by_user_id  uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_user_permissions UNIQUE (business_id, user_id, permission),
    CONSTRAINT fk_user_perms_membership FOREIGN KEY (user_id, business_id)
        REFERENCES memberships(user_id, business_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_user_perms_lookup ON user_permissions(business_id, user_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION check_user_permission_granter() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.granted_by_user_id IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1 FROM memberships 
             WHERE user_id = NEW.granted_by_user_id 
               AND business_id = NEW.business_id
        ) THEN
            RAISE EXCEPTION 'Granter % is not a member of business %', NEW.granted_by_user_id, NEW.business_id;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_check_user_permission_granter ON user_permissions;
CREATE TRIGGER trg_check_user_permission_granter
    BEFORE INSERT OR UPDATE OF business_id, granted_by_user_id ON user_permissions
    FOR EACH ROW EXECUTE FUNCTION check_user_permission_granter();

-- 8. Cleanup trigger on memberships: clears lead_user_id and revokes grants on membership deletion
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION cleanup_membership_on_delete() RETURNS TRIGGER AS $$
BEGIN
    -- Clear leadership on any teams led by this departing member in this business
    UPDATE teams 
       SET lead_user_id = NULL, updated_at = now() 
     WHERE business_id = OLD.business_id AND lead_user_id = OLD.user_id;

    -- Cascade-revoke any permissions that were granted by this member in this business
    WITH RECURSIVE delegation_tree AS (
        SELECT id, user_id, permission
          FROM user_permissions
         WHERE business_id = OLD.business_id AND granted_by_user_id = OLD.user_id
        UNION ALL
        SELECT c.id, c.user_id, c.permission
          FROM user_permissions c
          JOIN delegation_tree p 
            ON c.granted_by_user_id = p.user_id 
           AND c.permission = p.permission
         WHERE c.business_id = OLD.business_id
    )
    DELETE FROM user_permissions WHERE id IN (SELECT id FROM delegation_tree);

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_cleanup_membership_on_delete ON memberships;
CREATE TRIGGER trg_cleanup_membership_on_delete
    BEFORE DELETE ON memberships
    FOR EACH ROW EXECUTE FUNCTION cleanup_membership_on_delete();

-- 9. Organizational Audit Logs (survives member lifecycle removals)
CREATE TABLE IF NOT EXISTS organization_audit_logs (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id         uuid NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
    actor_user_id       uuid REFERENCES users(id) ON DELETE SET NULL,
    actor_email         text,
    actor_display_name  text,
    target_user_id      uuid REFERENCES users(id) ON DELETE SET NULL,
    target_email        text,
    target_display_name text,
    target_team_id      uuid REFERENCES teams(id) ON DELETE SET NULL,
    target_team_name    text,
    action              text NOT NULL,
    details             jsonb NOT NULL DEFAULT '{}',
    created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_org_audit_biz_ts ON organization_audit_logs(business_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_org_audit_actor ON organization_audit_logs(actor_user_id);
CREATE INDEX IF NOT EXISTS idx_org_audit_target ON organization_audit_logs(target_user_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION prevent_audit_log_modification() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'organization_audit_logs is append-only and cannot be updated or deleted';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_prevent_audit_log_modification ON organization_audit_logs;
CREATE TRIGGER trg_prevent_audit_log_modification
    BEFORE UPDATE OR DELETE ON organization_audit_logs
    FOR EACH ROW EXECUTE FUNCTION prevent_audit_log_modification();

-- +goose Down
DROP TRIGGER IF EXISTS trg_prevent_audit_log_modification ON organization_audit_logs;
DROP FUNCTION IF EXISTS prevent_audit_log_modification();
DROP TABLE IF EXISTS organization_audit_logs;

DROP TRIGGER IF EXISTS trg_cleanup_membership_on_delete ON memberships;
DROP FUNCTION IF EXISTS cleanup_membership_on_delete();

DROP TRIGGER IF EXISTS trg_check_user_permission_granter ON user_permissions;
DROP FUNCTION IF EXISTS check_user_permission_granter();
DROP TABLE IF EXISTS user_permissions;

DROP TRIGGER IF EXISTS trg_check_team_member_not_suspended ON team_members;
DROP FUNCTION IF EXISTS check_team_member_not_suspended();
DROP TABLE IF EXISTS team_members;

DROP TRIGGER IF EXISTS trg_check_team_hierarchy_cycle ON teams;
DROP FUNCTION IF EXISTS check_team_hierarchy_cycle();

DROP TRIGGER IF EXISTS trg_check_team_business_integrity ON teams;
DROP FUNCTION IF EXISTS check_team_business_integrity();

DROP TABLE IF EXISTS teams;

ALTER TABLE memberships DROP COLUMN IF EXISTS is_suspended;

UPDATE memberships SET role = 'employee' WHERE role = 'team_lead';

ALTER TABLE memberships DROP CONSTRAINT IF EXISTS memberships_role_check;
ALTER TABLE memberships ADD CONSTRAINT memberships_role_check 
    CHECK (role IN ('owner', 'employee'));
