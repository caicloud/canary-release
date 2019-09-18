FROM cargo.caicloud.xyz/library/debian:stretch

COPY bin/controller /usr/bin/canaryrelease

ENTRYPOINT ["/usr/bin/canaryrelease"]

CMD ["--debug"]
