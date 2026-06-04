# dws telemetry ingest — container image (AONE-deployable / any container platform).
#
# Build:  docker build -t dws-telemetry-ingest .
# Run:    docker run -p 9000:9000 -e INGEST_TOKEN=<token> dws-telemetry-ingest        # dry-run
#         docker run -p 9000:9000 -e INGEST_TOKEN=<token> \
#           -e SLS_ENDPOINT=... -e SLS_PROJECT=... -e SLS_LOGSTORE=... dws-telemetry-ingest   # -> SLS
#
# For AONE: point the app's build at this Dockerfile; expose port 9000; set the
# env vars (INGEST_TOKEN required; SLS_* to write to an internal SLS Logstore).
# Grant the running identity SLS PutLogs so app.py uses injected creds.
FROM python:3.10-slim

WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app.py .

ENV PORT=9000
EXPOSE 9000

# gunicorn web server; app.py auto-detects dry-run vs SLS mode from env.
CMD ["gunicorn", "-b", "0.0.0.0:9000", "--workers", "2", "--timeout", "30", "app:app"]
