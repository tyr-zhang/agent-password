#!/usr/bin/env python3
"""Generic login form with selectors supplied on the command line.

Usage:
  agent-pwd fill apply --handle <h> -- \
    python3 generic_form.py <url> <user_selector> <pass_selector> [submit_selector]

The username is a plain argument; use a second fill handle if it is sensitive.
"""
import os
import sys

from playwright.sync_api import sync_playwright


def main() -> int:
    if len(sys.argv) < 5:
        print("usage: generic_form.py <url> <username> <user_selector> <pass_selector> [submit_selector]", file=sys.stderr)
        return 2
    url, username = sys.argv[1], sys.argv[2]
    user_sel, pass_sel = sys.argv[3], sys.argv[4]
    submit_sel = sys.argv[5] if len(sys.argv) > 5 else 'button[type="submit"]'
    password = os.environ.get("AGENT_PWD_FILL")
    if not password:
        print("run through `agent-pwd fill apply --handle <h> -- ...`", file=sys.stderr)
        return 2

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=False)
        try:
            page = browser.new_page()
            page.goto(url, wait_until="domcontentloaded")
            page.fill(user_sel, username)
            page.fill(pass_sel, password)
            page.click(submit_sel)
            page.wait_for_load_state("networkidle")
            print("title:", page.title())
            print("url:", page.url)
        finally:
            browser.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
