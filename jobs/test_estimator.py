#!/usr/bin/env python3
"""Tests for estimator.py — task size estimation with variance correction."""

import json
import time as t
import io
import pytest
from estimator import estimate, history, calibration, annotate, BASE_MINUTES, COMPLEXITY_MULT


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


def _make_session(i, size="M", complexity="Medium", estimated=45, duration=2700, status="completed"):
    ts = t.time() - 7200 + i
    return {
        "session_id": f"job{i}_{int(ts)}",
        "job_id": f"job{i}", "agent": "lex",
        "status": status, "size": size, "complexity": complexity,
        "start_timestamp": ts, "end_timestamp": ts + duration,
        "duration_seconds": duration, "paused_time": 0, "pauses": [],
        "estimated_minutes": estimated, "notes": "",
        "start_time": "", "end_time": "", "created_at": ""
    }


def test_history_returns_completed_sessions(tmp_path, capsys):
    """history() prints completed sessions in tabular form."""
    sf = str(tmp_path / "h.jsonl")
    with open(sf, "w") as f:
        for i in range(3):
            f.write(json.dumps(_make_session(i, size="S")) + "\n")
    history(sessions_file=sf)
    captured = capsys.readouterr()
    assert "job0" in captured.out
    assert "job1" in captured.out
    assert "job2" in captured.out


def test_history_filter_by_size(tmp_path, capsys):
    """history(size_filter) only shows sessions with matching size."""
    sf = str(tmp_path / "h.jsonl")
    with open(sf, "w") as f:
        f.write(json.dumps(_make_session(0, size="S")) + "\n")
        f.write(json.dumps(_make_session(1, size="M")) + "\n")
    history(size_filter="S", sessions_file=sf)
    captured = capsys.readouterr()
    assert "job0" in captured.out
    assert "job1" not in captured.out


def test_history_empty_file(tmp_path, capsys):
    """history() with no sessions prints a message."""
    sf = str(tmp_path / "empty.jsonl")
    open(sf, "w").close()
    history(sessions_file=sf)
    captured = capsys.readouterr()
    assert "No matching" in captured.out


def test_calibration_output(tmp_path, capsys):
    """calibration() prints a table with all size buckets."""
    sf = str(tmp_path / "c.jsonl")
    with open(sf, "w") as f:
        for i in range(4):
            f.write(json.dumps(_make_session(i, size="M", estimated=45, duration=1350)) + "\n")
    calibration(sessions_file=sf)
    captured = capsys.readouterr()
    for size in ("XS", "S", "M", "L", "XL"):
        assert size in captured.out
    # M has 4 samples and ratio=0.5 → correction near 0.5
    lines = captured.out.strip().splitlines()
    m_line = next(l for l in lines if l.startswith("M"))
    assert "0.500" in m_line


def test_annotate_adds_estimate(tmp_path):
    """annotate() writes estimated_minutes into the matching session."""
    sf = str(tmp_path / "a.jsonl")
    session = _make_session(0)
    session.pop("estimated_minutes")
    with open(sf, "w") as f:
        f.write(json.dumps(session) + "\n")

    annotate(session["session_id"], 60, sessions_file=sf)

    with open(sf) as f:
        saved = json.loads(f.readline())
    assert saved["estimated_minutes"] == 60


def test_annotate_missing_session(tmp_path, capsys):
    """annotate() prints an error when session_id is not found."""
    sf = str(tmp_path / "a.jsonl")
    open(sf, "w").close()
    annotate("nonexistent_id", 30, sessions_file=sf)
    captured = capsys.readouterr()
    assert "[ERR]" in captured.out


def test_annotate_does_not_clobber_other_sessions(tmp_path):
    """annotate() only modifies the target session; others are unchanged."""
    sf = str(tmp_path / "a.jsonl")
    s0 = _make_session(0)
    s1 = _make_session(1)
    s0.pop("estimated_minutes")
    with open(sf, "w") as f:
        f.write(json.dumps(s0) + "\n")
        f.write(json.dumps(s1) + "\n")

    annotate(s0["session_id"], 99, sessions_file=sf)

    with open(sf) as f:
        lines = [json.loads(l) for l in f if l.strip()]
    target = next(s for s in lines if s["session_id"] == s0["session_id"])
    other = next(s for s in lines if s["session_id"] == s1["session_id"])
    assert target["estimated_minutes"] == 99
    assert other["estimated_minutes"] == s1["estimated_minutes"]
