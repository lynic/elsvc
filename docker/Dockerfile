FROM golang:1.13.4-alpine

ENV CONFIGPATH "/opt/etc/elsvc.yaml"

COPY ./elsvc /opt/

RUN  mkdir /opt/etc/ && \
     mkdir /opt/plugins/

WORKDIR /opt/

CMD ["/opt/elsvc"]