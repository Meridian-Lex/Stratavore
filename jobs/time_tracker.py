#!/usr/bin/env python3
"""
Meridian Lex — Granular Time Tracking System
Track work sessions with second precision.

Sessions file location (in priority order):
  1. LEX_TIME_SESSIONS environment variable
  2. ~/meridian-home/lex-internal/state/time_sessions.jsonl (default)
"""

import fcntl
import json
import os
import sys
import time
from contextlib import contextmanager
from datetime import datetime, timedelta
from typing import Dict, List, Optional

DEFAULT_SESSIONS_FILE = os.path.expanduser(
    "~/meridian-home/lex-internal/state/time_sessions.jsonl"
)

VALID_SIZES = {"XS", "S", "M", "L", "XL"}
VALID_COMPLEXITY = {"Low", "Medium", "High"}


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


class TimeTracker:
    def __init__(self):
        self.sessions_file = os.environ.get("LEX_TIME_SESSIONS", DEFAULT_SESSIONS_FILE)
        os.makedirs(os.path.dirname(self.sessions_file), exist_ok=True)
        self._ensure_file_exists()

    def _ensure_file_exists(self):
        if not os.path.exists(self.sessions_file):
            with open(self.sessions_file, "w") as f:
                pass

    # ── Session I/O ──────────────────────────────────────────────────────────

    def _append_session(self, session: Dict):
        with _sessions_file_lock(self.sessions_file, "a") as f:
            f.write(json.dumps(session) + "\n")

    def _load_sessions(self) -> List[Dict]:
        sessions = []
        try:
            with _sessions_file_lock(self.sessions_file, "r") as f:
                for line in f:
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        sessions.append(json.loads(line))
                    except json.JSONDecodeError as exc:
                        print(f"[WARN] Skipping malformed session line: {exc}")
        except FileNotFoundError:
            pass
        return sessions

    def _save_sessions(self, sessions: List[Dict]):
        with _sessions_file_lock(self.sessions_file, "w") as f:
            for session in sessions:
                f.write(json.dumps(session) + "\n")

    def _get_session(self, session_id: str) -> Optional[Dict]:
        for s in self._load_sessions():
            if s["session_id"] == session_id:
                return s
        return None

    # ── Core Commands ─────────────────────────────────────────────────────────

    def start_session(self, job_id: str, agent: str, description: str = "",
                      size: str = None, complexity: str = None) -> str:
        """Start a new work session."""
        if size is not None and size not in VALID_SIZES:
            raise ValueError(f"Invalid size '{size}'. Valid: {sorted(VALID_SIZES)}")
        if complexity is not None and complexity not in VALID_COMPLEXITY:
            raise ValueError(f"Invalid complexity '{complexity}'. Valid: {sorted(VALID_COMPLEXITY)}")

        # Warn if this job already has an active session
        active = [s for s in self._load_sessions()
                  if s["job_id"] == job_id and s["status"] == "active"]
        if active:
            print(f"[WARN] Job '{job_id}' already has {len(active)} active session(s):")
            for s in active:
                print(f"       {s['session_id']}")
            print("       Use 'end' or 'cancel' before starting a new session for the same job.")

        session_id = f"{job_id}_{int(time.time())}"
        now = time.time()
        session = {
            "session_id": session_id,
            "job_id": job_id,
            "agent": agent,
            "description": description,
            "status": "active",
            "size": size,
            "complexity": complexity,
            "start_time": datetime.utcnow().isoformat() + "Z",
            "start_timestamp": now,
            "end_time": None,
            "end_timestamp": None,
            "duration_seconds": None,
            "paused_time": 0,
            "pauses": [],
            "notes": "",
            "created_at": datetime.utcnow().isoformat() + "Z",
        }
        self._append_session(session)
        print(f"Started  {session_id}")
        print(f"  Job:    {job_id}")
        if description:
            print(f"  Desc:   {description}")
        print(f"  Time:   {session['start_time']}")
        return session_id

    def end_session(self, session_id: str, notes: str = "") -> bool:
        """End an active work session."""
        sessions = self._load_sessions()
        for i, s in enumerate(sessions):
            if s["session_id"] == session_id:
                if s["status"] == "paused":
                    # Auto-resume before ending
                    s = self._apply_resume(s)
                if s["status"] != "active":
                    print(f"[ERR] Session {session_id} is '{s['status']}', not active.")
                    return False
                now = time.time()
                s["status"] = "completed"
                s["end_time"] = datetime.utcnow().isoformat() + "Z"
                s["end_timestamp"] = now
                s["duration_seconds"] = now - s["start_timestamp"] - s["paused_time"]
                s["notes"] = notes
                sessions[i] = s
                self._save_sessions(sessions)
                duration = timedelta(seconds=int(s["duration_seconds"]))
                print(f"Ended    {session_id}")
                print(f"  Duration: {duration}")
                if notes:
                    print(f"  Notes:    {notes}")
                return True
        print(f"[ERR] Session '{session_id}' not found.")
        return False

    def pause_session(self, session_id: str) -> bool:
        """Pause an active session."""
        sessions = self._load_sessions()
        for i, s in enumerate(sessions):
            if s["session_id"] == session_id:
                if s["status"] != "active":
                    print(f"[ERR] Session {session_id} is '{s['status']}', not active.")
                    return False
                now = time.time()
                s["status"] = "paused"
                s["pauses"].append({"pause_start": now})
                sessions[i] = s
                self._save_sessions(sessions)
                elapsed = timedelta(seconds=int(now - s["start_timestamp"] - s["paused_time"]))
                print(f"Paused   {session_id}  (elapsed so far: {elapsed})")
                return True
        print(f"[ERR] Session '{session_id}' not found.")
        return False

    def resume_session(self, session_id: str) -> bool:
        """Resume a paused session."""
        sessions = self._load_sessions()
        for i, s in enumerate(sessions):
            if s["session_id"] == session_id:
                if s["status"] != "paused":
                    print(f"[ERR] Session {session_id} is '{s['status']}', not paused.")
                    return False
                s = self._apply_resume(s)
                sessions[i] = s
                self._save_sessions(sessions)
                print(f"Resumed  {session_id}")
                return True
        print(f"[ERR] Session '{session_id}' not found.")
        return False

    def cancel_session(self, session_id: str) -> bool:
        """Cancel (abandon) a session without recording it as completed."""
        sessions = self._load_sessions()
        for i, s in enumerate(sessions):
            if s["session_id"] == session_id:
                if s["status"] == "completed":
                    print(f"[ERR] Session {session_id} is already completed. Cannot cancel.")
                    return False
                s["status"] = "cancelled"
                s["end_time"] = datetime.utcnow().isoformat() + "Z"
                sessions[i] = s
                self._save_sessions(sessions)
                print(f"Cancelled {session_id}")
                return True
        print(f"[ERR] Session '{session_id}' not found.")
        return False

    # ── Query Commands ────────────────────────────────────────────────────────

    def show_status(self):
        """Quick health check — active and paused sessions."""
        sessions = self._load_sessions()
        active = [s for s in sessions if s["status"] == "active"]
        paused = [s for s in sessions if s["status"] == "paused"]

        if not active and not paused:
            print("No active or paused sessions.")
            return

        now = time.time()
        if active:
            print(f"Active ({len(active)}):")
            for s in active:
                elapsed = timedelta(seconds=int(now - s["start_timestamp"] - s["paused_time"]))
                print(f"  {s['session_id']}")
                print(f"    Job:     {s['job_id']}")
                print(f"    Elapsed: {elapsed}")
                if s.get("description"):
                    print(f"    Desc:    {s['description']}")

        if paused:
            print(f"Paused ({len(paused)}):")
            for s in paused:
                paused_so_far = s["paused_time"] + (now - s["pauses"][-1]["pause_start"])
                elapsed = timedelta(seconds=int(now - s["start_timestamp"] - paused_so_far))
                print(f"  {s['session_id']}  (active time: {elapsed})")

    def show_active(self):
        """Show all currently active sessions (legacy command)."""
        self.show_status()

    def show_job(self, job_id: str):
        """Show cumulative time for a job."""
        info = self._calculate_job_time(job_id)
        if info["completed_sessions"] == 0 and info["active_sessions"] == 0:
            print(f"No sessions found for job '{job_id}'.")
            return
        print(f"Job: {job_id}")
        print(f"  Total time:          {info['formatted_time']}  ({info['total_hours']:.2f}h)")
        print(f"  Completed sessions:  {info['completed_sessions']}")
        if info["active_sessions"]:
            print(f"  Active sessions:     {info['active_sessions']}  (time included above)")

    def show_all(self):
        """Aggregated stats across all jobs."""
        sessions = self._load_sessions()
        if not sessions:
            print("No sessions recorded.")
            return

        job_ids = sorted({s["job_id"] for s in sessions})
        print(f"All sessions ({len(sessions)} total across {len(job_ids)} jobs):")
        grand_total = 0
        for job_id in job_ids:
            info = self._calculate_job_time(job_id)
            grand_total += info["total_seconds"]
            active_note = f"  [{info['active_sessions']} active]" if info["active_sessions"] else ""
            print(f"  {job_id:<30} {info['formatted_time']:>12}  "
                  f"({info['completed_sessions']} sessions){active_note}")
        print(f"  {'TOTAL':<30} {str(timedelta(seconds=int(grand_total))):>12}")

    def generate_report(self, job_id: str):
        """Generate a TIME-TRACKING.md-compatible markdown entry for a job."""
        sessions = [s for s in self._load_sessions()
                    if s["job_id"] == job_id and s["status"] == "completed"]
        if not sessions:
            print(f"No completed sessions for job '{job_id}'.")
            return

        sessions.sort(key=lambda s: s["start_timestamp"])
        first = sessions[0]
        last = sessions[-1]
        total_seconds = sum(s["duration_seconds"] for s in sessions if s["duration_seconds"])
        total_duration = timedelta(seconds=int(total_seconds))
        total_minutes = int(total_seconds / 60)

        start_dt = datetime.fromisoformat(first["start_time"].rstrip("Z"))
        end_dt = datetime.fromisoformat(last["end_time"].rstrip("Z"))

        notes_lines = [s["notes"] for s in sessions if s.get("notes")]
        descriptions = list({s["description"] for s in sessions if s.get("description")})

        print(f"### Task {job_id}: {descriptions[0] if descriptions else job_id}")
        print(f"- **Estimate**: N/A")
        print(f"- **Actual**: {total_minutes} minutes ({total_duration})")
        print(f"- **Variance**: N/A")
        print(f"- **Started**: {start_dt.strftime('%Y-%m-%d %H:%M')} UTC")
        print(f"- **Completed**: {end_dt.strftime('%Y-%m-%d %H:%M')} UTC")
        print(f"- **Sessions**: {len(sessions)}")
        if notes_lines:
            print(f"- **Notes**:")
            for note in notes_lines:
                print(f"  - {note}")

    def md_append(self, job_id: str, estimate_minutes: int = None, md_file: str = None):
        """Append a formatted session block to a TIME-TRACKING.md file."""
        DEFAULT_MD = os.path.expanduser(
            "~/meridian-home/lex-internal/state/TIME-TRACKING.md"
        )
        if md_file is None:
            md_file = DEFAULT_MD

        sessions = [s for s in self._load_sessions()
                    if s["job_id"] == job_id and s["status"] == "completed"]
        if not sessions:
            print(f"[ERR] No completed sessions for job '{job_id}'.")
            return

        sessions.sort(key=lambda s: s["start_timestamp"])
        first = sessions[0]
        last = sessions[-1]
        total_seconds = sum(s["duration_seconds"] for s in sessions if s["duration_seconds"])
        total_duration = timedelta(seconds=int(total_seconds))
        total_minutes = int(total_seconds / 60)

        start_dt = datetime.fromisoformat(first["start_time"].rstrip("Z"))
        end_dt = datetime.fromisoformat(last["end_time"].rstrip("Z"))

        notes_lines = [s["notes"] for s in sessions if s.get("notes")]
        size = last.get("size") or first.get("size")
        complexity = last.get("complexity") or first.get("complexity")

        date_str = start_dt.strftime("%Y-%m-%d")
        lines = [f"### Session: {job_id} ({date_str})"]
        if estimate_minutes is not None:
            lines.append(f"- **Estimate**: {estimate_minutes} minutes")
        lines.append(f"- **Actual**: {total_minutes} minutes ({total_duration})")
        if estimate_minutes is not None and estimate_minutes > 0:
            variance_pct = int(((total_minutes - estimate_minutes) / estimate_minutes) * 100)
            sign = "+" if variance_pct >= 0 else ""
            lines.append(f"- **Variance**: {sign}{variance_pct}%")
        if size:
            lines.append(f"- **Size**: {size}")
        if complexity:
            lines.append(f"- **Complexity**: {complexity}")
        lines.append(f"- **Started**: {start_dt.strftime('%Y-%m-%d %H:%M:%S')} UTC")
        lines.append(f"- **Completed**: {end_dt.strftime('%Y-%m-%d %H:%M:%S')} UTC")
        if notes_lines:
            lines.append(f"- **Notes**: {'; '.join(notes_lines)}")
        block = "\n".join(lines) + "\n"

        # Persist estimated_minutes back to session JSONL for calibration
        if estimate_minutes is not None:
            self._persist_estimate(job_id, estimate_minutes)

        # Read existing content; insert before trailing --- if present
        if os.path.exists(md_file):
            with _sessions_file_lock(md_file, "r") as f:
                content = f.read()
        else:
            content = ""

        if content.rstrip().endswith("---"):
            # Insert block before the trailing ---
            idx = content.rfind("\n---")
            content = content[:idx] + "\n\n" + block + content[idx:]
        else:
            content = content.rstrip("\n") + "\n\n" + block

        with _sessions_file_lock(md_file, "w") as f:
            f.write(content)

        print(f"Appended session block for '{job_id}' to {md_file}")

    def _persist_estimate(self, job_id: str, estimate_minutes: int) -> None:
        """Write estimated_minutes into completed sessions for a job (for calibration)."""
        sessions = self._load_sessions()
        for s in sessions:
            if s["job_id"] == job_id and s.get("status") == "completed" and not s.get("estimated_minutes"):
                s["estimated_minutes"] = estimate_minutes
        self._save_sessions(sessions)

    # ── Internal Helpers ──────────────────────────────────────────────────────

    def _apply_resume(self, session: Dict) -> Dict:
        """Record pause end and update paused_time."""
        now = time.time()
        if session["pauses"]:
            last_pause = session["pauses"][-1]
            if "pause_end" not in last_pause:
                pause_duration = now - last_pause["pause_start"]
                last_pause["pause_end"] = now
                session["paused_time"] = session.get("paused_time", 0) + pause_duration
        session["status"] = "active"
        return session

    def _calculate_job_time(self, job_id: str) -> Dict:
        sessions = [s for s in self._load_sessions() if s["job_id"] == job_id]
        now = time.time()
        total_seconds = 0
        completed = 0
        active = 0
        for s in sessions:
            if s["status"] == "completed" and s.get("duration_seconds"):
                total_seconds += s["duration_seconds"]
                completed += 1
            elif s["status"] == "active":
                total_seconds += now - s["start_timestamp"] - s.get("paused_time", 0)
                active += 1
        return {
            "job_id": job_id,
            "total_seconds": total_seconds,
            "total_hours": total_seconds / 3600,
            "completed_sessions": completed,
            "active_sessions": active,
            "formatted_time": str(timedelta(seconds=int(total_seconds))),
        }


# ── CLI ───────────────────────────────────────────────────────────────────────

USAGE = """\
Meridian Lex Time Tracker

Usage: time_tracker.py <command> [args]

Commands:
  start <job_id> <agent> [description] [--size XS|S|M|L|XL] [--complexity Low|Medium|High]
                                        Start a new work session
  end   <session_id> [notes]            End an active session
  pause <session_id>                    Pause an active session
  resume <session_id>                   Resume a paused session
  cancel <session_id>                   Abandon a session (not recorded as complete)
  status                                Show active and paused sessions
  active                                Alias for status
  job   <job_id>                        Show cumulative time for a job
  all                                   Show stats for all jobs
  report <job_id>                       Generate TIME-TRACKING.md entry for a job
  md-append <job_id> [--estimate N] [--file PATH]
                                        Append session block to TIME-TRACKING.md

Sessions file: {sessions_file}
Override:      export LEX_TIME_SESSIONS=/path/to/file
"""


def main():
    tracker = TimeTracker()

    if len(sys.argv) < 2:
        print(USAGE.format(sessions_file=tracker.sessions_file))
        return

    cmd = sys.argv[1]

    if cmd == "start":
        if len(sys.argv) < 4:
            print("Usage: time_tracker.py start <job_id> <agent> [description] [--size S] [--complexity Medium]")
            return
        args = sys.argv[4:]
        description = args[0] if args and not args[0].startswith("--") else ""
        size = None
        complexity = None
        i = 1 if description else 0
        while i < len(args):
            if args[i] == "--size" and i + 1 < len(args):
                size = args[i + 1]; i += 2
            elif args[i] == "--complexity" and i + 1 < len(args):
                complexity = args[i + 1]; i += 2
            else:
                i += 1
        tracker.start_session(sys.argv[2], sys.argv[3], description, size=size, complexity=complexity)

    elif cmd == "end":
        if len(sys.argv) < 3:
            print("Usage: time_tracker.py end <session_id> [notes]")
            return
        tracker.end_session(sys.argv[2], sys.argv[3] if len(sys.argv) > 3 else "")

    elif cmd == "pause":
        if len(sys.argv) < 3:
            print("Usage: time_tracker.py pause <session_id>")
            return
        tracker.pause_session(sys.argv[2])

    elif cmd == "resume":
        if len(sys.argv) < 3:
            print("Usage: time_tracker.py resume <session_id>")
            return
        tracker.resume_session(sys.argv[2])

    elif cmd == "cancel":
        if len(sys.argv) < 3:
            print("Usage: time_tracker.py cancel <session_id>")
            return
        tracker.cancel_session(sys.argv[2])

    elif cmd in ("status", "active"):
        tracker.show_status()

    elif cmd == "job":
        if len(sys.argv) < 3:
            print("Usage: time_tracker.py job <job_id>")
            return
        tracker.show_job(sys.argv[2])

    elif cmd == "all":
        tracker.show_all()

    elif cmd == "report":
        if len(sys.argv) < 3:
            print("Usage: time_tracker.py report <job_id>")
            return
        tracker.generate_report(sys.argv[2])

    elif cmd == "md-append":
        if len(sys.argv) < 3:
            print("Usage: time_tracker.py md-append <job_id> [--estimate N] [--file PATH]")
            return
        job_id = sys.argv[2]
        estimate_minutes = None
        md_file = None
        i = 3
        while i < len(sys.argv):
            if sys.argv[i] == "--estimate" and i + 1 < len(sys.argv):
                try:
                    estimate_minutes = int(sys.argv[i + 1])
                except ValueError:
                    print(f"[ERR] --estimate requires an integer, got: '{sys.argv[i + 1]}'")
                    return
                i += 2
            elif sys.argv[i] == "--file" and i + 1 < len(sys.argv):
                md_file = sys.argv[i + 1]; i += 2
            else:
                i += 1
        tracker.md_append(job_id, estimate_minutes=estimate_minutes, md_file=md_file)

    else:
        print(f"[ERR] Unknown command: '{cmd}'")
        print(USAGE.format(sessions_file=tracker.sessions_file))


if __name__ == "__main__":
    main()
