# Build — GOTOOLCHAIN=auto downloads the toolchain required by go.mod if needed.
FROM golang:1.25-bookworm AS build
ENV GOTOOLCHAIN=auto
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/kinframe ./cmd/kinframe

# Runtime (alpine so Compose healthchecks can curl /healthz)
FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl \
  && adduser -D -u 65532 -g nonroot nonroot
WORKDIR /
COPY --from=build /out/kinframe /kinframe
COPY themes /themes
RUN mkdir -p /data && chown -R nonroot:nonroot /data /themes
USER nonroot
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/kinframe"]
CMD ["app"]
