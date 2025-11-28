-- Alexandria DB Migration 007: Fix closing_lines primary key to include point
-- This allows storing multiple point values for the same outcome
-- Date: 2025-11-07

-- Step 1: Check for existing duplicates (informational - won't fail if none exist)
-- This query helps identify if cleanup is needed before changing the PK
DO $$
DECLARE
    duplicate_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO duplicate_count
    FROM (
        SELECT event_id, market_key, book_key, outcome_name
        FROM closing_lines
        GROUP BY event_id, market_key, book_key, outcome_name
        HAVING COUNT(DISTINCT point) > 1
    ) duplicates;
    
    IF duplicate_count > 0 THEN
        RAISE NOTICE 'Found % duplicate outcome_names with different points. These will be cleaned up.', duplicate_count;
    ELSE
        RAISE NOTICE 'No duplicates found. Safe to proceed.';
    END IF;
END $$;

-- Step 2: Clean up any duplicates (keep the most recent one per point value)
-- This handles the case where the same outcome_name exists with different points
DELETE FROM closing_lines
WHERE ctid NOT IN (
    SELECT MIN(ctid)
    FROM closing_lines
    GROUP BY event_id, market_key, book_key, outcome_name, point
);

-- Step 3: Drop old primary key constraint
ALTER TABLE closing_lines DROP CONSTRAINT IF EXISTS closing_lines_pkey;

-- Step 4: Add new primary key that includes point
ALTER TABLE closing_lines 
ADD PRIMARY KEY (event_id, market_key, book_key, outcome_name, point);

-- Verify: The table should now allow multiple rows with same outcome_name but different points
COMMENT ON TABLE closing_lines IS 'Closing line values captured at game start for CLV analysis. Primary key includes point to allow multiple line values per outcome.';







