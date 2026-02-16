#!/usr/bin/env python3
"""Synchronize README files from canonical/sdks into docs/sdks/."""

import argparse
import base64
import json
import logging
import os
import re
import sys
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen

log = logging.getLogger(__name__)

GITHUB_API = "https://api.github.com"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Sync SDK README files from a source repository.",
    )
    parser.add_argument(
        "--source-repo",
        default="canonical/sdks",
        help='GitHub repository in "org/repo" format (default: canonical/sdks)',
    )
    parser.add_argument(
        "--source-branch",
        default="main",
        help="Branch name in the source repository (default: main)",
    )
    parser.add_argument(
        "--source-pattern",
        default=".*",
        help="Regex pattern to filter top-level directories (default: match all)",
    )
    parser.add_argument(
        "--target-dir",
        default="docs/sdks",
        help="Local directory for synced READMEs (default: docs/sdks)",
    )
    parser.add_argument(
        "--timeout",
        type=int,
        default=None,
        help="Timeout in seconds for GitHub API operations (default: no timeout)",
    )
    return parser.parse_args()


def _api_request(url: str, timeout: int | None) -> dict | list:
    """Perform an authenticated GET against the GitHub API."""
    headers = {"Accept": "application/vnd.github.v3+json"}
    token = os.environ.get("GITHUB_TOKEN", "")
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = Request(url, headers=headers)
    kwargs: dict = {}
    if timeout is not None:
        kwargs["timeout"] = timeout
    with urlopen(req, **kwargs) as resp:
        return json.loads(resp.read())


def list_top_level_dirs(source_repo: str, branch: str, timeout: int | None) -> list[str]:
    """Return sorted names of top-level directories in the repository."""
    url = f"{GITHUB_API}/repos/{source_repo}/contents/?ref={branch}"
    log.info("Listing top-level contents of %s", source_repo)
    entries = _api_request(url, timeout)
    return sorted(
        e["name"]
        for e in entries
        if e["type"] == "dir" and not e["name"].startswith(".")
    )


def fetch_readme(
    source_repo: str, dir_name: str, branch: str, timeout: int | None
) -> str | None:
    """Fetch README.md content for a directory, or None if it doesn't exist."""
    url = f"{GITHUB_API}/repos/{source_repo}/contents/{dir_name}/README.md?ref={branch}"
    try:
        data = _api_request(url, timeout)
    except HTTPError as exc:
        if exc.code == 404:
            return None
        raise
    content = data.get("content", "")
    if data.get("encoding") == "base64":
        return base64.b64decode(content).decode()
    return content


def main() -> int:
    logging.basicConfig(
        level=logging.INFO,
        format="%(levelname)s: %(message)s",
    )

    args = parse_args()

    log.info("Source repository: %s", args.source_repo)
    log.info("Source branch: %s", args.source_branch)
    log.info("Source pattern: %s", args.source_pattern)
    log.info("Target directory: %s", args.target_dir)

    target_dir = Path(args.target_dir)
    target_dir.mkdir(parents=True, exist_ok=True)
    regex = re.compile(args.source_pattern)

    try:
        dirs = list_top_level_dirs(args.source_repo, args.source_branch, args.timeout)

        copied = 0
        skipped = 0

        for name in dirs:
            if not regex.search(name):
                log.info("Skipping %s (does not match pattern)", name)
                skipped += 1
                continue

            content = fetch_readme(args.source_repo, name, args.source_branch, args.timeout)
            if content is None:
                log.info("Skipping %s (no README.md)", name)
                skipped += 1
                continue

            dest = target_dir / f"{name}.md"
            dest.write_text(content)
            log.info("Copied %s/README.md -> %s", name, dest)
            copied += 1

        log.info(
            "Summary: %d READMEs synchronized, %d directories skipped",
            copied,
            skipped,
        )
        return 0
    except Exception:
        log.exception("Fatal error during sync")
        return 1


if __name__ == "__main__":
    sys.exit(main())
