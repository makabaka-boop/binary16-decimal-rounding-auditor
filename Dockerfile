# syntax=docker/dockerfile:1

# Build stage: pure stdlib module, no module downloads required.
FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY cmd ./cmd
RUN CGO_ENABLED=0 GOFLAGS=-mod=mod go build -trimpath -ldflags="-s -w" \
    -o /out/halfconv ./cmd/halfconv

# Runtime stage: static binary on a minimal base.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/halfconv /usr/local/bin/halfconv
# Default batch input; mount or replace this file to supply measurements.
COPY samples/batch.json /data/batch.json
ENTRYPOINT ["/usr/local/bin/halfconv"]
# Run the bundled batch by default; override with a mounted JSON path or by
# piping JSON on stdin (see README.md).
CMD ["/data/batch.json"]
