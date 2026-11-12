# syntax=docker/dockerfile:1

FROM golang:1.27.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -u 10001 appuser
COPY --from=build /out/server /app/server
USER appuser
EXPOSE 8080
ENV PORT=8080
ENTRYPOINT ["/app/server"]
