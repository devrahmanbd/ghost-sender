#!/bin/bash

# Output file
output_file="db.md"

# Clear or create the output file
> "$output_file"

# Find all .go files recursively and process them
find . -type f -name "*.sql" | sort | while read -r file; do
    echo "## $file" >> "$output_file"
    cat "$file" >> "$output_file"
    echo "" >> "$output_file"
    echo "" >> "$output_file"
done

echo "Go files have been collected in $output_file"

