import os
from typing import Annotated

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
from stacyvm import Client, ProviderError


class RunPythonRequest(BaseModel):
    code: Annotated[str, Field(min_length=1, max_length=50_000)]
    timeout: str = "10s"


class RunPythonResponse(BaseModel):
    exit_code: int
    stdout: str
    stderr: str
    duration: str


app = FastAPI(title="StacyVM Code Runner")


def stacy_client() -> Client:
    return Client(
        base_url=os.getenv("STACYVM_URL", "http://localhost:7423"),
        api_key=os.getenv("STACYVM_API_KEY"),
        user_id=os.getenv("STACYVM_USER_ID", "example-code-runner"),
        timeout=60.0,
    )


@app.post("/run-python", response_model=RunPythonResponse)
def run_python(request: RunPythonRequest) -> RunPythonResponse:
    image = os.getenv("STACYVM_IMAGE", "python:3.12")
    sandbox = None

    try:
        client = stacy_client()
        sandbox = client.spawn(
            image=image,
            ttl="2m",
            memory_mb=512,
            vcpus=1,
            metadata={"example": "code-runner-python"},
        )
        sandbox.write_file("/app/main.py", request.code)
        result = sandbox.exec("python3 /app/main.py", timeout=request.timeout)
        return RunPythonResponse(
            exit_code=result.exit_code,
            stdout=result.stdout,
            stderr=result.stderr,
            duration=result.duration,
        )
    except ProviderError as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc
    except Exception as exc:
        raise HTTPException(status_code=500, detail=str(exc)) from exc
    finally:
        if sandbox is not None:
            try:
                sandbox.destroy()
            except Exception:
                pass
