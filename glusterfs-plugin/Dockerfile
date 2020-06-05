FROM golang:latest AS builder
COPY glusterfs-volume /go/src/docker-volume-glusterfs/glusterfs-volume/
COPY glusterfs-plugin /go/src/docker-volume-glusterfs/glusterfs-plugin/
COPY go.mod go.sum /go/src/docker-volume-glusterfs/
WORKDIR /go/src/docker-volume-glusterfs/glusterfs-plugin
RUN  go get -v .
RUN  go build
   
FROM ubuntu:latest
RUN apt update && \
    apt install -y software-properties-common && \
    add-apt-repository ppa:gluster/glusterfs-7 && \
    apt install -y glusterfs-client && \
    apt remove -y --purge software-properties-common && \
    apt autoremove -y && \
    rm -rf /var/lib/apt/lists/*
ADD https://github.com/krallin/tini/releases/download/v0.18.0/tini /tini
COPY --from=builder /go/src/docker-volume-glusterfs/glusterfs-plugin/glusterfs-plugin /
RUN chmod +x /tini

CMD ["/tini", "--", "glusterfs-plugin"]
