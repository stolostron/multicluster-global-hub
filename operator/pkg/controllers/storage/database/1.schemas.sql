
CREATE SCHEMA IF NOT EXISTS history;

CREATE SCHEMA IF NOT EXISTS local_spec;

CREATE SCHEMA IF NOT EXISTS local_status;

CREATE SCHEMA IF NOT EXISTS status;

CREATE SCHEMA IF NOT EXISTS event;

CREATE SCHEMA IF NOT EXISTS security;

CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

DO $$ BEGIN
  CREATE TYPE local_status.compliance_type AS ENUM (
    'compliant',
    'non_compliant',
    'pending',
    'unknown'
	);
EXCEPTION
  WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
  CREATE TYPE status.compliance_type AS ENUM (
    'compliant',
    'non_compliant',
    'pending',
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
