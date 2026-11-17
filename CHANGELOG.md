# Changelog

## Unreleased

### Fixed

- Default `settings.transport` to `nethernet` (BDS ≥1.26.60 rejects `raknet` for joins).
- Derive `SERVER_UDP_PORTS` (VIP form) and expose UDP `19140…19140+maxPlayers-1` on the Service.
- Map `settings.onlineMode` → `ONLINE_MODE` for LAN mcpelauncher testing.

### Docs

- Pages / CLIENT / ARCHITECTURE / BEDROCK_SERVER: NetherNet join path, direct VIP vs Envoy.

### Added

- Phase 1 MVP: `BedrockServer` CRD, StatefulSet/PVC/Gateway reconcile, EULA gate,
  typed settings→env mapping, Mojang version-feed watcher, envtest coverage, docs.
