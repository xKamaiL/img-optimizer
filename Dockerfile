FROM golang:1.20.3-alpine

RUN apk --no-cache add git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o app

EXPOSE 8080

CMD ["./app"]