#!/bin/sh
# Runtime image entrypoint. Fly volumes mount /data root-owned, but the app
# runs as uid 10001 (appuser), which could not create the SQLite file there
# unaided. As root, repair the ownership of /data best-effort, then drop
# privileges and exec the server (CMD). Local `docker run` without a volume
# must still boot, hence the `|| true` on the chown.
set -eu

if [ -d /data ]; then
    chown -R 10001:10001 /data || true
fi

# su-exec execs the server as appuser, replacing this process, so the server
# stays PID 1 and keeps its signal handling for graceful shutdown.
exec su-exec appuser "$@"
