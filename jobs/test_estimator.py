#!/usr/bin/env python3
"""Tests for estimator.py — task size estimation with variance correction."""

import json
import time as t
import pytest
from estimator import estimate, BASE_MINUTES, COMPLEXITY_MULT


def test_bootstrap_no_history(tmp_path):
    """With no matching sessions, returns correction=1.0 and confidence=low."""
    sessions_file = str(tmp_path / "empty.jsonl")
    open(sessions_file, "w").close()
    result = estimate("M", "Medium", sessions_file=sessions_file)
    assert result["variance_correction"] == 1.0
    assert result["confidence"] == "low"
    assert result["p50_minutes"] == BASE_MINUTES["M"] * COMPLEXITY_MULT["Medium"]


def test_known_size_complexity(tmp_path):
    """P50 = base * complexity_mult when no history."""
    sf = str(tmp_path / "e.jsonl")
    open(sf, "w").close()
    result = estimate("S", "Low", sessions_file=sf)
    assert result["p50_minutes"] == BASE_MINUTES["S"] * COMPLEXITY_MULT["Low"]
    assert result["points"] == 2


def test_variance_correction_applied(tmp_path):
    """If historical sessions show 0.5 ratio, p50 is halved."""
    sf = str(tmp_path / "s.jsonl")
    # Create 3 completed sessions with size=M, duration=22.5 min, estimate=45 min → ratio=0.5
    with open(sf, "w") as f:
        for i in range(3):
            ts = t.time() - 3600 + i
            session = {
                "session_id": f"job{i}_{int(ts)}",
                "job_id": f"job{i}", "agent": "lex",
                "status": "completed", "size": "M", "complexity": "Medium",
                "start_timestamp": ts, "end_timestamp": ts + 1350,
                "duration_seconds": 1350, "paused_time": 0, "pauses": [],
                "estimated_minutes": 45, "notes": "",
                "start_time": "", "end_time": "", "created_at": ""
            }
            f.write(json.dumps(session) + "\n")
    result = estimate("M", "Medium", sessions_file=sf)
    assert abs(result["variance_correction"] - 0.5) < 0.01
    assert result["confidence"] == "medium"
    assert abs(result["p50_minutes"] - 22.5) < 0.5


def test_invalid_size_raises():
    with pytest.raises(ValueError):
        estimate("XXL", "Medium")


def test_invalid_complexity_raises(tmp_path):
    sf = str(tmp_path / "e.jsonl")
    open(sf, "w").close()
    with pytest.raises(ValueError):
        estimate("M", "Extreme", sessions_file=sf)


def test_high_confidence_with_ten_samples(tmp_path):
    """10+ samples yields high confidence."""
    sf = str(tmp_path / "h.jsonl")
    with open(sf, "w") as f:
        for i in range(10):
            ts = t.time() - 7200 + i
            session = {
                "session_id": f"job{i}_{int(ts)}",
                "job_id": f"job{i}", "agent": "lex",
                "status": "completed", "size": "S", "complexity": "Low",
                "start_timestamp": ts, "end_timestamp": ts + 1200,
                "duration_seconds": 1200, "paused_time": 0, "pauses": [],
                "estimated_minutes": 20, "notes": "",
                "start_time": "", "end_time": "", "created_at": ""
            }
            f.write(json.dumps(session) + "\n")
    result = estimate("S", "Low", sessions_file=sf)
    assert result["confidence"] == "high"
    assert result["sample_count"] == 10


def test_sessions_without_estimate_ignored(tmp_path):
    """Sessions missing estimated_minutes are excluded from variance correction."""
    sf = str(tmp_path / "n.jsonl")
    with open(sf, "w") as f:
        for i in range(5):
            ts = t.time() - 3600 + i
            session = {
                "session_id": f"job{i}_{int(ts)}",
                "job_id": f"job{i}", "agent": "lex",
                "status": "completed", "size": "M", "complexity": "Medium",
                "start_timestamp": ts, "end_timestamp": ts + 2700,
                "duration_seconds": 2700, "paused_time": 0, "pauses": [],
                "notes": "", "start_time": "", "end_time": "", "created_at": ""
                # No estimated_minutes
            }
            f.write(json.dumps(session) + "\n")
    result = estimate("M", "Medium", sessions_file=sf)
    # No usable ratio data → correction stays 1.0
    assert result["variance_correction"] == 1.0
    assert result["confidence"] == "low"
