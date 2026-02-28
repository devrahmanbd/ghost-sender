#!/bin/bash
# dump_schema.sh — dumps column info for all key tables to .txt files
# Usage: bash dump_schema.sh
# Output: schema_*.txt files in current directory

DB="campaign_system"
USER="rahman"

tables=("templates" "accounts" "proxies" "campaigns" "recipients" "logs")

for tbl in "${tables[@]}"; do
    psql -U "$USER" -d "$DB" -A -F $'\t' -c "
        SELECT ordinal_position, column_name, data_type, character_maximum_length,
               column_default, is_nullable
        FROM information_schema.columns
        WHERE table_name = '$tbl'
        ORDER BY ordinal_position;
    " > "schema_${tbl}.txt" 2>&1
    echo "✅ schema_${tbl}.txt written"
done

# Also dump a combined quick-reference
psql -U "$USER" -d "$DB" -A -F $'\t' -c "
    SELECT table_name, column_name, data_type
    FROM information_schema.columns
    WHERE table_name IN ('templates','accounts','proxies','campaigns','recipients','logs')
    ORDER BY table_name, ordinal_position;
" > schema_all_tables.txt 2>&1
echo "✅ schema_all_tables.txt written"
