#!/usr/bin/env python3
"""Scrape nagios.com's Nagios XI changelog for the newest release.

Nagios XI has no GitHub repo, releases API, or RSS/Atom feed - this is the
only automated way to find out when a new version ships (see #127). The page
has no stability guarantee, so a parse failure here is a hard error (exit 1),
not a silent no-op.
"""

import os
import re
import sys
import urllib.request

CHANGELOG_URL = "https://www.nagios.com/changelog/nagios-xi/"
# e.g. "2026R1.6.1" -> prefix "2026R1". Patch/hotfix bumps after the second
# dot are not a "new version" for our purposes, only changes to this prefix.
VERSION_RE = re.compile(r"([0-9]{4}R[0-9]+)(?:\.[0-9]+){0,2}")


def fetch(url: str) -> str:
    req = urllib.request.Request(
        url,
        headers={
            "User-Agent": (
                "Mozilla/5.0 (compatible; nagios-xi-version-check/1.0; "
                "+https://github.com/dunkin0486/terraform-provider-nagios)"
            )
        },
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        return resp.read().decode("utf-8", errors="replace")


def extract_latest(html: str) -> dict:
    idx = html.find('class="changelog-card"')
    if idx == -1:
        raise ValueError("no element with class=\"changelog-card\" found on the page")
    # Cards are sorted newest-first by default; the first one is the latest
    # release. Limit the search window so a malformed page can't make these
    # regexes scan the entire multi-thousand-line document.
    window = html[idx : idx + 4000]

    version_match = VERSION_RE.search(window)
    if not version_match:
        raise ValueError("no <year>R<release>[.minor[.patch]] version string found in the first changelog card")
    version = version_match.group(0)
    prefix = version_match.group(1)

    date_match = re.search(r"-\s*([A-Za-z]+ \d{1,2},\s*\d{4})", window)
    date = date_match.group(1).strip() if date_match else "unknown date"

    summary_match = re.search(r'<div class="release-summary">\s*<p>(.*?)</p>', window, re.DOTALL)
    summary = re.sub(r"\s+", " ", summary_match.group(1)).strip() if summary_match else ""

    link_match = re.search(r'<a href="([^"]+)"\s+class="view-details-link">', window)
    detail_url = link_match.group(1) if link_match else CHANGELOG_URL

    return {
        "version": version,
        "prefix": prefix,
        "date": date,
        "summary": summary,
        "detail_url": detail_url,
    }


def write_output(values: dict) -> None:
    github_output = os.environ.get("GITHUB_OUTPUT")
    if not github_output:
        return
    with open(github_output, "a", encoding="utf-8") as f:
        for key in ("version", "prefix", "date", "detail_url"):
            f.write(f"{key}={values[key]}\n")
        # summary may contain characters that break single-line KEY=value
        # output syntax; use the multiline delimiter form.
        f.write("summary<<NAGIOS_XI_SUMMARY_EOF\n")
        f.write(values["summary"] + "\n")
        f.write("NAGIOS_XI_SUMMARY_EOF\n")


def main() -> None:
    try:
        html = fetch(CHANGELOG_URL)
        info = extract_latest(html)
    except Exception as exc:  # noqa: BLE001 - this is the top-level CLI entry point
        print(f"::error::Failed to determine the latest Nagios XI version from {CHANGELOG_URL}: {exc}", file=sys.stderr)
        sys.exit(1)

    write_output(info)
    print(f"Latest Nagios XI version: {info['version']} ({info['date']}), prefix {info['prefix']}")
    print(f"Summary: {info['summary']}")
    print(f"Detail URL: {info['detail_url']}")


if __name__ == "__main__":
    main()
