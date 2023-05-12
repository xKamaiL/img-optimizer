FROM golang:1.20-alpine3.17 AS builder

ENV GOOS=linux
ENV GOARCH=amd64
ENV CGO_ENABLED=1
ENV GOAMD64=v3

WORKDIR /app
COPY go.mod go.sum main.go ./
COPY . .
RUN apk add --no-cache build-base vips-dev upx \
    && go mod download \
    && go test ./... \
    && go build -ldflags="-s -w" -o /main \
    && upx --best --lzma /main

FROM alpine:3.17
RUN apk add --no-cache vips-poppler ttf-liberation
COPY --from=builder /main ./
EXPOSE 8080
ENTRYPOINT ["/main"]


