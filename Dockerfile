FROM golang:1.16.0-buster AS builder
LABEL stage=intermediate
WORKDIR /build
COPY . .
ENV GO111MODULE=on
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64
RUN go build -ldflags '-X "github.com/realDragonium/Ultraviolet/cmd.uvVersion=docker"' ./cmd/Ultraviolet/ 

FROM scratch
WORKDIR /
COPY --from=builder /build/Ultraviolet ./ultraviolet
ENTRYPOINT [ "./ultraviolet", "run" ]