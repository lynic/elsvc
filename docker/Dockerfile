FROM golang:1.13.4

ENV CONFIGPATH "/opt/etc/elsvc.yaml"

COPY ./elsvc /opt/

RUN  mkdir /opt/etc/ && \
     mkdir /opt/plugins/

CMD ["/opt/elsvc"]