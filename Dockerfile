FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN go build -o /out/umbrel-dropbox-sync ./cmd/umbrel-dropbox-sync && go build -o /out/umbrel-dropbox-syncd ./cmd/umbrel-dropbox-syncd

FROM alpine:3.22
RUN adduser -D -h /home/sync sync
COPY --from=build /out/umbrel-dropbox-sync /usr/bin/umbrel-dropbox-sync
COPY --from=build /out/umbrel-dropbox-syncd /usr/bin/umbrel-dropbox-syncd
USER sync
EXPOSE 8477
CMD ["/usr/bin/umbrel-dropbox-sync", "status", "--db", "/data/state.db"]
