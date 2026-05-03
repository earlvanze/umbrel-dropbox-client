FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN go build -o /out/umbrel-dropbox-client ./cmd/umbrel-dropbox-client && go build -o /out/umbrel-dropbox-clientd ./cmd/umbrel-dropbox-clientd

FROM alpine:3.22
RUN adduser -D -h /home/sync sync
COPY --from=build /out/umbrel-dropbox-client /usr/bin/umbrel-dropbox-client
COPY --from=build /out/umbrel-dropbox-clientd /usr/bin/umbrel-dropbox-clientd
USER sync
EXPOSE 8477
CMD ["/usr/bin/umbrel-dropbox-client", "status", "--db", "/data/state.db"]
