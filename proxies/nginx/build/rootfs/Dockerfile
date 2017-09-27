
FROM BASEIMAGE


RUN DEBIAN_FRONTEND=noninteractive apt-get update && apt-get install -y \
  diffutils \
  --no-install-recommends \
  && rm -rf /var/lib/apt/lists/*

RUN curl -sSL -o /tmp/dumb-init.deb http://ftp.us.debian.org/debian/pool/main/d/dumb-init/dumb-init_1.2.0-1_DUMB_ARCH.deb && \
  dpkg -i /tmp/dumb-init.deb && \
  rm /tmp/dumb-init.deb

ENTRYPOINT ["/usr/bin/dumb-init", "-v"]

COPY . /

CMD ["/proxy"]
