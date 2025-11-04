# Build Nexus proxy server from source
FROM golang:1.22 AS build
ARG NEXUS_VERSION=v0.2.0
WORKDIR /src
ENV GOTOOLCHAIN=auto
RUN go install github.com/AtDexters-Lab/nexus-proxy-server/proxy-server@${NEXUS_VERSION}

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /go/bin/proxy-server /usr/local/bin/nexus-proxy-server
EXPOSE 80 443 8443 8444
ENTRYPOINT ["/usr/local/bin/nexus-proxy-server"]
CMD ["-config", "/config/nexus-config.yaml"]
