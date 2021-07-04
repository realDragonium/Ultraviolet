FROM golang:1.16.0-buster AS builder
LABEL stage=intermediate
COPY . /ultraviolet
WORKDIR /ultraviolet
CMD pwd
ENV GO111MODULE=on
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -a --installsuffix cgo -v -tags netgo -ldflags '-extldflags "-static"' -o /main .

FROM scratch
WORKDIR /
COPY --from=builder /main ./
ENTRYPOINT [ "./main" ]


# FROM golang:1.16.0-buster AS builder
# LABEL stage=intermediate
# COPY . /ultraviolet
# WORKDIR /ultraviolet
# ENV GO111MODULE=on 
# RUN go build -o /go/bin/ultraviolet

# FROM scratch
# WORKDIR /go/bin
# COPY --from=builder  /go/bin/ultraviolet  ./
# ENTRYPOINT [ "./ultraviolet" ]