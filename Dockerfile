FROM golang:1.16 as build-env

WORKDIR /go/src/app
ADD . /go/src/app

RUN go get -d -v ./...

RUN go build -o /go/bin/mara-xporter

FROM gcr.io/distroless/base
COPY --from=build-env /go/bin/mara-xporter /
CMD ["/mara-xporter"]
