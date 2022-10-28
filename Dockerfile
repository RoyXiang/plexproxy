FROM golang:1.17 AS builder

ARG VERSION

ENV CGO_ENABLED=0

WORKDIR /go/src/github.com/RoyXiang/plexproxy/

COPY . .

RUN go install -v -ldflags "-s -w -X 'main.Version=${VERSION}'" -trimpath

FROM gcr.io/distroless/base-debian11:nonroot

COPY --from=builder --chown=nonroot /go/bin/plexproxy /usr/local/bin/

USER nonroot

EXPOSE 5000

ENTRYPOINT ["/usr/local/bin/plexproxy"]
