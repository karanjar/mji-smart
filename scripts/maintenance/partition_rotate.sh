#!/bin/bash
# scripts/maintenance/partition_rotate.sh
# Rotate partitions for reports table

set -e

DB_NAME="mjismart"
DB_USER="mjismart"
PARTITION_KEEP_MONTHS=24  # Keep 2 years of data

# Get current date
CURRENT_DATE=$(date +%Y-%m-%d)
ARCHIVE_DATE=$(date -d "$CURRENT_DATE - $PARTITION_KEEP_MONTHS months" +%Y-%m-%d)

echo "Rotating partitions older than $ARCHIVE_DATE"

# Create archive schema if not exists
psql -d $DB_NAME -U $DB_USER <<EOF
CREATE SCHEMA IF NOT EXISTS archive;
EOF

# Find and archive old partitions
psql -d $DB_NAME -U $DB_USER -t -A -c "
SELECT tablename 
FROM pg_tables 
WHERE schemaname = 'public' 
AND tablename LIKE 'reports_%' 
AND tablename < 'reports_' || to_char('$ARCHIVE_DATE'::date, 'YYYY_MM')
" | while read partition; do
    echo "Archiving $partition"
    
    # Move partition to archive schema
    psql -d $DB_NAME -U $DB_USER <<EOF
    ALTER TABLE $partition SET SCHEMA archive;
    -- Compress archive table
    ALTER TABLE archive.$partition SET (autovacuum_enabled = false);
    VACUUM FREEZE archive.$partition;
EOF
done

echo "Partition rotation complete"