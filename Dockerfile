FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN go build -o /out/umbrel-dropbox-client ./cmd/umbrel-dropbox-client && go build -o /out/umbrel-dropbox-clientd ./cmd/umbrel-dropbox-clientd

FROM alpine:3.22
RUN apk add --no-cache ca-certificates sqlite-libs
COPY --from=build /out/umbrel-dropbox-client /usr/bin/umbrel-dropbox-client
COPY --from=build /out/umbrel-dropbox-clientd /usr/bin/umbrel-dropbox-clientd
COPY packaging/docker/umbrel-entrypoint.sh /usr/bin/umbrel-dropbox-client-umbrel-entrypoint
RUN chmod +x /usr/bin/umbrel-dropbox-client-umbrel-entrypoint
EXPOSE 8477
ENTRYPOINT ["/usr/bin/umbrel-dropbox-client-umbrel-entrypoint"]
