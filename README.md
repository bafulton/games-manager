# games-manager

A Kubernetes controller, and soon a small web UI, for self-service game
servers. Friends create and run their own Minecraft, Valheim or Factorio
servers. The platform stays in git; the servers themselves don't.

## Resources

- **`Game`** (cluster-scoped, kept in git): how to run one game. Its image,
  command, ports, how players connect (a hostname, or a slot from a port
  pool), resource presets and storage size.
- **`GameServer`** (namespaced, created at runtime): one server of one game.
  Its owner, whether it's running, and its size preset.

The controller turns each `GameServer` into:

- a **StatefulSet** with 0 or 1 replicas, from `spec.running`. Changing
  `spec.restartedAt` restarts it.
- a **PVC**, `<server>-world`, labeled into the Longhorn `games` recurring-job
  group so backup jobs for that group cover every server. It has no owner:
  deleting a server keeps its world. Annotate the server with
  `games.homelab.io/purge: "true"` before deleting it to remove the world too.
- a **Service**. A `Hostname` game gets a ClusterIP that mc-router discovers
  by annotation. A `PortPool` game gets a Tailscale LoadBalancer named
  `<game>-slot-<n>`, which the edge forwarder routes the slot's public ports
  to. The slot is assigned once and kept in `spec.slot`, so a restored server
  keeps its address.

`examples/` has a `Game` and a `GameServer` for Minecraft (Hostname) and
Valheim (PortPool). The tests check that the CRDs accept them.

## Status

Early: `v1alpha1`, and the API will change. Design and roadmap are tracked in
the homelab project (CAT-84). It's built for one cluster first, so it assumes
Longhorn for storage and the Tailscale operator for exposing servers.

## Development

```sh
make generate manifests   # regenerate deepcopy code and CRDs
make test                 # unit and envtest tests
make build
```

Scaffolded with kubebuilder v4.6.0.

## Releasing

Release Please keeps a release PR open with the next version and changelog,
worked out from [conventional commits](https://www.conventionalcommits.org)
on `main`: `fix:` bumps the patch version, `feat:` the minor version, and,
before 1.0, a breaking change also bumps the minor version. Merging the PR
tags `vX.Y.Z` and publishes the image and chart to GHCR at that version.

## Installing

```sh
helm install games-manager oci://ghcr.io/bafulton/charts/games-manager \
  --version X.Y.Z --namespace games-manager --create-namespace
```

The chart installs the CRDs (from `chart/crds/`), the controller and its
RBAC. `make chart` regenerates the chart's CRDs and ClusterRole from the API;
CI fails if they're out of date.
