# Build the single Shorty binary with the Go version declared by go.mod.
FROM golang:1.25-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/shorty ./cmd/shorty && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/shorty-cli ./cmd/shorty-cli

# Minimal production runtime. Shorty persists SQLite data under /data.
FROM gcr.io/distroless/static-debian13:nonroot

WORKDIR /app
COPY --from=build /out/shorty /out/shorty-cli /app/

ENV SHORTY_ADDR=:8080
ENV SHORTY_DB=/data/shorty.db

VOLUME ["/data"]
EXPOSE 8080

USER nonroot:nonroot
ENTRYPOINT ["/app/shorty"]
