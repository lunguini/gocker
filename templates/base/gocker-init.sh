#!/bin/bash
set -e

# Set up cgroup v2 delegation for nested containers.
# Enable all available controllers so containerd/runc can apply resource limits.
echo "Configuring cgroup v2..."
if [ -f /sys/fs/cgroup/cgroup.subtree_control ]; then
    # Move all processes out of the root cgroup into an init scope,
    # so the root cgroup can delegate controllers to children.
    mkdir -p /sys/fs/cgroup/init
    for pid in $(cat /sys/fs/cgroup/cgroup.procs 2>/dev/null); do
        echo "$pid" > /sys/fs/cgroup/init/cgroup.procs 2>/dev/null || true
    done
    # Enable all controllers
    for controller in cpuset cpu io memory pids; do
        echo "+$controller" > /sys/fs/cgroup/cgroup.subtree_control 2>/dev/null || true
    done
fi

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

# Start buildkitd in the background (required for nerdctl compose build)
echo "Starting buildkitd..."
buildkitd &
BUILDKIT_PID=$!

# Wait for buildkit socket
for i in $(seq 1 15); do
    if [ -S /run/buildkit/buildkitd.sock ]; then
        break
    fi
    sleep 0.5
done

if [ -S /run/buildkit/buildkitd.sock ]; then
    echo "buildkitd is ready (pid $BUILDKIT_PID)"
else
    echo "WARNING: buildkitd socket not ready yet, continuing anyway"
fi

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
    kill "$BUILDKIT_PID" 2>/dev/null
    kill "$CONTAINERD_PID" 2>/dev/null
    wait "$GOCKER_PID" 2>/dev/null
    wait "$BUILDKIT_PID" 2>/dev/null
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
