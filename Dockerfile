FROM golang:latest


# Install BIMG and reqs
RUN apt-get update
RUN apt-get install -y libvips libvips-dev


RUN mkdir -p /workspace
WORKDIR /workspace
ADD go.mod go.sum ./
RUN go mod download
ADD . .
RUN go build -trimpath -o ./main -ldflags "-w -s -extldflags " ./main.go

ENTRYPOINT ["/main"]
