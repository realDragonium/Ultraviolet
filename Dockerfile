FROM golang:1.16.0-buster AS builder
LABEL stage=intermediate
COPY . /ultraviolet
WORKDIR /ultraviolet/cmd/Ultraviolet
ENV GO111MODULE=on
RUN go build 

FROM scratch
WORKDIR /
COPY --from=builder /ultraviolet/Ultraviolet ./
ENTRYPOINT [ "./Ultraviolet" ]