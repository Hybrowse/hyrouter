FROM golang:1.25-alpine AS build

WORKDIR /src

ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY config.yaml ./

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -buildvcs=false -trimpath -ldflags "-s -w" -o /out/hyrouter ./cmd/hyrouter

FROM alpine:3.23

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=build /out/hyrouter /usr/local/bin/hyrouter
COPY config.yaml /app/config.yaml

RUN mkdir -p /usr/share/licenses/hybrowse-hyrouter
COPY LICENSE NOTICE LICENSING.md COMMERCIAL_LICENSE.md TRADEMARKS.md /usr/share/licenses/hybrowse-hyrouter/

EXPOSE 5520/udp

ENTRYPOINT ["/usr/local/bin/hyrouter"]
CMD ["-config", "/app/config.yaml"]
