FROM golang:1.25.13-bookworm@sha256:e401dae1bf814e29204a8cb7915682e1780951e609ca0dd8865ee1937f510c48 AS build
ARG ZOEKT_VERSION=v0.0.0-20260814112500-b0de0bb820f5
# Force the module client onto HTTP/1.1. proxy.golang.org intermittently resets
# HTTP/2 streams ("INTERNAL_ERROR; received from peer"), aborting downloads
# mid-build; HTTP/1.1 plus the retry loops below make module fetches reliable.
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
WORKDIR /tmp/zoektbuild
# CVE-2026-71556 / CVE-2026-71557: zoekt's own go.mod still requires go-git
# v5.19.1; force the fixed v5.19.2 here without a `go mod tidy` (tidy drops this
# override since the throwaway module has no source files that import go-git).
# Remove this replace once zoekt's own go.mod requires go-git >= v5.19.2.
# CVE-2026-45404: opentelemetry-go's OpenTracing bridge has an unsynchronized
# baggage map that can panic under concurrent access, fixed in v1.45.0. zoekt's
# go.mod still pins v1.43.0; force the fix the same way. Remove once zoekt's own
# go.mod requires otel/bridge/opentracing >= v1.45.0.
# CVE-2026-56854: golang.org/x/crypto/ssh before v0.55.0 can bypass source-address
# restrictions (authentication bypass). zoekt's go.mod still pulls v0.54.0; force
# the fixed v0.55.0. Remove once zoekt's own go.mod requires x/crypto >= v0.55.0.
# CVE-2026-84304: gRPC-Go before v1.83.1 is flagged HIGH (trivy); zoekt's go.mod
# still pulls v1.82.1 into both zoekt binaries. Force the fixed v1.83.1 the same
# way. Remove once zoekt's own go.mod requires grpc >= v1.83.1.
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	mkdir -p /out \
	&& go mod init zoektbuild \
	&& go mod edit -replace github.com/go-git/go-git/v5=github.com/go-git/go-git/v5@v5.19.2 \
	&& go mod edit -replace go.opentelemetry.io/otel/bridge/opentracing=go.opentelemetry.io/otel/bridge/opentracing@v1.45.0 \
	&& go mod edit -replace golang.org/x/crypto=golang.org/x/crypto@v0.55.0 \
	&& go mod edit -replace google.golang.org/grpc=google.golang.org/grpc@v1.83.1 \
	&& export CGO_ENABLED=0 GOFLAGS=-mod=mod GOBIN=/out \
	&& for attempt in 1 2 3 4 5; do \
		if go get github.com/sourcegraph/zoekt@${ZOEKT_VERSION} \
			&& go install -trimpath -ldflags='-s -w' \
				github.com/sourcegraph/zoekt/cmd/zoekt-git-index \
				github.com/sourcegraph/zoekt/cmd/zoekt-webserver; \
		then break; fi; \
		if [ "$attempt" = 5 ]; then echo "zoekt build failed after 5 attempts" >&2; exit 1; fi; \
		echo "zoekt build attempt $attempt failed; retrying in 5s" >&2; \
		sleep 5; \
	done
WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    mkdir -p /out \
    && CGO_ENABLED=1 go build -tags='netgo osusergo' -trimpath -ldflags='-s -w -linkmode external -extldflags -static' -o /out/grepple ./cmd/grepple \
    && CGO_ENABLED=1 go build -tags='netgo osusergo' -trimpath -ldflags='-s -w -linkmode external -extldflags -static' -o /out/shard ./cmd/shard \
    && CGO_ENABLED=1 go build -tags='netgo osusergo' -trimpath -ldflags='-s -w -linkmode external -extldflags -static' -o /out/router ./cmd/router

FROM alpine/git:v2.54.0@sha256:d301ddc314bb6531726d37fbd435b5d736296ad0f77e54246ae78ef74031729d
RUN apk update && apk upgrade --no-cache && rm -rf /var/cache/apk/*
RUN addgroup -g 10001 -S grepple \
    && adduser -u 10001 -S -D -G grepple -h /home/grepple -s /sbin/nologin grepple \
    && mkdir -p /workspace /zoekt-index \
    && chown -R grepple:grepple /workspace /zoekt-index /home/grepple
COPY --from=build /out/grepple /usr/local/bin/grepple
COPY --from=build /out/shard /usr/local/bin/shard
COPY --from=build /out/router /usr/local/bin/router
COPY --from=build /out/zoekt-git-index /usr/local/bin/zoekt-git-index
COPY --from=build /out/zoekt-webserver /usr/local/bin/zoekt-webserver
EXPOSE 8080 8787
USER grepple
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/health || wget -q -O /dev/null http://127.0.0.1:8787/health || exit 1
ENTRYPOINT ["grepple"]
CMD ["--help"]
