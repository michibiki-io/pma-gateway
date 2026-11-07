#!/usr/bin/env python3
"""Resolve trusted push metadata before invoking the shared release helper."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
import urllib.request

ATTEMPTS = 5
RETRY_SECONDS = 2


def trusted_push(event, env):
    """Return immutable repository/default-branch/SHA identity, or skip."""
    if env.get("GITHUB_EVENT_NAME") != "push":
        return None
    repo = event["repository"]["full_name"]
    branch = event["repository"]["default_branch"]
    sha = event["after"]
    if (not isinstance(repo, str) or not re.fullmatch(r"[\w.-]+/[\w.-]+", repo)
            or not isinstance(branch, str) or not branch
            or not isinstance(sha, str) or not re.fullmatch(r"[0-9a-f]{40}", sha)):
        raise ValueError("invalid push identity")
    if repo != env["GITHUB_REPOSITORY"] or sha != env["GITHUB_SHA"] or event["ref"] != env["GITHUB_REF"]:
        raise ValueError("push identity does not match runner context")
    if event.get("deleted") or event["ref"] != f"refs/heads/{branch}":
        return None
    return {"repository": repo, "default_branch": branch, "sha": sha}


def validate_pr(pr):
    # Missing or malformed API fields are errors, never a no-release decision.
    if (type(pr["number"]) is not int or pr["number"] <= 0
            or pr["state"] not in ("open", "closed")
            or not isinstance(pr["base"]["ref"], str)
            or not isinstance(pr["base"]["repo"]["full_name"], str)
            or not isinstance(pr["head"]["ref"], str)
            or not isinstance(pr["title"], str)
            or not (pr["body"] is None or isinstance(pr["body"], str))
            or not (pr["merged_at"] is None or isinstance(pr["merged_at"], str))
            or not (pr["merge_commit_sha"] is None or isinstance(pr["merge_commit_sha"], str))):
        raise ValueError("malformed PR metadata")
    if "\0" in pr["title"] or "\0" in (pr["body"] or ""):
        raise ValueError("PR metadata cannot contain NUL")


def eligible(pr, identity):
    validate_pr(pr)
    return (pr["state"] == "closed" and bool(pr["merged_at"])
            and pr["merge_commit_sha"] == identity["sha"]
            and pr["base"]["repo"]["full_name"] == identity["repository"]
            and pr["base"]["ref"] == identity["default_branch"]
            and pr["head"]["ref"].startswith("feature/"))


def associated_prs(identity, env):
    result = []
    page = 1
    while True:
        url = (f"{env.get('GITHUB_API_URL', 'https://api.github.com')}/repos/"
               f"{identity['repository']}/commits/{identity['sha']}/pulls?per_page=100&page={page}")
        request = urllib.request.Request(url, headers={
            "Accept": "application/vnd.github+json",
            "Authorization": f"Bearer {env['GITHUB_TOKEN']}",
            "X-GitHub-Api-Version": "2022-11-28",
        })
        with urllib.request.urlopen(request, timeout=30) as response:
            items = json.load(response)
        if not isinstance(items, list):
            raise ValueError("expected a PR array from GitHub")
        result.extend(items)
        if len(items) < 100:
            return result
        page += 1


def resolve(identity, env):
    for attempt in range(ATTEMPTS):
        candidates = [pr for pr in associated_prs(identity, env) if eligible(pr, identity)]
        if len(candidates) > 1:
            raise ValueError("multiple merged feature PRs match the pushed SHA")
        if candidates:
            return {**identity, "pr": candidates[0]}
        if attempt + 1 < ATTEMPTS:
            time.sleep(RETRY_SECONDS)
    return None


def compute_version(snapshot, identity, env):
    if (identity is None or any(snapshot[k] != identity[k] for k in identity)
            or not eligible(snapshot["pr"], identity)):
        raise ValueError("release metadata does not match the trusted push")
    pr = snapshot["pr"]
    child_env = dict(env, MERGE_COMMIT_SHA=identity["sha"], PR_NUMBER=str(pr["number"]),
                     PR_TITLE=pr["title"], PR_BODY=pr["body"] or "")
    helper = Path(__file__).parent / "release/compute_release.py"
    return subprocess.run([sys.executable, str(helper)], env=child_env, check=False).returncode


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("resolve", "version"))
    parser.add_argument("snapshot", type=Path)
    args = parser.parse_args()
    try:
        env = os.environ.copy()
        event = json.loads(Path(env["GITHUB_EVENT_PATH"]).read_text())
        identity = trusted_push(event, env)
        if args.command == "version":
            return compute_version(json.loads(args.snapshot.read_text()), identity, env)
        args.snapshot.unlink(missing_ok=True)
        snapshot = resolve(identity, env) if identity else None
        if snapshot:
            args.snapshot.write_text(json.dumps(snapshot, ensure_ascii=False), encoding="utf-8")
        else:
            print("::notice::No matching merged feature PR for this push; release skipped. "
                  "After API indexing, rerun this same push if a release was expected.")
        # Arbitrary PR text stays in JSON and a subprocess environment, never outputs/shell.
        with open(env["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
            output.write(f"eligible={'true' if snapshot else 'false'}\n")
            output.write(f"pr_number={snapshot['pr']['number'] if snapshot else ''}\n")
        return 0
    except (OSError, ValueError, KeyError, TypeError) as error:
        print(f"Release metadata resolution failed: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
