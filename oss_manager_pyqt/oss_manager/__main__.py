from __future__ import annotations

import argparse
from pathlib import Path

try:
    from .app import run_app
except ImportError:
    # Fallback for script-style execution where package context is absent.
    from oss_manager.app import run_app


def main() -> int:
    parser = argparse.ArgumentParser(description="Standalone PyQt OSS manager")
    parser.add_argument("path", nargs="?", default=".", help="Git repository path, usually '.'")
    args = parser.parse_args()
    return run_app(Path(args.path))


if __name__ == "__main__":
    raise SystemExit(main())

