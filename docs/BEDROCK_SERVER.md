# BedrockServer

Deploy a Minecraft Bedrock Dedicated Server (BDS) with a `BedrockServer` custom resource.

## License & EULA

This operator **does not** bundle or redistribute Mojang software. The default image
[`itzg/minecraft-bedrock-server`](https://hub.docker.com/r/itzg/minecraft-bedrock-server)
downloads BDS from Mojang at container startup. You must accept the Minecraft EULA:

```yaml
spec:
  eula:
    accepted: true
```

If `accepted` is not `true`, reconcile sets `status.phase=Failed` and creates **no**
child resources (same hard gate as the image’s `EULA=TRUE` requirement).

## Quick start

```bash
kubectl apply -f config/samples/bedrock_v1alpha1_bedrockserver.yaml
kubectl get bedrockservers -o wide
```

Connect clients to `status.connectionAddress`:`status.connectionPort` (default UDP 19132).
Example: `192.168.14.190:19132`. How to obtain Bedrock on Windows, console, mobile, and
Linux (mcpelauncher): [CLIENT.md](CLIENT.md).

**LAN discovery** (Friends tab / local network browse) does not work through pod
networking — UDP broadcasts are not forwarded. Use the VIP:port directly, enable
`spec.hostNetworkDiscovery: true` (trade-offs: host ports, weaker isolation), or run a
BedrockConnect companion later.

## Spec overview

| Field | Purpose |
|-------|---------|
| `eula.accepted` | Required Mojang EULA gate |
| `serverImage.image` | Default `itzg/minecraft-bedrock-server:latest` |
| `version.mode` | `LATEST` \| `PREVIEW` \| `PREVIOUS` \| `PINNED` |
| `version.pinned` | Exact version when `mode=PINNED` (e.g. `1.21.93.05`) |
| `version.directDownloadUrl` | Bypass Mojang lookup (air-gapped / pinned zip) |
| `settings.*` | Typed `server.properties` → itzg env vars |
| `persistence` | PVC size / storageClass / `existingClaim` for DR |
| `update.checkInterval` | Mojang feed poll interval (default `6h`) |
| `update.notifyOnly` | Default `true` — status only; `false` may patch download URL |
| `gateway` | Envoy Gateway VIP or Service LoadBalancer fallback |
| `hostNetworkDiscovery` | Opt-in `hostNetwork` for LAN discovery |
| `mods` / `adminUI` | Phase 2 / Phase 3 stubs |

### Settings → env

| Spec | Env |
|------|-----|
| `settings.levelName` | `LEVEL_NAME` |
| `settings.levelSeed` | `LEVEL_SEED` |
| `settings.levelType` | `LEVEL_TYPE` |
| `settings.gameMode` | `GAMEMODE` |
| `settings.difficulty` | `DIFFICULTY` |
| `settings.maxPlayers` | `MAX_PLAYERS` |
| `settings.allowCheats` | `ALLOW_CHEATS` |
| `settings.defaultPlayerPermissionLevel` | `DEFAULT_PLAYER_PERMISSION_LEVEL` |
| `settings.whiteList` | `ALLOW_LIST` and `WHITE_LIST` (1.26+ uses allow-list) |
| `settings.transport` | `TRANSPORT` — `raknet` (default) \| `nethernet` |
| `settings.serverPort` | `SERVER_PORT` (default 19132) |
| `settings.ipv6Enabled` | enables `SERVER_PORT_V6` + second UDPRoute |

Only known keys are admitted.

**Transport:** Phase 1 defaults to `raknet` so clients can join via `IP:port` over
classic UDP 19132 (mcpelauncher external-server, console Add Server, etc.). BDS
≥1.26.50 defaults to NetherNet when unset in the image; the operator overrides that
with `TRANSPORT=raknet`. Set `settings.transport: nethernet` only when clients use
the NetherNet signaling path (`GET /v1/join` on TCP 19132 + ephemeral UDP gameplay) —
classic UDP ping / external-server join will not work in that mode.

## Updates

The reconciler polls Mojang’s public download-links API (same feed the itzg image uses)
and sets `status.availableVersion` / `status.updateAvailable`.

- **Default (`notifyOnly: true`)**: status only — never mutates the pod.
- **`notifyOnly: false`**: may set `spec.version.directDownloadUrl` to the latest zip so
  the next pod restart re-downloads (runtime-download analogue of Palworld’s
  `autoUpdateImage`).

Prefer `VERSION=LATEST` in production **when clients can update to match**. Bedrock
rejects mismatched client/server builds (often as a generic “connection failed”).
Check `GET http://<VIP>:19132/v1/join` for the advertised `version` / `protocol`, and
either update mcpelauncher’s installed build or set `spec.version.mode=PINNED` with
`spec.version.pinned` to the client version. Older builds (notably ≤1.19.80) had auth
issues — do not recommend pinning ancient versions in production docs.

## Persistence & recreate

Worlds live under `/data/worlds` on the PVC. The StatefulSet is single-replica; rolling
the pod (Recreate semantics) is safe as long as the PVC survives. Use
`persistence.existingClaim` to adopt a restored volume.

## Admin console (Phase 2)

BDS has no REST/RCON. Administration is the interactive console (`tty`/`stdin` enabled).
Phase 2 adds annotation-triggered `kubectl exec` / `send-command` helpers.

## Sample

See [examples/bedrockserver-sample.yaml](../examples/bedrockserver-sample.yaml).
