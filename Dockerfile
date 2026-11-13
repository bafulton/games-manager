# The manager is cross-compiled before the image is built (make
# build-linux), where Go's build cache applies, so building the image only
# copies the binary for the target platform in. No Go toolchain or emulation
# is needed in the build.
FROM gcr.io/distroless/static:nonroot
ARG TARGETARCH
LABEL org.opencontainers.image.licenses="Apache-2.0"
WORKDIR /
COPY bin/linux-${TARGETARCH}/manager /manager
COPY LICENSE /licenses/LICENSE
USER 65532:65532

ENTRYPOINT ["/manager"]
