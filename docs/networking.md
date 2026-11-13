# Networking

A version's `spec.template.service` configures a normal Kubernetes Service.
Its `connection` describes the address players use. Neither automatically
installs an external controller, proxy, DNS record or edge forwarder.

## Direct Service connections

```yaml
service:
  type: LoadBalancer
  # Optional: select any installed provider, not just Tailscale.
  # loadBalancerClass: example.com/provider
  annotations:
    example.com/provider-setting: value
connection:
  type: Service
```

The default Service type is ClusterIP. A direct connection reports
`<server>.<namespace>.svc:<port>` for ClusterIP and waits for ingress for
LoadBalancer. For NodePort, set `connection.service.host` to a reachable
node's DNS name/IP; the address uses the allocated NodePort. An explicit
host also overrides LoadBalancer ingress discovery. `status.address` reports
the first declared port; all named TCP/UDP ports are available on the Service.
Pod readiness does not prove that external DNS/firewalls are configured.

## Hostname routing and mc-router

`connection.type: Hostname` reports
`<server>.<namespace>.<connection.hostname.domain>`. This supports a single
shared namespace and separates identically named servers in other namespaces.
DNS and your chosen protocol-aware proxy must route those hostnames.
Hostname routing is not a generic TCP/UDP capability.

For Minecraft Java, opt into `integrations.mcRouter: true`. It adds
`mc-router.itzg.me/externalServerName` to the Service. Install and expose
mc-router separately with its Kubernetes discovery enabled. See
`examples/minecraft.yaml`; it also needs DNS pointing at the router.

## Port pools and optional Tailscale exposure

A PortPool reserves a slot cluster-wide within one Game, including across
its versions and namespaces. Slot n advertises `basePort + n * stride` on
`publicHost`. Stopped servers retain slots. Deleted servers release them.
New servers retry exhausted pools; duplicate or out-of-range explicit slots
report `SlotConflict` without silently changing the requested address.

An externally configured forwarder must map those public ports to the
Service's container ports. The manager never configures the forwarder.
Keep pool layouts consistent between versions when keeping public addresses.

For Tailscale, explicitly configure:

```yaml
service:
  type: LoadBalancer
  loadBalancerClass: tailscale
  allocateLoadBalancerNodePorts: false
integrations:
  tailscale:
    tags: tag:k8s-games # optional; otherwise use the operator's defaults
```

The installed Tailscale operator creates the proxies. Port-pool proxy names
are `<game>-slot-<n>`, matching the separately managed forwarder's targets.
Direct connections use a server name plus a stable namespace/name hash.
Tailnet tags, grants, DNS and public forwarding are administrator concerns.
`examples/valheim.yaml` demonstrates this deployment, not a requirement for
running other games.

## Changes and migration

The old alpha API inferred mc-router from Hostname and Tailscale from
PortPool. Add the explicit `service` and `integrations` settings shown above
when migrating those deployments into GameVersions. Move
`connection.portPool.tailscaleTags` to `integrations.tailscale.tags`.
Hostname addresses now include the namespace; update DNS/proxy expectations.

Kubernetes does not allow changing an existing Service's loadBalancerClass.
If a new version changes that class, stop the server and delete its Service
before starting it with that version. The controller recreates it; assigned
load balancer addresses/NodePorts may change. Ordinary container-port changes
use named targets so traffic follows the selected version's port.
