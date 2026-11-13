# Roadmap

## Available in the alpha API

- Catalog, immutable GameVersions and independently versioned GameServers.
- Start/stop/restart, readiness conditions and connection addresses.
- Retained worlds, recovery by namespace/name and explicit purge.
- Ordinary Kubernetes Services; optional mc-router/Tailscale integrations
  and separately operated port-pool forwarding.
- Helm/release automation, schema drift checks, unit/envtest coverage and
  Helm/Kind workload lifecycle tests.

## Next milestones

1. Harden upgrades/recovery across representative games, storage/network
   providers and versions; publish the tested Kubernetes/architecture matrix.
2. Build the self-service UI/API with OIDC (including Pocket ID), verified
   ownership, admin/member authorization and bounded configuration. See
   [the authentication design](docs/authentication.md).
3. Add self-service limits/quotas, operational visibility, game recipes and
   application-consistent backup/restore examples.

Persistent community servers on one cluster are the first target. GitOps,
kubectl, automation and the future UI are supported management workflows.
Session matchmaking, ephemeral fleets and multi-cluster orchestration are
outside current scope. DNS/proxies, backup systems and edge forwarders are
separately operated integrations.

Use this repository's GitHub issues for public design discussions, bugs and
contributions; no private homelab tracker is required.
