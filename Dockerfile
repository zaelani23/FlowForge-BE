# # Builder
FROM golang:1.25.0 as builder

WORKDIR /app

COPY . .

RUN go build -ldflags="-s -w" -o engine main.go

# # UPX
FROM hairyhenderson/upx:latest as upx

WORKDIR /app

COPY --from=builder /app/engine /app

RUN upx /app/engine

# # Distribution
FROM frolvlad/alpine-glibc:latest

WORKDIR /app

COPY --from=upx /app/engine /app

RUN touch .env

ENTRYPOINT ["/app/engine"]