FROM golang:1.27 as build-root

WORKDIR /go/src/github.com/cyverse-de/dashboard-aggregator
COPY . .

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

RUN go build --buildvcs=false .
RUN go clean -cache -modcache
RUN cp ./dashboard-aggregator /bin/dashboard-aggregator

ENTRYPOINT ["dashboard-aggregator"]

EXPOSE 3000
