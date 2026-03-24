#!/bin/bash
set -e

# Start containerd in the background
echo "Starting containerd..."
containerd &
CONTAINERD_PID=$!

# Wait for containerd socket to be ready
for i in $(seq 1 30); do
    if [ -S /run/containerd/containerd.sock ]; then
        break
    fi
    sleep 0.5
done

if [ ! -S /run/containerd/containerd.sock ]; then
    echo "ERROR: containerd failed to start"
    exit 1
fi

echo "containerd is ready (pid $CONTAINERD_PID)"

# Trap shutdown signals to cleanly stop containerd
cleanup() {
    echo "Shutting down..."
    kill "$CONTAINERD_PID" 2>/dev/null
    wait "$CONTAINERD_PID" 2>/dev/null
}
trap cleanup SIGTERM SIGINT

# Run the provided command, or keep the container alive
if [ $# -gt 0 ]; then
    exec "$@"
else
    echo "gocker-base is ready. Waiting for commands..."
    # Keep container alive — gocker proxies commands via `container exec`
    wait "$CONTAINERD_PID"
fi
