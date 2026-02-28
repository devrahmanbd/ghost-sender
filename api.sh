#!/bin/bash

BASE="http://localhost:8080/api/v1"

echo "🧪 Testing Email Campaign API"
echo ""

# Health
echo "1️⃣  Health Check:"
curl -s "$BASE/health" | jq .
echo ""

# Create account
echo "2️⃣  Creating Account:"
ACCOUNT=$(curl -s -X POST "$BASE/accounts" \
  -H "Content-Type: application/json" \
  -d '{"email":"test@gmail.com","name":"Test Account","provider":"gmail"}')
echo "$ACCOUNT" | jq .
ACC_ID=$(echo "$ACCOUNT" | jq -r '.id')
echo ""

# List accounts
echo "3️⃣  Listing Accounts:"
curl -s "$BASE/accounts" | jq .
echo ""

# Create template
echo "4️⃣  Creating Template:"
TEMPLATE=$(curl -s -X POST "$BASE/templates" \
  -H "Content-Type: application/json" \
  -d '{"name":"Test Template","subject":"Hello {{NAME}}","html_content":"<h1>Hi</h1>"}')
echo "$TEMPLATE" | jq .
TPL_ID=$(echo "$TEMPLATE" | jq -r '.id')
echo ""

# Create campaign
echo "5️⃣  Creating Campaign:"
CAMPAIGN=$(curl -s -X POST "$BASE/campaigns" \
  -H "Content-Type: application/json" \
  -d '{"name":"Test Campaign"}')
echo "$CAMPAIGN" | jq .
CMP_ID=$(echo "$CAMPAIGN" | jq -r '.id')
echo ""

# Start campaign
echo "6️⃣  Starting Campaign:"
curl -s -X POST "$BASE/campaigns/$CMP_ID/start" | jq .
echo ""

# Get campaign
echo "7️⃣  Campaign Status:"
curl -s "$BASE/campaigns/$CMP_ID" | jq .
echo ""

echo "✅ All tests complete!"
