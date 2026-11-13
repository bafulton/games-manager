# Contributing

Use GitHub issues for bugs, game recipes and design discussions. Include
versions, redacted resources, expected/actual behavior and reproduction steps.
Follow [SECURITY.md](SECURITY.md) for sensitive reports. API PRs should explain
their migration and world/connection lifecycle impact.

Use the Go version in `go.mod`. Make installs tools under `bin/`; envtest
uses an isolated API server, not your Kubernetes cluster.

```sh
make generate manifests chart
make test
make lint
helm lint chart
helm template games-manager chart --include-crds > /tmp/games-manager.yaml
git diff --check
```

Commit generated Go, CRDs and chart/RBAC changes together. `make chart` also
copies LICENSE into the chart. CI checks chart drift and builds amd64/arm64
images. Example resources are schema-validated; add behavior-focused
regression tests for controller changes.

## E2E

Install Docker with a running daemon, Kind, Helm and kubectl, then run
`make test-e2e`. It creates a dedicated Kind cluster and installs the Helm
chart, checking real pods, persistence, explicit upgrades and lifecycle.
The default cluster name is `games-manager-test-e2e`; clean up a failed run
with `make cleanup-test-e2e`. This is a workstation/CI test, not a production
cluster check.

Use conventional PR titles (`fix:`, `feat:`, `docs:`, `chore:`); append `!`
for breaking changes. PRs are squash-merged and Release Please selects
versions from those commits. Keep changes focused and identify stack
dependencies. Contributions use the project's Apache-2.0 license.
