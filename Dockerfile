# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY documentation/embed.go documentation/openapi.yaml ./documentation/
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/openrgb-api ./cmd/openrgb-api \
    && mkdir /out/data

FROM scratch
COPY --from=build /out/openrgb-api /openrgb-api
# Configuration and paired devices live in a volume so they survive upgrades.
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
VOLUME /data
# The control panel listens on all container interfaces so Docker can forward to it.
# Publish it on the host's loopback address only.
ENV OPENRGB_API_ADMIN_ADDRESS=0.0.0.0:8743
EXPOSE 8742 8743
ENTRYPOINT ["/openrgb-api", "-config", "/data/config.toml"]
# host.docker.internal reaches the OpenRGB SDK server running on the Docker host.
CMD ["-listen", "0.0.0.0:8742", "-host", "host.docker.internal"]
