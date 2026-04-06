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

# Start gocker daemon with Docker-compatible socket so tools like Portainer
# can connect to /var/run/docker.sock inside the VM.
echo "Starting gocker daemon..."
gocker daemon start --foreground --socket /var/run/docker.sock &
GOCKER_PID=$!

# Wait for the socket to appear
for i in $(seq 1 15); do
    if [ -S /var/run/docker.sock ]; then
        break
    fi
    sleep 0.5
done

if [ -S /var/run/docker.sock ]; then
    echo "gocker daemon is ready (pid $GOCKER_PID)"
else
    echo "WARNING: gocker daemon socket not ready yet, continuing anyway"
fi

# Trap shutdown signals to cleanly stop both processes
cleanup() {
    echo "Shutting down..."
    kill "$GOCKER_PID" 2>/dev/null
    kill "$CONTAINERD_PID" 2>/dev/null
    wait "$GOCKER_PID" 2>/dev/null
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
