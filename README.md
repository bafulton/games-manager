# games-manager

A Kubernetes controller, and soon a small web UI, for self-service game
servers. Friends create and run their own Minecraft, Valheim or Factorio
servers. The platform stays in git; the servers themselves don't.

## Resources

- **`Game`** (cluster-scoped, kept in git): how to run one game. Its image,
  command, ports, how players connect (a hostname, or a slot from a port
  pool), resource presets, storage size and the versions servers can run.
- **`GameServer`** (namespaced, created at runtime): one server of one game.
  Its owner, whether it's running, its size preset and its version.

### Versions

A Game can list `versions`, so servers of one game can run different
releases at once, eg one Minecraft world on 1.21 and another on 1.20. A
version overrides the Game's `image`, `args` and `env` (its env comes after
the Game's and before the server's); everything else, including ports,
storage and connection, is shared by every version.

A new server gets the Game's `defaultVersion`, recorded in its
`spec.version`, and keeps it: adding versions or changing the default moves
no existing server. Changing a server's `spec.version` upgrades or
downgrades that server alone, on the same world, so back the world up
first. Mark a version `deprecated` to stop offering it for new servers while
existing servers keep running it. Removing a version that servers still use
puts them in `UnknownVersion`; they keep running as they were, and can still
be stopped, until they name another version.

Editing a version in place, or the shared parts of the Game, rolls every
server that uses it. Name versions by release line (`"1.21"`) so updates
within that line reach its servers.

The controller turns each `GameServer` into:

- a **StatefulSet** with 0 or 1 replicas, from `spec.running`. Changing
  `spec.restartedAt` restarts it.
- a **PVC**, `<server>-world`, of the Game's `storage.storageClassName` (the
  cluster default if unset) and carrying its `storage.labels` and
  `storage.annotations`, eg to join a backup job. It has no owner: deleting
  a server keeps its world. Annotate the server with
  `games-manager.io/purge: "true"` before deleting it to remove the world too.
- a **Service**, shaped by the Game's `service`: its type, load balancer
  class, labels and annotations. Annotation and label values can use
  `$(SERVER)`, `$(NAMESPACE)`, `$(GAME)`, `$(SLOT)` and `$(ADDRESS)`, so the
  Service can carry whatever the cluster's proxy or load balancer needs to
  find it.

How players connect is up to the Game's `connection`:

- **`Hostname`**: each server is `<server>.<domain>`, and a proxy that reads
  the hostname from the game's protocol routes it, so every server shares
  one port. For Minecraft Java, mc-router does this; `examples/minecraft.yaml`
  sets the annotation it looks for.
- **`PortPool`**: each server gets a numbered slot, and slot `n` is
  `<publicHost>:<basePort + n * stride>`. Something outside games-manager
  forwards each slot's ports to its server's Service. The slot is assigned
  once and kept in `spec.slot`, so a restored server keeps its address.

`examples/` has a `Game` and a `GameServer` for Minecraft (Hostname) and
Valheim (PortPool). The tests check that the CRDs accept them.

## Status

Early: `v1alpha1`, and the API will change.

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
on `main`. PRs are squash-merged, so the PR title is that commit, and CI
checks it. `fix:` bumps the patch version, `feat:` the minor version, and,
before 1.0, a breaking change also bumps the minor version. Merging the PR
tags `vX.Y.Z` and publishes the image and chart to GHCR at that version. On
`main`, Release Please runs only after lint, tests, e2e and the chart check
pass.

## Installing

```sh
helm install games-manager oci://ghcr.io/bafulton/charts/games-manager \
  --version X.Y.Z --namespace games-manager --create-namespace
```

The chart installs the CRDs (from `chart/crds/`), the controller and its
RBAC. `make chart` regenerates the chart's CRDs and ClusterRole from the API;
CI fails if they're out of date.

## License

Licensed under [Apache-2.0](LICENSE).
