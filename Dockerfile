FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a

ENV GOPATH /go

COPY . /src

RUN apk update \
	&& apk add go git subversion libc-dev mercurial breezy openssh tini build-base npm rsync \
	&& cd /src \
	&& make \
	&& cp .build/bin/houndd /bin \
	&& rm -r .build \
	&& apk del go build-base rsync npm \
	&& rm -f /var/cache/apk/*

VOLUME ["/data"]

EXPOSE 6080

ENTRYPOINT ["/sbin/tini", "--", "/bin/houndd", "-conf", "/data/config.json"]
