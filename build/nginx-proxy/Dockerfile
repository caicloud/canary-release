
FROM cargo.caicloudprivatetest.com/caicloud/canary-nginx-base:0.30 

COPY build/nginx-proxy /
COPY bin/linux_amd64/nginx-proxy /nginx-proxy

RUN mkdir -p /var/log/nginx && \
    touch /var/log/nginx/access.log && \
    touch /var/log/nginx/error.log

CMD ["/nginx-proxy"]
