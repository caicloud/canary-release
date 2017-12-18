
FROM cargo.caicloudprivatetest.com/caicloud/canary-nginx-base:0.30 

COPY build/nginx-proxy /
COPY bin/linux_amd64/nginx-proxy /nginx-proxy

CMD ["/nginx-proxy"]
