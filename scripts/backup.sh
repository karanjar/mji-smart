#!/bin/bash

BACKUP_DIR="/backups/mjismart"
DATE=$(date +%Y%m%d_%H%M%S)

# Create backup directory
mkdir -p $BACKUP_DIR

# Backup PostgreSQL
docker exec mjismart-postgres pg_dump -U mjismart mjismart > $BACKUP_DIR/db_$DATE.sql

# Backup Redis
docker exec mjismart-redis redis-cli --rdb $BACKUP_DIR/redis_$DATE.rdb

# Upload to S3 (if configured)
if [ -n "$AWS_S3_BUCKET" ]; then
    aws s3 cp $BACKUP_DIR/db_$DATE.sql s3://$AWS_S3_BUCKET/backups/db_$DATE.sql
    aws s3 cp $BACKUP_DIR/redis_$DATE.rdb s3://$AWS_S3_BUCKET/backups/redis_$DATE.rdb
fi

# Clean old backups (keep last 30 days)
find $BACKUP_DIR -type f -mtime +30 -delete

echo "Backup completed: $DATE"