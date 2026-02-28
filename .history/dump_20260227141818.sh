#!/bin/bash
# dump_full_db.sh

DB="campaign_system"
USER="rahman"
OUTPUT="campaign_system_full_dump.sql"

pg_dump -U "$USER" -d "$DB" -F p > "$OUTPUT"

echo "✅ Full database dump written to $OUTPUT"