# Changelog

## 0.1.0 (2026-11-13)


### ⚠ BREAKING CHANGES

* rename the API group to games-manager.io ([#24](https://github.com/bafulton/games-manager/issues/24))
* configure Services generically instead of building in mc-router and Tailscale ([#23](https://github.com/bafulton/games-manager/issues/23))
* use portable storage defaults and optional backup metadata ([#17](https://github.com/bafulton/games-manager/issues/17))

### Features

* configure Services generically instead of building in mc-router and Tailscale ([#23](https://github.com/bafulton/games-manager/issues/23)) ([294542c](https://github.com/bafulton/games-manager/commit/294542c569a2145d8221783fa6cdd657b5306036))
* connect players directly to a server's own Service ([#25](https://github.com/bafulton/games-manager/issues/25)) ([4eaacc2](https://github.com/bafulton/games-manager/commit/4eaacc21db4190f1db70759774fe85b966479ecc))
* Game and GameServer v1alpha1 API and controller ([3e610f0](https://github.com/bafulton/games-manager/commit/3e610f05eadeadfc79a2e6a8dcb9f5c2468f843f))
* Game and GameServer v1alpha1 API and controller ([056b31f](https://github.com/bafulton/games-manager/commit/056b31f6ae2bc83a9ec10a5d3b2ac606dd6f1a56))
* Helm chart, and a release workflow that publishes it to GHCR ([#2](https://github.com/bafulton/games-manager/issues/2)) ([a404bfa](https://github.com/bafulton/games-manager/commit/a404bfa6e2ff38eae9924051da6097b22749f78c))
* let servers of one game run different versions ([#22](https://github.com/bafulton/games-manager/issues/22)) ([901b787](https://github.com/bafulton/games-manager/commit/901b787b413cfd5a544650859a1842fce5df1732))
* nodeSelector, affinity and tolerations on Game ([#12](https://github.com/bafulton/games-manager/issues/12)) ([04cdbc3](https://github.com/bafulton/games-manager/commit/04cdbc383a62383c1692a5815de7a2bb4257255a))
* rename the API group to games-manager.io ([#24](https://github.com/bafulton/games-manager/issues/24)) ([8417052](https://github.com/bafulton/games-manager/commit/8417052189d68a8bf3e061a2621c9ce83e6b96e8))
* use portable storage defaults and optional backup metadata ([#17](https://github.com/bafulton/games-manager/issues/17)) ([723c7cb](https://github.com/bafulton/games-manager/commit/723c7cb212bbaa4b3497ea8e78cb39f57be585e0))


### Bug Fixes

* honor stop requests when game definitions are unavailable ([#13](https://github.com/bafulton/games-manager/issues/13)) ([d08ca2a](https://github.com/bafulton/games-manager/commit/d08ca2a15f765070c972291db5d4a18d522f0a8b))
* report hostname clashes between namespaces ([#15](https://github.com/bafulton/games-manager/issues/15)) ([7cf289c](https://github.com/bafulton/games-manager/commit/7cf289c3ae27fad60b53c1dca2dc2f76687a7db9))
* route Services through named container ports ([#16](https://github.com/bafulton/games-manager/issues/16)) ([d70de58](https://github.com/bafulton/games-manager/commit/d70de58d916c52dc485e62425c6154d1e9f0a463))
* validate restored slots and retry exhausted pools ([#14](https://github.com/bafulton/games-manager/issues/14)) ([38156e6](https://github.com/bafulton/games-manager/commit/38156e6d76cd0f770c553dbdfd0f58fd9d00dd2e))
