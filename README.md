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

The controller turns each `GameServer` into a StatefulSet, a PVC and a
Service. The PVC is not owned by the `GameServer`, so deleting a server keeps
its world until it's explicitly purged.

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
