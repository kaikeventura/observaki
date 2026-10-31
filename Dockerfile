# Minimal runtime image for the Observaki server.
#
# The release workflow builds dist/observaki-linux-${TARGETARCH} before this
# image is assembled. Compose/Helm packaging is Story 5.1; this image only
# packages the alpha server binary.

FROM gcr.io/distroless/static-debian12:nonroot

ARG TARGETARCH
COPY dist/observaki-linux-${TARGETARCH} /observaki

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/observaki"]
CMD ["serve"]
