"""A standard-library-only example of the direct executor JSON protocol."""

import json
from pathlib import Path
import sys
from urllib.error import HTTPError, URLError
from urllib.request import urlopen


def main():
    request = json.load(sys.stdin)
    if request.get("protocolVersion") != 1 or request.get("mode") != "direct":
        raise ValueError("expected direct protocol version 1")
    project = Path(__file__).resolve().parent
    cases = json.loads((project / "cases.json").read_text())
    if request["operation"] == "discover":
        print(json.dumps({
            "protocolVersion": 1,
            "sourceFiles": ["adapter.py", "cases.json"],
            "cases": [{"id": case["id"], "title": case["title"],
                       "kind": "case", "sourcePath": "cases.json"} for case in cases],
        }))
        return 0
    if request["operation"] != "run":
        raise ValueError("unknown operation")
    case = next(case for case in cases if case["id"] == request["caseId"])
    endpoint = request["targetUrl"].rstrip("/") + case["path"]
    status, body, error = 0, "", ""
    try:
        with urlopen(endpoint, timeout=10) as response:
            status = response.status
            body = response.read(65536).decode("utf-8", errors="replace")
    except HTTPError as exc:
        status = exc.code
        body = exc.read(65536).decode("utf-8", errors="replace")
    except URLError as exc:
        error = str(exc.reason)
    passed = status == case["expectedStatus"]
    print(json.dumps({
        "protocolVersion": 1, "caseId": case["id"], "executed": True,
        "status": "passed" if passed else "failed",
        "checks": [{"name": "expected-status", "passed": passed,
                    "message": error or f"expected {case['expectedStatus']}, got {status}"}],
        "data": {"url": endpoint, "statusCode": status, "body": body},
    }))
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
