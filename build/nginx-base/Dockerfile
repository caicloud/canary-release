FROM quay.io/kubernetes-ingress-controller/nginx-arm64:0.30

LABEL maintainer="Jun Zhang <jim.zhang@caicloud.io>"

RUN clean-install \
  diffutils \
  dumb-init

ENTRYPOINT ["/usr/bin/dumb-init"]
