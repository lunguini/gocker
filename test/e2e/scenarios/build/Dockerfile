FROM docker.io/library/alpine:3
ARG BUILD_MARKER=from-build-arg
RUN echo "marker=$BUILD_MARKER" > /etc/e2e-marker
CMD ["/bin/sh", "-c", "sleep infinity"]
