# Build the single Shorty binary with a reproducible Go toolchain.
FROM golang:1.24-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/shorty ./cmd/shorty

# Minimal production runtime. Shorty persists SQLite data under /data.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/shorty /app/shorty

ENV SHORTY_ADDR=:8080
ENV SHORTY_DB=/data/shorty.db

VOLUME ["/data"]
EXPOSE 8080

USER nonroot:nonroot
ENTRYPOINT ["/app/shorty"]
