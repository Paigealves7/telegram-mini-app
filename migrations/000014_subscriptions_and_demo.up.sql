ALTER TABLE organizations ADD COLUMN IF NOT EXISTS paid_until TIMESTAMPTZ;

-- Одинаковые названия контрагентов допустимы в разных лесопилках.
ALTER TABLE contractors
    DROP CONSTRAINT contractors_name_key,
    ADD CONSTRAINT contractors_organization_name_key UNIQUE (organization_id, name);
