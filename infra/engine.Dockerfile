FROM golang:1.25-alpine AS build
WORKDIR /src
COPY apps/engine/go.mod apps/engine/go.sum ./
RUN go mod download
COPY apps/engine/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/engine ./cmd/engine

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 terminal
COPY --from=build /out/engine /usr/local/bin/engine
USER terminal
ENV ENGINE_ADDRESS=0.0.0.0:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/engine"]
