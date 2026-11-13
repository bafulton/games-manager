# Tenancy, ownership and the planned UI

## Current behavior

One shared `games` namespace suits trusted friends. Separate namespaces are
optional for routing: hostname addresses include the namespace and slots
are reserved across namespaces. The operator creates no network-isolation
policies.

Direct API access uses Kubernetes RBAC. Only trusted administrators should
manage cluster-scoped Games/GameVersions: recipes select images, commands,
security contexts and Secret references. Namespace RBAC can grant GameServer
access, but does not restrict objects based on `spec.owner`. A user with
broad permissions in a shared namespace can edit/purge other users' servers.
Use separate namespaces/RBAC when direct API users need that boundary.

`owner` is descriptive, not a verified identity or authorization decision.
There is no UI/OIDC implementation yet. Namespace-local Secret references
also require trusted users or a future UI that limits which secrets can be used.

## Planned OIDC UI

OIDC integration, including Pocket ID, is in scope. The UI backend must
authorize reads, logs, edits, restarts, upgrades, ownership transfers and
deletion/purge. Browser controls are not an authorization boundary; browsers
must not receive the backend's Kubernetes service account token.

Acceptance criteria:

- Configurable HTTPS issuer discovery, client ID, exact redirect URI and
  a client secret sourced from a Kubernetes Secret.
- Authorization Code flow with PKCE and state/nonce validation. A maintained
  library verifies signatures, issuer, audience and expiry.
- Server-managed sessions with Secure/HttpOnly cookies and CSRF protection.
- Stable ownership using the issuer and `sub`; usernames/emails are display
  attributes, not durable keys.
- Configured admin/member mappings from validated subject/group claims;
  unmatched identities are denied. Untrusted headers/user-supplied owner
  values must not grant access.
- Admin-managed catalog/versions, independently selected server upgrades,
  and bounded self-service configuration/Secret references. Ownership
  transfers and purge have explicit authorization checks.

These are planned requirements, not Helm settings available today. Provider
configuration and integration tests ship with the UI. The foundation is
[OpenID Connect Core](https://openid.net/specs/openid-connect-core-1_0.html).
Public edge-forwarder management remains outside the project.
