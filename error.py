import json
import sys
from pathlib import Path


def get_code_value(code):
    # In your file, "code" can be a string or an object like {"value": "...", "target": {...}}
    if isinstance(code, dict):
        return code.get("value") or ""
    return code or ""


def main():
    if len(sys.argv) < 2:
        print("Usage: python diag_to_bullets.py paste.txt > errors.md", file=sys.stderr)
        sys.exit(2)

    path = Path(sys.argv[1])
    data = json.loads(path.read_text(encoding="utf-8", errors="replace"))

    # Stable sort: file, then line/col, then message
    def key(d):
        return (
            d.get("resource", ""),
            d.get("startLineNumber", 0),
            d.get("startColumn", 0),
            d.get("message", ""),
        )

    data_sorted = sorted(data, key=key)

    for i, d in enumerate(data_sorted, 1):
        resource = d.get("resource", "?")
        line = d.get("startLineNumber", "?")
        col = d.get("startColumn", "?")
        end_line = d.get("endLineNumber", line)
        end_col = d.get("endColumn", col)

        msg = (d.get("message") or "").strip()
        src = d.get("source", "")
        sev = d.get("severity", "")
        code = get_code_value(d.get("code"))

        # Bullet format: keep it compact for AI input
        loc = f"{line}:{col}"
        if end_line != line or end_col != col:
            loc += f"-{end_line}:{end_col}"

        extra = []
        if code:
            extra.append(f"code={code}")
        if src:
            extra.append(f"src={src}")
        if sev != "":
            extra.append(f"sev={sev}")

        extra_txt = f" ({', '.join(extra)})" if extra else ""
        print(f"- {i}. {resource}:{loc} — {msg}{extra_txt}")


if __name__ == "__main__":
    main()
