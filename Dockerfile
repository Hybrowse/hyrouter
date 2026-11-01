FROM golang:1.27.3-alpine3.24@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS build

WORKDIR /src

ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY config.yaml ./

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -buildvcs=false -trimpath -ldflags "-s -w" -o /out/hyrouter ./cmd/hyrouter

FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=build /out/hyrouter /usr/local/bin/hyrouter
COPY config.yaml /app/config.yaml

RUN mkdir -p /usr/share/licenses/hybrowse-hyrouter
COPY LICENSE NOTICE LICENSING.md COMMERCIAL_LICENSE.md TRADEMARKS.md /usr/share/licenses/hybrowse-hyrouter/

EXPOSE 5520/udp

ENTRYPOINT ["/usr/local/bin/hyrouter"]
CMD ["-config", "/app/config.yaml"]
