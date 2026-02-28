-- ╔══════════════════════════════════════════════════════╗
-- ║  CLI FIX — run this ONCE on the current broken DB    ║
-- ║  to finish what 000013 got wrong for proxies         ║
-- ╚══════════════════════════════════════════════════════╝

-- 000013 used wrong target names for 3 proxy columns:
--   last_failure_at  → needs to be  last_error_at
--   last_success_at  → needs to be  last_healthy_at
--   current_connections → needs to be  current_conns

DO $$
BEGIN
    -- Fix last_error_at
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name='proxies' AND column_name='last_failure_at')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns
                       WHERE table_name='proxies' AND column_name='last_error_at')
    THEN
        ALTER TABLE proxies RENAME COLUMN last_failure_at TO last_error_at;
        RAISE NOTICE 'Renamed last_failure_at → last_error_at';
    END IF;

    -- Fix last_healthy_at
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name='proxies' AND column_name='last_success_at')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns
                       WHERE table_name='proxies' AND column_name='last_healthy_at')
    THEN
        ALTER TABLE proxies RENAME COLUMN last_success_at TO last_healthy_at;
        RAISE NOTICE 'Renamed last_success_at → last_healthy_at';
    END IF;

    -- Fix current_conns
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name='proxies' AND column_name='current_connections')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns
                       WHERE table_name='proxies' AND column_name='current_conns')
    THEN
        ALTER TABLE proxies RENAME COLUMN current_connections TO current_conns;
        RAISE NOTICE 'Renamed current_connections → current_conns';
    END IF;
END $$;
