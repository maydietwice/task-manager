FROM golang:1.26-alpine3.23
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN go build -o server ./cmd/server
ENTRYPOINT ["/app/server"]