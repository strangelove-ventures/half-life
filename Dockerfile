FROM golang:1.19-alpine AS build-env

ENV PACKAGES make git

RUN apk add --no-cache $PACKAGES

WORKDIR /go/src/github.com/strangelove-ventures/half-life

ADD . .

ARG TARGETARCH=amd64
ARG TARGETOS=linux

RUN export GOOS=${TARGETOS} GOARCH=${TARGETARCH} && make build

FROM alpine:edge

RUN apk add --no-cache ca-certificates

WORKDIR /root

COPY --from=build-env /go/src/github.com/strangelove-ventures/half-life/bin/halflife  /usr/bin/halflife

CMD ["halflife"]