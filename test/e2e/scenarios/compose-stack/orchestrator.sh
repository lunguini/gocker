#!/bin/sh
set -eu

apk add --no-cache postgresql-client redis >/dev/null

echo "[orchestrator] waiting for db..."
until pg_isready -h db -U postgres >/dev/null 2>&1; do sleep 1; done

echo "[orchestrator] waiting for cache..."
until redis-cli -h cache ping 2>/dev/null | grep -q PONG; do sleep 1; done

echo "[orchestrator] reading from db..."
export PGPASSWORD=postgres
VALUE=$(psql -h db -U postgres -d postgres -Atqc "SELECT v FROM secrets WHERE k='answer';")
echo "[orchestrator] got value=$VALUE"

echo "[orchestrator] writing to cache..."
redis-cli -h cache SET answer "$VALUE" >/dev/null
redis-cli -h cache SAVE >/dev/null

echo "[orchestrator] writing backup..."
mkdir -p /backup
echo "$VALUE" > /backup/answer.txt
touch /backup/DONE

echo "[orchestrator] DONE"
exec sleep infinity
