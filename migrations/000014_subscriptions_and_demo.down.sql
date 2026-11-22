ALTER TABLE contractors
    DROP CONSTRAINT contractors_organization_name_key,
    ADD CONSTRAINT contractors_name_key UNIQUE (name);

ALTER TABLE organizations DROP COLUMN IF EXISTS paid_until;
