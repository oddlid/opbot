FROM golang:stretch as builder

RUN go get -d -u github.com/oddlid/opbot/cmd
WORKDIR ${GOPATH}/src/github.com/oddlid/opbot/cmd
RUN go get -d -v ./...
RUN make

FROM alpine:latest
LABEL maintainer="Odd E. Ebbesen <oddebb@gmail.com>"
RUN apk add --no-cache --update tini ca-certificates \
		&& \
		rm -rf /var/cache/apk/*

RUN adduser -D -u 1000 opbot
COPY --from=builder /go/src/github.com/oddlid/opbot/cmd/opbot.bin /usr/local/bin/opbot
RUN chown opbot /usr/local/bin/opbot && chmod 555 /usr/local/bin/opbot

USER opbot

ENTRYPOINT ["tini", "-g", "--", "opbot"]
CMD ["-h"]
