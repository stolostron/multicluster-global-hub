-- grant privileges to a readonly user
DO $$ BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '$1') THEN
        CREATE ROLE '$1' LOGIN PASSWORD '$2';
    END IF
    IF EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '$1') THEN
        GRANT USAGE ON SCHEMA status TO $1;
        GRANT USAGE ON SCHEMA event TO $1;
        GRANT USAGE ON SCHEMA history TO $1;
        GRANT USAGE ON SCHEMA local_spec TO $1;
        GRANT USAGE ON SCHEMA local_status TO $1;

        GRANT SELECT ON ALL TABLES IN SCHEMA status TO $1;
        GRANT SELECT ON ALL TABLES IN SCHEMA event TO $1;
        GRANT SELECT ON ALL TABLES IN SCHEMA history TO $1;
        GRANT SELECT ON ALL TABLES IN SCHEMA local_spec TO $1;
        GRANT SELECT ON ALL TABLES IN SCHEMA local_status TO $1;
   END IF;
END $$;
