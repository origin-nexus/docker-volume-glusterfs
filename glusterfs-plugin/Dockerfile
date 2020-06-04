FROM golang:latest AS builder
COPY . /go/src/docker-volume-glusterfs
WORKDIR /go/src/docker-volume-glusterfs
RUN  go get .
RUN  go build
   
FROM ubuntu:latest
RUN apt update && \
    apt install -y software-properties-common && \
    add-apt-repository ppa:gluster/glusterfs-7 && \
    apt install -y glusterfs-client && \
    apt remove -y --purge software-properties-common && \
    apt autoremove -y && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /go/src/docker-volume-glusterfs/docker-volume-glusterfs /
ADD https://github.com/krallin/tini/releases/download/v0.18.0/tini /tini
RUN chmod +x /tini

CMD ["/tini", "--", "docker-volume-glusterfs"]
