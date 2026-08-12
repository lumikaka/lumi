-- The preceding schema already declares these columns as DATETIME. Rolling
-- back the repair version must not restore the incompatible legacy TEXT types.
SELECT 1;
