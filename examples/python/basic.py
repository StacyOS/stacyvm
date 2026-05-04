#!/usr/bin/env python3
"""Basic StacyVM Python SDK usage.

Demonstrates:
  - Connecting to a StacyVM server
  - Spawning a sandbox
  - Executing a command
  - Writing and reading a file
  - Destroying the sandbox

Prerequisites:
  pip install stacyvm
  # StacyVM server running at localhost:7423
"""

from stacyvm import Client


def main() -> None:
    # Connect to a local StacyVM server (default port 7423).
    # Pass api_key="..." if authentication is enabled.
    with Client("http://localhost:7423") as client:

        # Check the server is healthy before doing real work.
        health = client.health()
        print(f"Server status: {health['status']}  version: {health.get('version', 'n/a')}")

        # Spawn a sandbox using the default provider and image.
        sandbox = client.spawn(image="alpine:latest", ttl="10m")
        print(f"Spawned sandbox: {sandbox.id}  state={sandbox.state}")

        # Get a preview URL (e.g. for a web server running on port 3000).
        preview_url = sandbox.get_preview_url(3000)
        print(f"Live Preview URL: {preview_url}")

        # Execute a command inside the sandbox.
        result = sandbox.exec("echo 'Hello from StacyVM!'")
        print(f"Exit code: {result.exit_code}")
        print(f"Stdout: {result.stdout.strip()}")

        # Write a file into the sandbox.
        sandbox.write_file("/tmp/greeting.txt", "Hello, world!\n")
        print("Wrote /tmp/greeting.txt")

        # Read it back.
        content = sandbox.read_file("/tmp/greeting.txt")
        print(f"Read back: {content.strip()!r}")

        # List files in /tmp.
        files = sandbox.list_files("/tmp")
        print(f"Files in /tmp: {[f['path'] for f in files]}")

        # Clean up.
        sandbox.destroy()
        print(f"Sandbox {sandbox.id} destroyed.")


if __name__ == "__main__":
    main()
