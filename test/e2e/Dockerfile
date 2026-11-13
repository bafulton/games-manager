# The e2e image: a manager binary compiled on the CI runner, where Go's
# build cache is warm, rather than inside a cold Docker build.
FROM gcr.io/distroless/static:nonroot
COPY manager /manager
USER 65532:65532
ENTRYPOINT ["/manager"]
