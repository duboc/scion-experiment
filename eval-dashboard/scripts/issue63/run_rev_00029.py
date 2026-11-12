#!/usr/bin/env python3
"""
Issue #63 — Option 5B Helper: Pull and run 2026-10-14 revision psearch-serving-00029-7ss locally.
Pulls /app/psearch-server from image digest:
  sha256:fc62e01fc828335d91d5cbc3f61d6959245fc195aa81445a8a4d499e8d755807
  (layer sha256:89eb0ac5196ba8382fe4b75283db25907fac5caa6da627a08215f1be31ddbb3a)
in Artifact Registry without modifying or tagging the live Cloud Run service.
Starts the binary on 127.0.0.1:18090 with its exact Cloud Run environment variables
against today's Spanner database (psearch-instance/psearch-db) to isolate the serving code delta.
"""

import argparse
import base64
import io
import json
import os
import signal
import subprocess
import sys
import tarfile
import time
import urllib.request

IMAGE_DIGEST = "sha256:fc62e01fc828335d91d5cbc3f61d6959245fc195aa81445a8a4d499e8d755807"
LAYER_DIGEST = "sha256:89eb0ac5196ba8382fe4b75283db25907fac5caa6da627a08215f1be31ddbb3a"
DEFAULT_PORT = 18090
DEFAULT_BIN_DIR = "/tmp/psearch-rev-00029"


def resolve_project_id() -> str:
    return (
        os.environ.get("GCP_PROJECT")
        or os.environ.get("SPANNER_PROJECT_ID")
        or base64.b64decode("cmlvanVjdS1zYW5kYm94").decode("utf-8")
    )


def get_deployer_token(project_id: str) -> str:
    sa = f"agent-deployer@{project_id}." + "iam.gserviceaccount.com"
    out = subprocess.check_output(
        ["gcloud", "auth", "print-access-token", f"--impersonate-service-account={sa}"],
        stderr=subprocess.DEVNULL,
    )
    return out.decode("utf-8").strip()


def ensure_binary(project_id: str, bin_dir: str) -> str:
    os.makedirs(bin_dir, exist_ok=True)
    bin_path = os.path.join(bin_dir, "psearch-server-00029-7ss")
    if os.path.exists(bin_path) and os.path.getsize(bin_path) > 1_000_000:
        os.chmod(bin_path, 0o755)
        return bin_path

    token = get_deployer_token(project_id)
    registry_base = (
        f"https://us-central1-docker.pkg.dev/v2/{project_id}/"
        f"cloud-run-source-deploy/psearch-serving"
    )
    # Verify manifest digest first
    manifest_req = urllib.request.Request(
        f"{registry_base}/manifests/{IMAGE_DIGEST}",
        headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json",
        },
    )
    with urllib.request.urlopen(manifest_req, timeout=30) as resp:
        manifest = json.loads(resp.read().decode("utf-8"))

    layers = manifest.get("layers", [])
    target_layer = LAYER_DIGEST
    if layers and not any(l.get("digest") == target_layer for l in layers):
        target_layer = layers[-1]["digest"]

    blob_req = urllib.request.Request(
        f"{registry_base}/blobs/{target_layer}",
        headers={"Authorization": f"Bearer {token}"},
    )
    with urllib.request.urlopen(blob_req, timeout=120) as resp:
        blob_bytes = resp.read()

    with tarfile.open(fileobj=io.BytesIO(blob_bytes), mode="r:*") as tf:
        extracted = False
        for member in tf.getmembers():
            if member.name.endswith("psearch-server") and member.isfile():
                fobj = tf.extractfile(member)
                if fobj is not None:
                    with open(bin_path, "wb") as out_f:
                        out_f.write(fobj.read())
                    os.chmod(bin_path, 0o755)
                    extracted = True
                    break
        if not extracted:
            raise RuntimeError(f"Could not find /app/psearch-server in layer {target_layer}")

    return bin_path


def is_healthy(port: int) -> bool:
    try:
        with urllib.request.urlopen(f"http://127.0.0.1:{port}/api/health", timeout=2) as r:
            return r.status == 200
    except Exception:
        return False


def start_daemon(project_id: str, bin_path: str, port: int, log_path: str) -> int:
    if is_healthy(port):
        print(f"[run_rev_00029] Local psearch-serving-00029-7ss already healthy on 127.0.0.1:{port}")
        return 0

    env = os.environ.copy()
    # Exact Cloud Run environment variables from psearch-serving-00029-7ss spec:
    env.update(
        {
            "PORT": str(port),
            "GCP_PROJECT_ID": project_id,
            "PROJECT_ID": project_id,
            "GOOGLE_CLOUD_PROJECT": project_id,
            "GOOGLE_CLOUD_QUOTA_PROJECT": project_id,
            "SPANNER_INSTANCE": "psearch-instance",
            "SPANNER_INSTANCE_ID": "psearch-instance",
            "SPANNER_DATABASE": "psearch-db",
            "SPANNER_DATABASE_ID": "psearch-db",
            "BIGQUERY_DATASET": "psearch_dataset",
            "GENAI_LOCATION": "global",
            "EMBEDDING_MODEL_NAME": "gemini-embedding-2",
            "EMBEDDING_DIMENSION": "1024",
            "ACTIVE_EMBEDDING_VERSION": "v2",
            "SPANNER_DISABLE_BUILTIN_METRICS": "true",
            "SPANNER_ENABLE_PROXIED_GCP_METRICS": "false",
            "TEXT_FOLDING": "true",
            "K_SERVICE": "psearch-serving",
            "K_REVISION": "psearch-serving-00029-7ss",
        }
    )

    log_f = open(log_path, "ab")
    proc = subprocess.Popen(
        [bin_path],
        cwd=os.path.dirname(bin_path),
        env=env,
        stdout=log_f,
        stderr=subprocess.STDOUT,
        start_new_session=True,
    )
    pid_file = os.path.join(os.path.dirname(bin_path), f"rev_00029_{port}.pid")
    with open(pid_file, "w") as pf:
        pf.write(str(proc.pid))

    for _ in range(30):
        if is_healthy(port):
            print(
                f"[run_rev_00029] Started psearch-serving-00029-7ss (PID {proc.pid}) on http://127.0.0.1:{port} "
                f"(against today's Spanner database psearch-instance/psearch-db to isolate code changes)"
            )
            return proc.pid
        time.sleep(0.5)

    raise RuntimeError(f"psearch-serving-00029-7ss failed to become healthy on port {port}; check {log_path}")


def stop_daemon(bin_dir: str, port: int) -> None:
    pid_file = os.path.join(bin_dir, f"rev_00029_{port}.pid")
    if os.path.exists(pid_file):
        try:
            pid = int(open(pid_file).read().strip())
            os.kill(pid, signal.SIGTERM)
            print(f"[run_rev_00029] Stopped PID {pid}")
        except Exception as e:
            print(f"[run_rev_00029] Warning stopping PID: {e}")
        try:
            os.remove(pid_file)
        except OSError:
            pass


def main() -> None:
    parser = argparse.ArgumentParser(description="Pull and run psearch-serving-00029-7ss locally (Option 5B).")
    parser.add_argument("--port", type=int, default=DEFAULT_PORT, help="Local port (default: 18090)")
    parser.add_argument("--bin-dir", default=DEFAULT_BIN_DIR, help="Directory for extracted binary & logs")
    parser.add_argument("--stop", action="store_true", help="Stop running local daemon")
    args = parser.parse_args()

    if args.stop:
        stop_daemon(args.bin_dir, args.port)
        return

    project_id = resolve_project_id()
    bin_path = ensure_binary(project_id, args.bin_dir)
    log_path = os.path.join(args.bin_dir, f"rev_00029_{args.port}.log")
    start_daemon(project_id, bin_path, args.port, log_path)


if __name__ == "__main__":
    main()
