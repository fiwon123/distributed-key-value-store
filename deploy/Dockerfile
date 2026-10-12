# syntax=docker/dockerfile:1.6

# Runtime image for one cluster node. The build context is the repo root:
#
#   docker build -f deploy/Dockerfile -t distributed-key-value-store/node .
#
# The node embeds backend/internal/webui/dist, which holds the committed
# placeholder unless `make web-build` ran first — run it to bake the real
# dashboard in. deploy/docker-compose.yml builds this image for the 3-node
# cluster.
#
# go.mod stays at the repo root; Go sources live under backend/.
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -o /out/kvnode ./backend/cmd/kvnode

# Static (CGO off) single binary; distroless has no shell, so the node is the
# only thing running. It runs as root, which is what the named data volume
# mounted at /data expects.
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/kvnode /kvnode
EXPOSE 8080
ENTRYPOINT ["/kvnode"]
