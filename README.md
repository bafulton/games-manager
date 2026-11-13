# games-manager

A Kubernetes operator for persistent, self-service game servers. Publish an
administrator-approved catalog of games and immutable versions; manage
individual servers using Kubernetes resources, GitOps or a future UI.

- **Game**: a cluster-scoped catalog entry.
- **GameVersion**: an immutable, cluster-scoped deployment recipe belonging
  to a Game. Different servers can select different versions.
- **GameServer**: a namespaced instance with an explicit version, resource
  preset, running state and optional owner metadata.

Running servers get a StatefulSet, a retained world PVC and a Service.
Start/stop, restarts, explicit upgrades, recovery and opt-in purge are
implemented. Ordinary ClusterIP, NodePort and LoadBalancer Services work
without Longhorn, Tailscale or mc-router; those integrations are optional.

This is an early **v1alpha1** API with breaking changes. The UI and OIDC
support, including Pocket ID integration, are **planned**, not implemented.
The first target is persistent community servers on one cluster. Session
matchmaking, ephemeral fleets and multi-cluster orchestration are outside
the current scope. Public edge forwarders are managed separately.

## Guides

- [Installation and first server](docs/getting-started.md)
- [Version selection and alpha migration](docs/versions.md)
- [Networking and optional integrations](docs/networking.md)
- [World retention, storage and backups](docs/storage.md)
- [Controller and CRD upgrades](docs/upgrading.md)
- [Tenancy and planned OIDC UI](docs/authentication.md)
- [Public roadmap](ROADMAP.md), [contributing](CONTRIBUTING.md) and
  [security reporting](SECURITY.md)

A shared namespace works for trusted friends. Multiple namespaces also work:
hostnames include the namespace and slots are reserved across namespaces.
The owner field alone does not restrict API access.

## Development

```sh
make generate manifests chart
make test
make lint
make build
```

Unit/envtest checks cover validation, generated resources, slots, version
selection and world lifecycle. The Helm/Kind e2e suite exercises real pods,
explicit upgrades, stop/start, recovery and purge. See CONTRIBUTING.md.

## Releases

Release Please opens version/changelog PRs from conventional commits. PRs
are squash-merged, so titles follow that format. Checks precede publishing
the image and OCI chart to GHCR. Before 1.0, breaking changes increment the
minor version. Design, roadmap and compatibility information live here so
contributors do not need access to a private homelab tracker.

## License

Licensed under [Apache-2.0](LICENSE).
