FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/hyrouter ./cmd/hyrouter

FROM alpine:3.23

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=build /out/hyrouter /usr/local/bin/hyrouter
COPY config.yaml /app/config.yaml

EXPOSE 5520/udp

ENTRYPOINT ["/usr/local/bin/hyrouter"]
CMD ["-config", "/app/config.yaml"]
