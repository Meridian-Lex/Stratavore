#!/usr/bin/env python3
"""
Mock Stratavore Daemon for WebUI Integration Testing
Provides minimal API endpoints to simulate daemon behavior
"""

from http.server import HTTPServer, BaseHTTPRequestHandler
import json
import logging

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


class MockDaemonHandler(BaseHTTPRequestHandler):
    """Mock daemon request handler"""

    def do_GET(self):
        """Handle GET requests"""
        if "/api/v1/health" in self.path:
            self.send_json_response(200, {
                "status": "healthy",
                "service": "mock-daemon",
                "version": "test"
            })
        elif "/api/status" in self.path:
            self.send_json_response(200, {
                "status": "success",
                "data": {
                    "jobs": [],
                    "progress": {},
                    "time_sessions": [],
                    "agents": {},
                    "agent_todos": []
                }
            })
        elif "/api/" in self.path:
            self.send_json_response(200, {
                "status": "success",
                "data": {"mock": True}
            })
        else:
            self.send_error(404, "Not Found")

    def do_POST(self):
        """Handle POST requests"""
        if "/api/" in self.path:
            content_length = int(self.headers.get("Content-Length", 0))
            if content_length > 0:
                body = self.rfile.read(content_length)
                try:
                    data = json.loads(body.decode("utf-8"))
                    logger.info(f"Received POST to {self.path}: {data}")
                except json.JSONDecodeError:
                    pass

            self.send_json_response(200, {
                "status": "success",
                "data": {"acknowledged": True}
            })
        else:
            self.send_error(404, "Not Found")

    def send_json_response(self, status_code, data):
        """Send JSON response"""
        body = json.dumps(data).encode("utf-8")
        self.send_response(status_code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format, *args):
        """Override log message"""
        logger.info(format % args)


def main():
    port = 8080
    server = HTTPServer(("127.0.0.1", port), MockDaemonHandler)
    logger.info(f"Mock Daemon running on port {port}")
    server.serve_forever()


if __name__ == "__main__":
    main()
