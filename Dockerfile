FROM --platform=${BUILDPLATFORM} golang:alpine as compiler
ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0

WORKDIR /go/src/s3dir

COPY . .

RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /bin/s3dir github.com/relvacode/s3dir

FROM --platform=${TARGETPLATFORM} alpine
ENV LISTEN_ADDRESS=0.0.0.0:80
COPY --from=compiler /bin/s3dir /bin/s3dri

ENTRYPOINT ["/bin/s3dir"]
