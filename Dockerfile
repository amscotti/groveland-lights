# syntax=docker/dockerfile:1

FROM golang:1.27.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.22
# ca-certificates for outbound TLS; tzdata for TZ=America/New_York; su-exec
# lets the entrypoint drop from root to appuser (tiny, no util-linux).
RUN apk add --no-cache ca-certificates tzdata su-exec \
 && adduser -D -H -u 10001 appuser
COPY --from=build /out/server /app/server
COPY scripts/docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod 0755 /docker-entrypoint.sh
# The entrypoint chowns the root-owned /data volume, so the image starts as
# root and drops to appuser via su-exec before running CMD.
USER root
EXPOSE 8080
ENV PORT=8080
ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["/app/server"]
