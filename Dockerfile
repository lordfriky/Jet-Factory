FROM ubuntu:19.10
ARG DEBIAN_FRONTEND=noninteractive

RUN apt update -y && apt upgrade -y && apt install -y software-properties-common
RUN add-apt-repository ppa:longsleep/golang-backports -y
RUN apt update -y && apt install -y qemu qemu-user-static arch-install-scripts linux-image-generic golang-go libguestfs-tools libguestfs-dev

WORKDIR /root/
ADD ./*.go /root/
ADD ./go.* /root/
ADD ./*.json /root/
RUN go build

VOLUME [ "/root/linux", "/root/android" ]
ENTRYPOINT [ "/root/jetfactory" ]