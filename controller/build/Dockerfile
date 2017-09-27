FROM alpine:3.6

WORKDIR /root

COPY ./bin/controller /usr/bin/canaryrelease

ENTRYPOINT ["/usr/bin/canaryrelease"]

CMD ["--debug"]
