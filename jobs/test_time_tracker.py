#!/usr/bin/env python3
"""Tests for time_tracker.py — size/complexity metadata and md-append command."""

import os
import json
import tempfile
import pytest
from time_tracker import TimeTracker


def make_tracker(tmp_path):
    t = TimeTracker()
    t.sessions_file = str(tmp_path / "sessions.jsonl")
    t._ensure_file_exists()
    return t


def test_start_with_metadata(tmp_path):
    t = make_tracker(tmp_path)
    sid = t.start_session("job1", "lex", "fix parser", size="M", complexity="High")
    sessions = t._load_sessions()
    assert sessions[-1]["size"] == "M"
    assert sessions[-1]["complexity"] == "High"


def test_start_without_metadata_defaults_none(tmp_path):
    t = make_tracker(tmp_path)
    sid = t.start_session("job2", "lex", "misc work")
    sessions = t._load_sessions()
    assert sessions[-1].get("size") is None
    assert sessions[-1].get("complexity") is None


def test_md_append_writes_block(tmp_path):
    t = make_tracker(tmp_path)
    sid = t.start_session("job3", "lex", "some task", size="S", complexity="Low")
    t.end_session(sid, notes="done")
    md_file = str(tmp_path / "TIME-TRACKING.md")
    with open(md_file, "w") as f:
        f.write("# Time Tracking\n\n---\n")
    t.md_append("job3", estimate_minutes=20, md_file=md_file)
    content = open(md_file).read()
    assert "job3" in content
    assert "Actual" in content
    assert "Variance" in content


def test_md_append_no_estimate_omits_variance(tmp_path):
    t = make_tracker(tmp_path)
    sid = t.start_session("job4", "lex", "another task")
    t.end_session(sid)
    md_file = str(tmp_path / "TT.md")
    open(md_file, "w").close()
    t.md_append("job4", md_file=md_file)
    content = open(md_file).read()
    assert "job4" in content
    assert "Variance" not in content


def test_md_append_includes_size_complexity(tmp_path):
    t = make_tracker(tmp_path)
    sid = t.start_session("job5", "lex", "sized task", size="L", complexity="High")
    t.end_session(sid, notes="complete")
    md_file = str(tmp_path / "TT2.md")
    open(md_file, "w").close()
    t.md_append("job5", estimate_minutes=90, md_file=md_file)
    content = open(md_file).read()
    assert "Size" in content
    assert "L" in content
    assert "Complexity" in content
    assert "High" in content
