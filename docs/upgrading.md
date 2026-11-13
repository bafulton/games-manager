# Controller and CRD upgrades

Read the target release notes, export custom resources and back up worlds.
The alpha API can change incompatibly. The original Game-to-GameVersion
change has a [specific migration guide](versions.md); migrate before starting
the new controller because old Game spec fields leave the catalog schema.

Helm installs CRDs from `crds/` but does **not** upgrade/delete them on ordinary
upgrades. See [Helm's documentation](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/).
CI checks that chart schemas match the API, but users must apply them explicitly.

```sh
VERSION=0.1.0 # replace with the target published release
helm show crds oci://ghcr.io/bafulton/charts/games-manager --version "$VERSION" > games-manager-crds.yaml
# Review differences first; kubectl diff exits 1 when differences exist.
kubectl diff --server-side --field-manager=games-manager-crds -f games-manager-crds.yaml
kubectl apply --server-side --field-manager=games-manager-crds -f games-manager-crds.yaml
helm upgrade games-manager oci://ghcr.io/bafulton/charts/games-manager \
  --version "$VERSION" -n games-manager --wait
```

For installations created with Helm-owned CRDs/another GitOps manager,
resolve field-ownership conflicts deliberately, rather than automatically
forcing ownership away. Designate one CRD owner and align schema/controller
versions. For unreleased builds, apply `chart/crds/` from the image's commit.

Do not delete/recreate CRDs to upgrade: deletion removes their custom
resources and can trigger workload garbage collection. Helm rollback does
not roll back schemas or game world formats. Use release-specific migration
or restore instructions when a rollback requires either.
