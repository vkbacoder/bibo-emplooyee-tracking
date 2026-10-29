-- +goose Up
-- Add global screenshot controls to businesses
ALTER TABLE businesses ADD COLUMN screenshots_enabled boolean NOT NULL DEFAULT true;
ALTER TABLE businesses ADD COLUMN continuous_capture_enabled boolean NOT NULL DEFAULT true;
ALTER TABLE businesses ADD COLUMN manual_capture_enabled boolean NOT NULL DEFAULT true;
ALTER TABLE businesses ADD COLUMN policy_version integer NOT NULL DEFAULT 1;
ALTER TABLE businesses ADD COLUMN policy_updated_at timestamptz NOT NULL DEFAULT now();

-- Per-employee screenshot policy overrides.
-- Nullable settings mean "inherit the business default".
CREATE TABLE employee_screenshot_policies (
    id                         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id                uuid NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
    employee_id                uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    screenshots_enabled        boolean,
    continuous_capture_enabled boolean,
    screenshot_interval_s      integer CHECK (screenshot_interval_s IS NULL OR screenshot_interval_s > 0),
    manual_capture_enabled     boolean,
    policy_version             integer NOT NULL DEFAULT 1,
    policy_updated_at          timestamptz NOT NULL DEFAULT now(),
    last_ack_version           integer,
    last_ack_at                timestamptz,
    last_ack_device_id         uuid,
    created_at                 timestamptz NOT NULL DEFAULT now(),
    updated_at                 timestamptz NOT NULL DEFAULT now(),
    UNIQUE (business_id, employee_id)
);
CREATE INDEX idx_emp_policy_biz_emp ON employee_screenshot_policies(business_id, employee_id);
CREATE INDEX idx_emp_policy_emp ON employee_screenshot_policies(employee_id);

-- On-demand / manual screenshot capture requests created by managers.
CREATE TABLE screenshot_requests (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id            uuid NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
    employee_id            uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    requested_by_user_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status                 text NOT NULL DEFAULT 'pending' CHECK (status IN (
        'pending',
        'acknowledged',
        'completed',
        'rejected_disabled',
        'rejected_manual_disabled',
        'rejected_expired',
        'rejected_wrong_employee',
        'rejected_duplicate',
        'rejected_stale_policy',
        'rejected_privacy_exclusion',
        'failed'
    )),
    policy_version         integer NOT NULL,
    created_at             timestamptz NOT NULL DEFAULT now(),
    expires_at             timestamptz NOT NULL,
    acknowledged_at        timestamptz,
    completed_at           timestamptz,
    screenshot_client_uuid uuid,
    error_message          text
);
CREATE INDEX idx_screenshot_req_emp_status ON screenshot_requests(employee_id, status);
CREATE INDEX idx_screenshot_req_biz_created ON screenshot_requests(business_id, created_at DESC);
CREATE INDEX idx_screenshot_req_client_uuid ON screenshot_requests(screenshot_client_uuid) WHERE screenshot_client_uuid IS NOT NULL;

-- Track request_id on screenshot metadata when captured as part of a manual request
ALTER TABLE screenshots ADD COLUMN request_id uuid REFERENCES screenshot_requests(id) ON DELETE SET NULL;
CREATE INDEX idx_screenshots_request_id ON screenshots(request_id) WHERE request_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_screenshots_request_id;
ALTER TABLE screenshots DROP COLUMN IF EXISTS request_id;

DROP INDEX IF EXISTS idx_screenshot_req_client_uuid;
DROP TABLE IF EXISTS screenshot_requests;
DROP TABLE IF EXISTS employee_screenshot_policies;

ALTER TABLE businesses DROP COLUMN IF EXISTS policy_updated_at;
ALTER TABLE businesses DROP COLUMN IF EXISTS policy_version;
ALTER TABLE businesses DROP COLUMN IF EXISTS manual_capture_enabled;
ALTER TABLE businesses DROP COLUMN IF EXISTS continuous_capture_enabled;
ALTER TABLE businesses DROP COLUMN IF EXISTS screenshots_enabled;
