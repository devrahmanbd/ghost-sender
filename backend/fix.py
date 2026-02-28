#!/usr/bin/env python3
"""
fix_repo_columns.py
Replaces no-underscore SQL column names with snake_case in
repository Go files. Works on macOS and Linux with zero deps.
Run from: backend/
  python3 fix_repo_columns.py
"""

import re
import sys
import os

REPO_DIR = "internal/storage/repository"

# ── Replacements per file ─────────────────────────────────────
# Order matters: longer/more-specific patterns FIRST
# to avoid partial replacements (e.g. oauthrefreshtoken before oauthtoken)

TEMPLATE_RENAMES = [
    ("lastspamcheckat",  "last_spam_check_at"),
    ("lastfailuremsg",   "last_failure_msg"),
    ("lastfailureat",    "last_failure_at"),
    ("lastrenderedat",   "last_rendered_at"),
    ("renderingconfig",  "rendering_config"),
    ("parenttemplateid", "parent_template_id"),
    ("spamcheckedat",    "spam_checked_at"),
    ("rotationweight",   "rotation_weight"),
    ("rotationgroup",    "rotation_group"),
    ("rotationindex",    "rotation_index"),
    ("customheaders",    "custom_headers"),
    ("spamdetails",      "spam_details"),
    ("spamscore",        "spam_score"),
    ("htmlcontent",      "html_content"),
    ("textcontent",      "text_content"),
    ("rendercount",      "render_count"),
    ("usagecount",       "usage_count"),
    ("failurecount",     "failure_count"),
    ("lastusedat",       "last_used_at"),
    ("archivedat",       "archived_at"),
    ("isarchived",       "is_archived"),
    ("isdefault",        "is_default"),
    ("isactive",         "is_active"),
    ("fromname",         "from_name"),
    ("fromemail",        "from_email"),
    ("replyto",          "reply_to"),
    ("createdby",        "created_by"),
    ("updatedby",        "updated_by"),
    ("createdat",        "created_at"),
    ("updatedat",        "updated_at"),
    ("preheader",        "preheader"),   # already correct, no-op
]

ACCOUNT_RENAMES = [
    ("consecutivefailures", "consecutive_failures"),
    ("oauthrefreshtoken",   "oauth_refresh_token"),
    ("encryptedpassword",   "encrypted_password"),
    ("lasthealthcheck",     "last_health_check"),
    ("suspensionreason",    "suspension_reason"),
    ("suspendeduntil",      "suspended_until"),
    ("rotationlimit",       "rotation_limit"),
    ("lastfailureat",       "last_failure_at"),
    ("cooldownuntil",       "cooldown_until"),
    ("smtpusername",        "smtp_username"),
    ("smtpusetls",          "smtp_use_tls"),
    ("smtpusessl",          "smtp_use_ssl"),
    ("suspendedat",         "suspended_at"),
    ("oauthtoken",          "oauth_token"),
    ("oauthexpiry",         "oauth_expiry"),
    ("healthscore",         "health_score"),
    ("issuspended",         "is_suspended"),
    ("rotationsent",        "rotation_sent"),
    ("dailylimit",          "daily_limit"),
    ("spamscore",           "spam_score"),
    ("smtphost",            "smtp_host"),
    ("smtpport",            "smtp_port"),
    ("senttoday",           "sent_today"),
    ("totalsent",           "total_sent"),
    ("totalfailed",         "total_failed"),
    ("successrate",         "success_rate"),
    ("lastusedat",          "last_used_at"),
    ("lastreset",           "last_reset"),
    ("useproxy",            "use_proxy"),
    ("proxyid",             "proxy_id"),
    ("isactive",            "is_active"),
    ("createdat",           "created_at"),
    ("updatedat",           "updated_at"),
]

PROXY_RENAMES = [
    ("bandwidthlimitmb",     "bandwidth_limit_mb"),
    ("anonymityverifiedat",  "anonymity_verified_at"),
    ("locationcheckedat",    "location_checked_at"),
    ("assignedaccounts",     "assigned_accounts"),
    ("consecutivefails",     "consecutive_fails"),
    ("suspensionreason",     "suspension_reason"),
    ("rotationgroup",        "rotation_group"),
    ("rotationweight",       "rotation_weight"),
    ("maxconnections",       "max_connections"),
    ("suspendeduntil",       "suspended_until"),
    ("lasthealthyat",        "last_healthy_at"),
    ("bandwidthmb",          "bandwidth_mb"),
    ("maxlatencyms",         "max_latency_ms"),
    ("minlatencyms",         "min_latency_ms"),
    ("currentconns",         "current_conns"),
    ("lasterrorat",          "last_error_at"),
    ("lastcheckedat",        "last_checked_at"),
    ("lastusedat",           "last_used_at"),
    ("suspendedat",          "suspended_at"),
    ("maxaccounts",          "max_accounts"),
    ("latencyms",            "latency_ms"),
    ("healthscore",          "health_score"),
    ("lasterror",            "last_error"),
    ("isanonymous",          "is_anonymous"),
    ("isauthenticated",      "is_authenticated"),
    ("isactive",             "is_active"),
    ("inuse",                "in_use"),
    ("createdby",            "created_by"),
    ("updatedby",            "updated_by"),
    ("createdat",            "created_at"),
    ("updatedat",            "updated_at"),
    ("deletedat",            "deleted_at"),
]

FILES = {
    "template.go": TEMPLATE_RENAMES,
    "account.go":  ACCOUNT_RENAMES,
    "proxy.go":    PROXY_RENAMES,
}

def patch_file(filepath, renames):
    with open(filepath, "r", encoding="utf-8") as f:
        original = f.read()

    content = original
    counts = {}

    for old, new in renames:
        if old == new:
            continue
        # Match only inside backtick SQL strings to avoid touching Go struct fields/vars
        # Strategy: replace the word only when it appears as a standalone SQL token
        # i.e. surrounded by: space, comma, newline, (, ), backtick, tab, =, <, >
        pattern = r'(?<![a-zA-Z_])' + re.escape(old) + r'(?![a-zA-Z_0-9])'
        new_content, n = re.subn(pattern, new, content)
        if n > 0:
            counts[old] = n
            content = new_content

    if content == original:
        print(f"  ⚪ {os.path.basename(filepath)}: no changes needed")
        return 0

    with open(filepath, "w", encoding="utf-8") as f:
        f.write(content)

    total = sum(counts.values())
    print(f"  ✅ {os.path.basename(filepath)}: {total} replacements")
    for old, n in counts.items():
        print(f"     {old:30s} → {dict(renames)[old]} ({n}x)")
    return total

def verify(filepath, renames):
    with open(filepath, "r", encoding="utf-8") as f:
        content = f.read()
    issues = []
    for old, _ in renames:
        if old == _:
            continue
        pattern = r'(?<![a-zA-Z_])' + re.escape(old) + r'(?![a-zA-Z_0-9])'
        for m in re.finditer(pattern, content):
            line_no = content[:m.start()].count('\n') + 1
            issues.append((os.path.basename(filepath), line_no, old))
    return issues

def main():
    if not os.path.isfile(os.path.join(REPO_DIR, "template.go")):
        print("❌ Run from backend/ root directory")
        sys.exit(1)

    print("=" * 60)
    print("fix_repo_columns.py — snake_case SQL column patcher")
    print("=" * 60)

    total_changes = 0
    for filename, renames in FILES.items():
        filepath = os.path.join(REPO_DIR, filename)
        print(f"\n🔵 Patching {filepath}...")
        total_changes += patch_file(filepath, renames)

    print("\n" + "=" * 60)
    print("🔵 Verifying...")
    all_issues = []
    for filename, renames in FILES.items():
        filepath = os.path.join(REPO_DIR, filename)
        all_issues.extend(verify(filepath, renames))

    if not all_issues:
        print("✅ Verification passed — no remaining no-underscore columns")
    else:
        print("⚠️  Remaining occurrences (may need manual review):")
        for fname, lineno, col in all_issues:
            print(f"   {fname}:{lineno}  →  {col}")

    print(f"\n📊 Total replacements made: {total_changes}")
    print("\n🔵 Run: go build ./cmd/server")

if __name__ == "__main__":
    main()
