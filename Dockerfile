FROM golang:1.27-alpine

WORKDIR /src
RUN apk add --no-cache git bash

COPY . .
RUN go build -o /tf-provider/terraform-provider-nagios

# Drop root for the interactive shell below - the build above still runs as
# root (needed to write into /src and /tf-provider), but there's no reason
# for the dev shell someone lands in via `docker run` to default to root.
RUN adduser -D -u 1000 appuser
USER appuser

ENTRYPOINT ["/bin/bash"]
