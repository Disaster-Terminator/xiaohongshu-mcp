# syntax=docker/dockerfile:1
FROM golang:1.24 AS build
WORKDIR /src
COPY . .
RUN go test ./... && go vet ./...
ARG VERSION=v2.5.5-cloudops.6
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o /out/app . && \
    CGO_ENABLED=0 go test -c ./xiaohongshu -o /out/navigation.test && \
    CGO_ENABLED=0 go test -c . -o /out/ownership.test && \
    cd third_party/headless_browser && CGO_ENABLED=0 go test -c -o /out/cleanup.test

FROM xpzouying/xiaohongshu-mcp:v2.5.5@sha256:5bc690d0c7907f069886ddeb6b37abbe4bdd37bd431975b914c7c27435b1b602 AS browser-test
COPY --from=build /out/*.test /tests/
ENV BROWSER_TEST_BIN=/app/cache/xiaohongshu-mcp/browser/148.0.7778.215/chrome
ENTRYPOINT ["sh", "-ec"]
CMD ["/tests/navigation.test -test.run 'TestDocumentReadiness|TestInitialCommentsReadiness|TestSearchNavigation' -test.v -test.timeout 60s; /tests/ownership.test -test.run TestQRCodeFailure -test.v -test.timeout 30s; /tests/cleanup.test -test.v -test.timeout 40s"]

FROM xpzouying/xiaohongshu-mcp:v2.5.5@sha256:5bc690d0c7907f069886ddeb6b37abbe4bdd37bd431975b914c7c27435b1b602 AS runtime
COPY --from=build /out/app /app/app
