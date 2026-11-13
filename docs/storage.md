# World storage

Running servers use `<server>-world` in their namespace. Claims have no
GameServer owner reference; deleting a server retains its world. Servers
created stopped allocate nothing until started.

Omit `storage.storageClassName` for the cluster default, select a named
provider class, or explicitly set `""` to disable dynamic provisioning.
New claims use ReadWriteOnce and the recipe's size. Existing claims are
reused unchanged: version changes do not resize/reclassify or relabel them.

## Recovery and purge

Recreate a server with the same namespace/name and intended game version to
reuse its world. For PortPool, restore `spec.slot` too. Occupied/out-of-range
slots report conflicts when starting rather than silently moving the address.
Reuse a namespace/name only when intending to attach that world; `owner`
metadata is not a storage authorization boundary.

To delete the server and request deletion of its world:

```sh
kubectl annotate gameserver alice -n games games.homelab.io/purge=true
kubectl delete gameserver alice -n games
```

Kubernetes volume protection/provider cleanup determine when deletion
finishes. The PV's reclaim policy determines whether underlying storage is
deleted. Namespace deletion can delete retained claims too. Retention is not
a backup.

## Optional backup integration

Backup jobs, snapshots and repositories are managed separately. Configure
new claim labels/annotations for your provider. For an existing Longhorn
recurring-job group named `games`, opt in with:

```yaml
storage:
  size: 10Gi
  mountPath: /data
  storageClassName: config # example: use your actual class
  labels:
    recurring-job.longhorn.io/source: enabled
    recurring-job-group.longhorn.io/games: enabled
```

The class/group must already exist; labels do not create them. Existing
claims require an explicit metadata update. Use the game's save/quiesce
procedure when needed and test restores. Shutdown hooks do not automatically
quiesce games for an external backup job.
