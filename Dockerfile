FROM golang:1.27.1-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195 AS build
ENV GODEBUG=http2client=0
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    for attempt in 1 2 3 4 5; do \
        go mod download && break; \
        if [ "$attempt" = 5 ]; then echo "go mod download failed after 5 attempts" >&2; exit 1; fi; \
        echo "go mod download attempt $attempt failed; retrying in 5s" >&2; \
        sleep 5; \
    done
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -tags='netgo osusergo' -trimpath \
        -ldflags='-s -w -linkmode external -extldflags -static' \
        -o /out/grepple ./cmd/grepple

FROM alpine/git:v2.54.0@sha256:d301ddc314bb6531726d37fbd435b5d736296ad0f77e54246ae78ef74031729d
RUN apk update && apk upgrade --no-cache && rm -rf /var/cache/apk/* \
    && addgroup -g 10001 -S grepple \
    && adduser -u 10001 -S -D -G grepple -h /home/grepple -s /sbin/nologin grepple
COPY --from=build /out/grepple /usr/local/bin/grepple
USER grepple
ENTRYPOINT ["grepple"]
CMD ["--help"]
