FROM golang:1.25-alpine

WORKDIR /src
RUN apk add --update git bash

COPY . .
RUN go build -o /tf-provider/terraform-provider-nagios

ENTRYPOINT ["/bin/bash"]
