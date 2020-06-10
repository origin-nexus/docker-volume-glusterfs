FROM golang:latest AS builder
COPY glusterfs-volume /go/src/docker-volume-glusterfs/glusterfs-volume/
COPY gluster-block-file-plugin /go/src/docker-volume-glusterfs/gluster-block-file-plugin/
COPY go.mod go.sum /go/src/docker-volume-glusterfs/
WORKDIR /go/src/docker-volume-glusterfs/gluster-block-file-plugin
RUN  go get -v .
RUN  go build
   
FROM ubuntu:latest
RUN apt update && \
    apt install -y software-properties-common && \
    add-apt-repository ppa:gluster/glusterfs-7 && \
    apt install -y glusterfs-client btrfs-progs exfat-utils f2fs-tools dosfstools hfsprogs jfsutils nilfs-tools ntfs-3g reiser4progs reiserfsprogs xfsprogs && \
    apt remove -y --purge software-properties-common && \
    apt autoremove -y && \
    rm -rf /var/lib/apt/lists/*
ADD https://github.com/krallin/tini/releases/download/v0.18.0/tini /tini
COPY --from=builder /go/src/docker-volume-glusterfs/gluster-block-file-plugin/gluster-block-file-plugin /
RUN chmod +x /tini

CMD ["/tini", "--", "gluster-block-file-plugin"]
