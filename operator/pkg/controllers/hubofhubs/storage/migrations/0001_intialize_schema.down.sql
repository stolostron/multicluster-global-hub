DO $$ BEGIN
  DROP TYPE IF EXISTS local_status.compliance_type;
EXCEPTION
  WHEN undefined_object THEN null;
END $$;

DO $$ BEGIN
  DROP TYPE IF EXISTS status.compliance_type;
EXCEPTION
  WHEN undefined_object THEN null;
END $$;

DO $$ BEGIN
  DROP TYPE IF EXISTS status.error_type;
EXCEPTION
  WHEN undefined_object THEN null;
END $$;

DROP SCHEMA IF EXISTS history CASCADE;
DROP SCHEMA IF EXISTS local_spec CASCADE;
DROP SCHEMA IF EXISTS local_status CASCADE;
DROP SCHEMA IF EXISTS status CASCADE;
DROP SCHEMA IF EXISTS event CASCADE;
DROP SCHEMA IF EXISTS security CASCADE;

DROP EXTENSION IF EXISTS pg_stat_statements;
