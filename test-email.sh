#!/bin/bash
set -euo pipefail

BASE_URL="http://localhost:8080"
API_URL="${BASE_URL}/api/v1"

echo "Testing Email Campaign System"
echo "============================="
echo

echo "0) Health check..."
curl -sS -i "${BASE_URL}/health"
echo -e "\n"

echo "1) Creating Email Account..."
ACCOUNT_RESPONSE=$(curl -sS -X POST "${API_URL}/accounts" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "your-email@gmail.com",
    "provider": "gmail",
    "sendername": "Test Sender",
    "dailylimit": 10,
    "rotationlimit": 5,
    "smtphost": "smtp.gmail.com",
    "smtpport": 587,
    "usetls": true,
    "usessl": false
  }')

ACCOUNT_ID=$(echo "$ACCOUNT_RESPONSE" | jq -r '.id // empty')
echo "Account response: $ACCOUNT_RESPONSE"
echo "Account ID: ${ACCOUNT_ID:-"(not returned)"}"
echo

echo "2) Creating Email Template..."
TEMPLATE_RESPONSE=$(curl -sS -X POST "${API_URL}/templates" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Test Campaign Email",
    "subject": "Test Email from Campaign System",
    "htmlcontent": "<h1>Hello!</h1><p>This is a test email.</p>"
  }')

TEMPLATE_ID=$(echo "$TEMPLATE_RESPONSE" | jq -r '.id // empty')
echo "Template response: $TEMPLATE_RESPONSE"
echo "Template ID: ${TEMPLATE_ID:-"(not returned)"}"
echo

# NOTE: Campaign creation requires many fields in CreateCampaignRequest.
# Also: accountids/templateids are arrays in JSON, but your struct currently declares them as uint slices incorrectly.
# We'll still send arrays, because that is what the validation tag "min=1" implies. [file:1]
echo "3) Creating Campaign..."
NOW="$(date +%Y%m%d-%H%M%S)"

CAMPAIGN_RESPONSE=$(curl -sS -X POST "${API_URL}/campaigns" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Test Campaign - '"$NOW"'",
    "description": "API smoke test",
    "templatedir": "./storage/templates",
    "recipientfile": "./storage/recipients/test.csv",
    "subjectlines": ["Subject A"],
    "sendernames": ["Test Sender"],
    "workercount": 1,
    "ratelimit": 1,
    "dailylimit": 10,
    "rotationlimit": 5,
    "accountids": [1],
    "templateids": [1],
    "proxyenabled": false,
    "attachmentenabled": false,
    "trackingenabled": false,
    "config": {}
  }')

CAMPAIGN_ID=$(echo "$CAMPAIGN_RESPONSE" | jq -r '.id // empty')
echo "Campaign response: $CAMPAIGN_RESPONSE"
echo "Campaign ID: ${CAMPAIGN_ID:-"(not returned)"}"
echo

echo "4) Starting Campaign (if created)..."
if [[ -n "${CAMPAIGN_ID:-}" ]]; then
  curl -sS -X POST "${API_URL}/campaigns/${CAMPAIGN_ID}/start" \
    -H "Content-Type: application/json" | jq '.'
else
  echo "Skipping start: campaign id not returned."
fi
echo

echo "5) Checking Campaign Stats (mock/real depending on implementation)..."
if [[ -n "${CAMPAIGN_ID:-}" ]]; then
  curl -sS -X GET "${API_URL}/campaigns/${CAMPAIGN_ID}/stats" | jq '.'
else
  echo "Skipping stats: campaign id not returned."
fi
echo

echo "Done."
