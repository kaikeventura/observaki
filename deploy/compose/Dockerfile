# syntax=docker/dockerfile:1
#
# Single image for the Observaki services used by the Docker Compose stack
# (Story 5.1). It builds the SvelteKit UI and the Go binary from source, so
# `docker compose up` works from a fresh clone without a local Go or Node
# toolchain. The release image (gcr.io/distroless) is assembled from prebuilt
# binaries by the release workflow; this file is for the single-node stack.

FROM node:26-alpine AS web
WORKDIR /src/web
RUN npm install --global pnpm@12.16.1
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM golang:1.27-alpine AS build
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
	-ldflags "-s -w \
	-X github.com/observaki/observaki/internal/version.Version=${VERSION} \
	-X github.com/observaki/observaki/internal/version.Commit=${COMMIT} \
	-X github.com/observaki/observaki/internal/version.Date=${BUILD_DATE}" \
	-o /out/observaki ./cmd/observaki \
	&& mkdir -p /out/spool

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/observaki /observaki
COPY --from=web /src/web/build /srv/web
# The writer spool lives in a named volume; create it here so a fresh volume
# inherits the nonroot ownership instead of root:root.
COPY --from=build --chown=nonroot:nonroot /out/spool /var/lib/observaki/spool

ENV OBSERVAKI_WEB_DIR=/srv/web
EXPOSE 4317 4318 8080 8082
ENTRYPOINT ["/observaki"]
CMD ["serve"]
