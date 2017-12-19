FROM cargo.caicloudprivatetest.com/caicloud/alpine:3.6

COPY bin/linux_amd64/controller /usr/bin/canaryrelease

ENTRYPOINT ["/usr/bin/canaryrelease"]

CMD ["--debug"]
