# syntax=docker/dockerfile:1

# Build a multi-platform image with:
#   docker buildx build --platform linux/amd64,linux/arm64 \
#     --build-arg VERSION=0.0.0 -t mcnull/sshex:latest .
#
# Run:
#   docker run --rm -it \
#     -v "$HOME/.ssh:/home/sshex/.ssh:ro" \
#     -v "$HOME/.config/sshex:/home/sshex/.config/sshex" \
#     mcnull/sshex exec --help
#
# sshex shells out to the system `ssh` client, so the runtime image includes
# openssh-client. ca-certificates is needed for release/update checks.

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
	-trimpath -tags osusergo,netgo \
	-ldflags "-s -w -buildid= -X github.com/mcnull/sshex/internal/assets.version=${VERSION}" \
	-o /out/sshex .

FROM alpine:3.22

RUN apk add --no-cache ca-certificates openssh-client \
	&& adduser -D -u 1000 sshex

COPY --from=build /out/sshex /usr/local/bin/sshex

USER 1000:1000
WORKDIR /home/sshex

ENTRYPOINT ["sshex"]
