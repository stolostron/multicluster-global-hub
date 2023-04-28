
CREATE SCHEMA IF NOT EXISTS history;

CREATE SCHEMA IF NOT EXISTS local_spec;

CREATE SCHEMA IF NOT EXISTS local_status;

CREATE SCHEMA IF NOT EXISTS spec;

CREATE SCHEMA IF NOT EXISTS status;

CREATE SCHEMA IF NOT EXISTS event;

DO $$ BEGIN
  CREATE TYPE local_status.compliance_type AS ENUM (
    'compliant',
    'non_compliant',
    'unknown'
	);
EXCEPTION
  WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
  CREATE TYPE status.compliance_type AS ENUM (
    'compliant',
    'non_compliant',
    'unknown'
	);
EXCEPTION
  WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
  CREATE TYPE status.error_type AS ENUM (
    'disconnected',
    'none'
  );
EXCEPTION
  WHEN duplicate_object THEN null;
END $$;
