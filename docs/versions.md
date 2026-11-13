# Game versions

`Game` is the administrator-managed catalog entry. `GameVersion` is a
cluster-scoped, immutable deployment recipe for that game: the image, engine
configuration, ports, resource presets, storage and exposure settings.
`GameServer.spec.game` identifies the catalog entry and cannot change;
`spec.gameVersion` selects one of its recipes and can change independently
for each server. There is no floating "latest" version.

Publish a new GameVersion under a new name to update a recipe. Existing
servers keep their selected versions. Use image digests where possible and
pin the engine version in the container's configuration too: an immutable
recipe cannot prevent an upstream mutable tag or automatic game updater
from downloading a different binary.

To upgrade one server, first back up its world using your storage provider,
check game/mod compatibility and the new version's mount path, resource
presets and connection settings, then explicitly select the new recipe:

```sh
kubectl patch gameserver alice -n games --type merge \
  -p '{"spec":{"gameVersion":"minecraft-v2"}}'
```

The StatefulSet rolls to the selected version. Its existing world PVC and
port-pool slot are retained. Changing storage size/class or labels in a new
recipe does not modify an existing PVC. Keep pool layouts consistent across
versions when preserving public addresses. A version switch is not a world
migration or a backup; downgrading the image might not undo a world upgrade.

Only administrators should publish or delete GameVersions. Do not delete
versions still referenced by GameServers, including stopped servers. A
missing version prevents starting/reconfiguring a server, but stopping and
deleting it remain available.

## Migrating the original alpha API

This is a breaking alpha change. Before applying the new CRDs, export the
old objects and back up worlds:

```sh
kubectl get games -o yaml > games-before-versions.yaml
kubectl get gameservers -A -o yaml > servers-before-versions.yaml
```

Pause the old controller and any automatic controller upgrades using your
deployment tool during migration. Existing game pods can keep running while
the controller is paused. Do not restart the controller until references and
schemas have been migrated.

For each exported Game, create a GameVersion manifest whose `spec.game` is
the old Game name and whose `spec.template` is the old Game spec. Leave the
Game as a catalog entry with an optional `spec.description`. Add the chosen
`spec.gameVersion` to each server manifest, preserving its namespace, name,
game and slot. Do not delete/recreate the servers or PVCs to migrate.

Apply the new CRDs, catalog entries, GameVersions and updated GameServers
before starting the new controller. See the installation/upgrade guide for
managing CRDs explicitly; Helm's `crds/` directory is install-only.
