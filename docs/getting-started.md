# Installation and a first server

Use a Kubernetes cluster with a working dynamic provisioner/default
StorageClass, kubectl, and Helm with OCI support. Examples need Linux nodes
and sufficient memory. Kind CI currently tests Kubernetes 1.33; compatibility
with every Kubernetes release is not yet established.

## Install

Select a published version from [Releases](https://github.com/bafulton/games-manager/releases).
Before the first release, build/install from a checkout as described below.
The version here is an example; use chart and examples from the same release.

```sh
VERSION=0.1.0
helm show crds oci://ghcr.io/bafulton/charts/games-manager --version "$VERSION" > games-manager-crds.yaml
kubectl apply --server-side --field-manager=games-manager-crds -f games-manager-crds.yaml
helm upgrade --install games-manager oci://ghcr.io/bafulton/charts/games-manager \
  --version "$VERSION" --namespace games-manager --create-namespace --skip-crds --wait
```

CRDs are applied explicitly for clear upgrade ownership. For an unreleased
checkout, build/push an image to a registry your cluster can reach:

```sh
make docker-build docker-push IMG=YOUR_REGISTRY/games-manager:dev
kubectl apply --server-side --field-manager=games-manager-crds -f chart/crds/
helm upgrade --install games-manager ./chart -n games-manager --create-namespace \
  --skip-crds --set image.repository=YOUR_REGISTRY/games-manager --set image.tag=dev --wait
```

## First server without integrations

From the matching release's checkout:

```sh
kubectl create namespace games
kubectl apply -f examples/minecraft-direct.yaml
kubectl wait -n games --for=condition=Ready gameserver/alice --timeout=10m
kubectl get gameserver -n games
kubectl port-forward -n games service/alice 25565:25565
```

Connect Minecraft Java to `localhost:25565` while forwarding is running.
This uses ClusterIP and the default StorageClass, without public DNS,
load balancers or proxies. The example accepts Minecraft's EULA through its
container configuration; use it only if you accept those terms. The engine
version is pinned separately from the container image.

If readiness stalls, inspect `kubectl describe gameserver alice -n games`,
`kubectl describe pod alice-0 -n games`, `kubectl logs alice-0 -n games` and
PVCs/events. Check storage provisioning, image pulls, free memory, architecture
and game startup time. A custom StorageClass is configured in the version recipe.

## Stop, start and restart

```sh
kubectl patch gameserver alice -n games --type merge -p '{"spec":{"running":false}}'
kubectl patch gameserver alice -n games --type merge -p '{"spec":{"running":true}}'
kubectl patch gameserver alice -n games --type merge \
  -p "{\"spec\":{\"restartedAt\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}}"
```

Stopping scales to zero and retains the world/slot, even if the selected
catalog/version is missing. See [versions](versions.md) for upgrades and
[storage](storage.md) for recovery/purge.

## Optional public/LAN exposure

Follow [networking](networking.md). `examples/minecraft.yaml` needs an
installed/exposed mc-router and DNS for `*.games.mc.example.com` (adjust the
domain). `examples/valheim.yaml` needs the Tailscale operator, tailnet
permissions, DNS and an independently configured edge forwarder.

Valheim's referenced Secret must exist before its server starts:

```sh
kubectl create secret generic bob-valheim -n games \
  --from-file=password=/path/to/a/password-file
```

Use a password meeting the game's requirements. Secret references resolve in
the server's namespace. Examples do not install external dependencies or DNS.
