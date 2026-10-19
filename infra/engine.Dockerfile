FROM golang:1.26-alpine AS build
WORKDIR /src
COPY apps/engine/go.mod apps/engine/go.sum ./
RUN go mod download
COPY apps/engine/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/engine ./cmd/engine
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/credentials ./cmd/credentials
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/snapshot-transition ./cmd/snapshot-transition

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 terminal
COPY --from=build /out/engine /usr/local/bin/engine
COPY --from=build /out/migrate /usr/local/bin/migrate
COPY --from=build /out/credentials /usr/local/bin/credentials
COPY --from=build /out/snapshot-transition /usr/local/bin/snapshot-transition
COPY database/migrations/ /opt/terminal/migrations/
USER terminal
ENV ENGINE_ADDRESS=[::]:8080 MIGRATIONS_DIR=/opt/terminal/migrations
EXPOSE 8080
CMD ["/usr/local/bin/engine"]
