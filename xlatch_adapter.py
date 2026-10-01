#!/usr/bin/python3
"""xlatch adapter: one shared HTTP(S) URL -> scrpr markdown -> typed text."""

import json
import subprocess
import sys
from urllib.parse import urlsplit

SCRPR = "/Users/tommyfalkowski/go/bin/scrpr"


def main() -> None:
    request = json.load(sys.stdin)
    value = request.get("text")
    if not isinstance(value, str):
        raise ValueError("text URL is required")
    url = value.strip()
    parsed = urlsplit(url)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise ValueError("only an absolute HTTP(S) URL is accepted")
    result = subprocess.run(
        [SCRPR, "--format", "markdown", "--quiet", url],
        check=True,
        capture_output=True,
        text=True,
    )
    content = result.stdout.strip()
    if not content:
        raise RuntimeError("scrpr returned no content")
    json.dump({"text": content}, sys.stdout)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
