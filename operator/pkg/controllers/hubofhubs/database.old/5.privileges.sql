-- grant privileges to a readonly user
DO $$ BEGIN
    IF EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '$1') THEN
        GRANT USAGE ON SCHEMA spec TO "$1";

        GRANT SELECT ON ALL TABLES IN SCHEMA spec TO "$1";
   END IF;
END $$;
