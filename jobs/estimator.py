#!/usr/bin/env python3
"""
Meridian Lex — Task Size Estimation Module
Calibrating estimation algorithm with historical variance correction.

Usage:
  estimator.py estimate <size> <complexity>     P50/P80/P95 estimate
  estimator.py history [--size M]               Historical sessions
  estimator.py calibration                       Variance correction per size bucket
  estimator.py annotate <session_id> --estimate N  Add estimated_minutes retroactively
"""

import fcntl
import json
import os
import sys
import tempfile
from contextlib import contextmanager
from datetime import datetime
from typing import Optional

DEFAULT_SESSIONS_FILE = os.environ.get(
    "LEX_TIME_SESSIONS",
    os.path.expanduser("~/meridian-home/lex-internal/state/time_sessions.jsonl"),
)

# ── Constants ─────────────────────────────────────────────────────────────────

BASE_MINUTES = {"XS": 10, "S": 20, "M": 45, "L": 90, "XL": 180}
COMPLEXITY_MULT = {"Low": 0.8, "Medium": 1.0, "High": 1.5}
POINTS = {"XS": 1, "S": 2, "M": 3, "L": 5, "XL": 8}

MIN_SAMPLES_FOR_CORRECTION = 3
WINDOW_SIZE = 10  # last N sessions used for variance correction
TAIL_SAMPLE_SIZE = 30  # tail sample for empirical p95/p80 percentile calculation


# ── File Locking Helpers ──────────────────────────────────────────────────────

@contextmanager
def _sessions_file_lock(path, mode):
    """Context manager for exclusive file locking using fcntl."""
    with open(path, mode) as f:
        fcntl.flock(f, fcntl.LOCK_EX)
        try:
            yield f
        finally:
            fcntl.flock(f, fcntl.LOCK_UN)


# ── Core Logic ────────────────────────────────────────────────────────────────

def _load_sessions(sessions_file: str):
    sessions = []
    try:
        with open(sessions_file, "r") as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                try:
                    sessions.append(json.loads(line))
                except json.JSONDecodeError:
                    pass
    except FileNotFoundError:
        pass
    return sessions


def estimate(size: str, complexity: str, sessions_file: str = DEFAULT_SESSIONS_FILE) -> dict:
    """
    Estimate task duration using historical variance correction.

    Returns dict with:
      size, complexity, points, base_minutes,
      variance_correction (float, 1.0 if insufficient data),
      p50_minutes, p80_minutes, p95_minutes,
      sample_count, confidence ("low"|"medium"|"high")
    """
    if size not in BASE_MINUTES:
        raise ValueError(f"Invalid size '{size}'. Valid: {list(BASE_MINUTES.keys())}")
    if complexity not in COMPLEXITY_MULT:
        raise ValueError(f"Invalid complexity '{complexity}'. Valid: {list(COMPLEXITY_MULT.keys())}")

    sessions = _load_sessions(sessions_file)

    # Find completed sessions matching this size with both duration and estimate
    matching = [
        s for s in sessions
        if s.get("status") == "completed"
        and s.get("size") == size
        and s.get("duration_seconds") is not None
        and s.get("estimated_minutes") is not None
        and s["estimated_minutes"] > 0
    ]

    # Sort by start_timestamp ascending, take last WINDOW_SIZE
    matching.sort(key=lambda s: s.get("start_timestamp", 0))
    window = matching[-WINDOW_SIZE:]

    sample_count = len(window)

    if sample_count >= MIN_SAMPLES_FOR_CORRECTION:
        ratios = [(s["duration_seconds"] / 60) / s["estimated_minutes"] for s in window]
        variance_correction = sum(ratios) / len(ratios)
    else:
        variance_correction = 1.0

    base = BASE_MINUTES[size]
    mult = COMPLEXITY_MULT[complexity]
    p50 = base * mult * variance_correction

    # P80 and P95: empirical ratios from the ratio distribution if enough samples,
    # else fixed multipliers. For percentile calculation, use TAIL_SAMPLE_SIZE
    # (decoupled from WINDOW_SIZE used for variance correction) to ensure sufficient data.
    # Both are multiplied by base * mult (not base * mult * correction)
    # so they represent the complexity-adjusted tail, not the mean-corrected value.
    tail_window = matching[-TAIL_SAMPLE_SIZE:]
    tail_sample_count = len(tail_window)

    if tail_sample_count >= 20:
        ratios = sorted(s["duration_seconds"] / 60 / s["estimated_minutes"] for s in tail_window)
        p80_idx = int(len(ratios) * 0.80)
        p95_idx = int(len(ratios) * 0.95)
        p80 = base * mult * ratios[min(p80_idx, len(ratios) - 1)]
        p95 = base * mult * ratios[min(p95_idx, len(ratios) - 1)]
    elif tail_sample_count >= 10:
        ratios = sorted(s["duration_seconds"] / 60 / s["estimated_minutes"] for s in tail_window)
        p80_idx = int(len(ratios) * 0.80)
        p95_idx = int(len(ratios) * 0.95)
        p80 = base * mult * ratios[min(p80_idx, len(ratios) - 1)]
        p95 = base * mult * ratios[min(p95_idx, len(ratios) - 1)]
    else:
        p80 = p50 * 1.5
        p95 = p50 * 2.5

    # Monotonicity guards: P80 >= P50 and P95 >= P80 must always hold.
    p80 = max(p80, p50)
    p95 = max(p95, p80)

    if sample_count >= 10:
        confidence = "high"
    elif sample_count >= MIN_SAMPLES_FOR_CORRECTION:
        confidence = "medium"
    else:
        confidence = "low"

    return {
        "size": size,
        "complexity": complexity,
        "points": POINTS[size],
        "base_minutes": base,
        "variance_correction": variance_correction,
        "p50_minutes": p50,
        "p80_minutes": p80,
        "p95_minutes": p95,
        "sample_count": sample_count,
        "confidence": confidence,
    }


def history(size_filter: Optional[str] = None, sessions_file: str = DEFAULT_SESSIONS_FILE):
    """Show historical completed sessions, optionally filtered by size."""
    sessions = _load_sessions(sessions_file)
    completed = [s for s in sessions if s.get("status") == "completed"]
    if size_filter:
        completed = [s for s in completed if s.get("size") == size_filter]
    completed.sort(key=lambda s: s.get("start_timestamp", 0))

    if not completed:
        print("No matching completed sessions.")
        return

    print(f"{'Session ID':<40} {'Size':<6} {'Complexity':<12} {'Duration':>10} {'Estimate':>10}")
    print("-" * 82)
    for s in completed:
        dur = s.get("duration_seconds")
        dur_str = f"{int(dur // 60)}m" if dur else "N/A"
        est = s.get("estimated_minutes")
        est_str = f"{est}m" if est else "N/A"
        size = s.get("size") or "-"
        comp = s.get("complexity") or "-"
        print(f"{s['session_id']:<40} {size:<6} {comp:<12} {dur_str:>10} {est_str:>10}")


def calibration(sessions_file: str = DEFAULT_SESSIONS_FILE):
    """Show variance correction factor per size bucket."""
    sessions = _load_sessions(sessions_file)
    print(f"{'Size':<6} {'Samples':>8} {'Correction':>12} {'Confidence':<12}")
    print("-" * 42)
    for size in BASE_MINUTES:
        matching = [
            s for s in sessions
            if s.get("status") == "completed"
            and s.get("size") == size
            and s.get("duration_seconds") is not None
            and s.get("estimated_minutes") is not None
            and s["estimated_minutes"] > 0
        ]
        matching.sort(key=lambda s: s.get("start_timestamp", 0))
        window = matching[-WINDOW_SIZE:]
        n = len(window)
        if n >= MIN_SAMPLES_FOR_CORRECTION:
            ratios = [(s["duration_seconds"] / 60) / s["estimated_minutes"] for s in window]
            correction = sum(ratios) / len(ratios)
            confidence = "high" if n >= 10 else "medium"
        else:
            correction = 1.0
            confidence = "low"
        print(f"{size:<6} {n:>8} {correction:>12.3f} {confidence:<12}")


def annotate(session_id: str, estimated_minutes: int, sessions_file: str = DEFAULT_SESSIONS_FILE):
    """Retroactively add estimated_minutes to a session.

    Validates that estimated_minutes > 0 before modifying the sessions file.
    Uses an exclusive file lock and atomic rename to prevent data loss when
    time_tracker.py appends to the same file concurrently.
    """
    if estimated_minutes <= 0:
        raise ValueError(f"estimated_minutes must be positive, got {estimated_minutes}")

    try:
        with _sessions_file_lock(sessions_file, "r+") as f:
            sessions = []
            for line in f:
                line = line.strip()
                if not line:
                    continue
                try:
                    sessions.append(json.loads(line))
                except json.JSONDecodeError:
                    pass

            found = False
            for s in sessions:
                if s["session_id"] == session_id:
                    s["estimated_minutes"] = estimated_minutes
                    found = True
                    break
            if not found:
                print(f"[ERR] Session '{session_id}' not found.")
                return

            dir_name = os.path.dirname(os.path.abspath(sessions_file))
            tmp_fd, tmp_path = tempfile.mkstemp(dir=dir_name)
            try:
                with os.fdopen(tmp_fd, "w") as tmp:
                    for s in sessions:
                        tmp.write(json.dumps(s) + "\n")
                os.replace(tmp_path, sessions_file)
            except Exception:
                os.unlink(tmp_path)
                raise

    except FileNotFoundError:
        print(f"[ERR] Sessions file not found: {sessions_file}")
        return

    print(f"Annotated '{session_id}' with estimated_minutes={estimated_minutes}")


# ── CLI ───────────────────────────────────────────────────────────────────────

USAGE = """\
Meridian Lex — Task Size Estimator

Usage: estimator.py <command> [args]

Commands:
  estimate <size> <complexity>       P50/P80/P95 estimate with variance correction
  history [--size <size>]            Show historical completed sessions
  calibration                        Variance correction factor per size bucket
  annotate <session_id> --estimate N Add estimated_minutes to a session retroactively

Sizes:       XS  S  M  L  XL
Complexity:  Low  Medium  High
"""


def main():
    if len(sys.argv) < 2:
        print(USAGE)
        return

    cmd = sys.argv[1]

    if cmd == "estimate":
        if len(sys.argv) < 4:
            print("Usage: estimator.py estimate <size> <complexity>")
            return
        size = sys.argv[2]
        complexity = sys.argv[3]
        try:
            result = estimate(size, complexity)
        except ValueError as e:
            print(f"[ERR] {e}")
            return
        print(f"Size:              {result['size']} ({result['points']} points)")
        print(f"Complexity:        {result['complexity']}")
        print(f"Base minutes:      {result['base_minutes']}")
        print(f"Variance factor:   {result['variance_correction']:.3f}  "
              f"(confidence: {result['confidence']}, n={result['sample_count']})")
        print(f"P50 (most likely): {result['p50_minutes']:.1f} min")
        print(f"P80 (comfortable): {result['p80_minutes']:.1f} min")
        print(f"P95 (worst case):  {result['p95_minutes']:.1f} min")

    elif cmd == "history":
        size_filter = None
        i = 2
        while i < len(sys.argv):
            if sys.argv[i] == "--size" and i + 1 < len(sys.argv):
                size_filter = sys.argv[i + 1]
                i += 2
            else:
                i += 1
        history(size_filter=size_filter)

    elif cmd == "calibration":
        calibration()

    elif cmd == "annotate":
        if len(sys.argv) < 5:
            print("Usage: estimator.py annotate <session_id> --estimate N")
            return
        session_id = sys.argv[2]
        est = None
        i = 3
        while i < len(sys.argv):
            if sys.argv[i] == "--estimate" and i + 1 < len(sys.argv):
                try:
                    est = int(sys.argv[i + 1])
                except ValueError:
                    print(f"[ERR] --estimate requires an integer, got: '{sys.argv[i + 1]}'")
                    return
                i += 2
            else:
                i += 1
        if est is None:
            print("[ERR] --estimate N required")
            return
        try:
            annotate(session_id, est)
        except ValueError as e:
            print(f"[ERR] {e}")
            return

    else:
        print(f"[ERR] Unknown command: '{cmd}'")
        print(USAGE)


if __name__ == "__main__":
    main()
