#!/usr/bin/env python3
"""Why did the Claude review end without a verdict? Reads claude-code-action's execution file.

Usage: review_failure_cause.py <execution_file>  → prints one fixed line, exit 0.
Upstream copy and tests: weside-ai/claude-code-plugin scripts/claude_review_gate.py (classify).
Stdlib only, Python 3.9+.
"""

from __future__ import annotations

import json
import re
import sys

AUTH_RE = re.compile(r"\b401\b|invalid (api key|bearer)|failed to authenticate|oauth token", re.I)
QUOTA_RE = re.compile(r"\b429\b|usage limit|rate.?limit|quota|credit balance", re.I)
REASONS = {
    "auth": "token invalid: regenerate with `claude setup-token` and update CLAUDE_CODE_OAUTH_TOKEN",
    "quota": "subscription quota/rate limit: re-run later, nothing to fix",
    "refused": (
        "refused before the model ran, cause not stated (quota or token): re-run after the "
        "quota resets; if it still fails, regenerate the token"
    ),
    "other": "action error: see the run log",
}


def classify(messages: object) -> str:
    """`auth`, `quota`, `refused` or `other` for a run that produced no verdict.

    Only an explicit signal names auth or quota: the assistant message's `error`, the result's
    `api_error_status`, or the error text. A bad token and an exhausted quota share the shape
    "is_error, $0, empty modelUsage, a few seconds" (bad token probed 2026-10-03: `error:
    authentication_failed`, `api_error_status: 401`, 2.5 s; quota run 37133981476: 540 ms), so
    that shape alone is `refused`. Text is read only from the result and from assistant messages
    that carry `error`, never from the review's own prose.
    """
    if not isinstance(messages, list):
        return "other"
    dicts = [m for m in messages if isinstance(m, dict)]
    flagged = [m for m in dicts if m.get("type") == "assistant" and m.get("error")]
    results = [m for m in dicts if m.get("type") == "result"]
    errors = {str(m["error"]) for m in flagged}
    statuses = {m.get("api_error_status") for m in results}
    text = " ".join(
        [str(m.get("result", "")) for m in results]
        + [json.dumps(m.get("message", "")) for m in flagged]
    )
    if "authentication_failed" in errors or statuses & {401, 403} or AUTH_RE.search(text):
        return "auth"
    if errors & {"rate_limit", "billing_error"} or 429 in statuses or QUOTA_RE.search(text):
        return "quota"
    result = results[-1] if results else None
    if (
        result
        and result.get("is_error")
        and not result.get("total_cost_usd")
        and not result.get("modelUsage")
    ):
        return "refused"
    return "other"


def classify_file(path: str) -> tuple[int, str]:
    try:
        with open(path, encoding="utf-8") as fh:
            messages = json.load(fh)
    except (OSError, ValueError):
        messages = None
    kind = classify(messages)
    return 1, f"Claude review produced no verdict ({kind}): {REASONS[kind]}."



if __name__ == "__main__":
    print(classify_file(sys.argv[1] if len(sys.argv) > 1 else "")[1])
