# Minimal runtime image for the Observaki server.
#
# The release workflow builds dist/observaki-linux-${TARGETARCH} before this
# image is assembled. The single-node Compose stack builds its own image from
# source in deploy/compose/Dockerfile; this file only packages the alpha
# server binary for releases.

FROM gcr.io/distroless/static-debian12:nonroot

ARG TARGETARCH
COPY dist/observaki-linux-${TARGETARCH} /observaki

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/observaki"]
CMD ["serve"]
