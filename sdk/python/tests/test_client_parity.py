"""Mocked SDK parity checks that do not require a live StacyVM server."""

from __future__ import annotations

import unittest

import httpx

from stacyvm import Client


class FakeHTTP:
    def __init__(self):
        self.requests: list[dict] = []
        self.headers = {}

    def post(self, path: str, json: dict | None = None):
        self.requests.append({"method": "POST", "path": path, "json": json})
        if path == "/api/v1/sandboxes":
            body = json or {}
            return response(
                {
                    "id": "sb-parity",
                    "state": "running",
                    "provider": body.get("provider", "mock"),
                    "image": body.get("image", "alpine:latest"),
                    "memory_mb": body.get("memory_mb", 512),
                    "vcpus": body.get("vcpus", 1),
                    "created_at": "2026-05-08T00:00:00Z",
                    "expires_at": "2026-05-08T00:30:00Z",
                    "metadata": body.get("metadata", {}),
                    "preview_domain": "localhost",
                }
            )
        if path == "/api/v1/sandboxes/admission":
            return response(
                {
                    "allowed": True,
                    "queueable": False,
                    "active_sandboxes": 1,
                    "max_sandboxes": 100,
                }
            )
        raise AssertionError(f"unexpected POST {path}")

    def get(self, path: str):
        self.requests.append({"method": "GET", "path": path})
        if path == "/api/v1/providers":
            return response([{"name": "mock", "healthy": True, "default": True}])
        if path == "/api/v1/quotas/summary":
            return response(
                {
                    "total": 1,
                    "with_max_sandboxes": 1,
                    "with_max_ttl": 0,
                    "with_max_exec_timeout": 0,
                }
            )
        if path == "/api/v1/health":
            return response({"status": "ok", "version": "test", "uptime": "1s"})
        raise AssertionError(f"unexpected GET {path}")

    def close(self):
        pass


def response(body):
    request = httpx.Request("GET", "http://stacyvm.test")
    return httpx.Response(200, json=body, request=request)


def client_with_fake_http() -> tuple[Client, FakeHTTP]:
    client = Client("http://stacyvm.test", api_key="api-key", user_id="team-a")
    fake = FakeHTTP()
    client._http = fake
    client.templates._http = fake
    return client, fake


class ClientParityTests(unittest.TestCase):
    def test_spawn_sends_control_plane_fields(self):
        client, fake = client_with_fake_http()

        sandbox = client.spawn(
            image="python:3.12-slim",
            provider="mock",
            memory_mb=1024,
            vcpus=2,
            ttl="1h",
            owner_id="team-a",
            template="python-dev",
            metadata={"purpose": "parity"},
        )

        self.assertEqual(sandbox.id, "sb-parity")
        self.assertEqual(
            fake.requests[0],
            {
                "method": "POST",
                "path": "/api/v1/sandboxes",
                "json": {
                    "image": "python:3.12-slim",
                    "provider": "mock",
                    "memory_mb": 1024,
                    "vcpus": 2,
                    "ttl": "1h",
                    "owner_id": "team-a",
                    "template": "python-dev",
                    "metadata": {"purpose": "parity"},
                },
            },
        )

    def test_exposes_admission_providers_quota_summary_and_health_helpers(self):
        client, fake = client_with_fake_http()

        self.assertTrue(client.admission(image="alpine", owner_id="team-a").allowed)
        self.assertEqual(
            client.providers(), [{"name": "mock", "healthy": True, "default": True}]
        )
        self.assertEqual(client.quota_summary().total, 1)
        self.assertEqual(client.health()["status"], "ok")

        self.assertEqual(
            [request["path"] for request in fake.requests],
            [
                "/api/v1/sandboxes/admission",
                "/api/v1/providers",
                "/api/v1/quotas/summary",
                "/api/v1/health",
            ],
        )


if __name__ == "__main__":
    unittest.main()
