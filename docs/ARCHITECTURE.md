# Architecture

Kubebuilder-style Go operator that reconciles a `BedrockServer` custom resource into a
Minecraft Bedrock Dedicated Server (BDS) workload: StatefulSet, PVC, Secret, ClusterIP
services, and Envoy Gateway (UDPRoute + TCPRoute for NetherNet).

Patterns (Gateway naming, `{name}` / `{name}-envoy` backend split, PVC mounts, status
fields) align with [palworld-operator](https://github.com/DataKnifeAI/palworld-operator)
and [dragonwilds-operator](https://github.com/DataKnifeAI/dragonwilds-operator).

## Licensing (read first)

Mojang does **not** permit redistributing BDS binaries in published images. This operator:

- Does **not** bake or repack Mojang software into any image we publish
- Uses [`itzg/minecraft-bedrock-server`](https://github.com/itzg/docker-minecraft-bedrock-server),
  which downloads BDS from Mojang at container startup
- Requires `spec.eula.accepted: true` (maps to `EULA=TRUE`) before creating children

## Data path (recommended: direct VIP)

```
Clients → spec.gateway.address (Kube-VIP / MetalLB LoadBalancer)
              ↓
      {name} Service  TCP 19132 + UDP 19140…(19140+maxPlayers-1)
              ↓
      StatefulSet  (itzg/minecraft-bedrock-server)  replicas=1
              ↓
      PVC (/data)  — worlds, server package, properties, permissions, packs
```

Set `gateway.enabled: false`, `serviceType: LoadBalancer`, and `gateway.address`.
The operator sets `TRANSPORT=nethernet` and `SERVER_UDP_PORTS` in VIP form so
clients reach gameplay behind kube-vip.

Optional Envoy path (`gateway.enabled: true`) still wires TCP/UDP **19132** only —
it does **not** yet expose the NetherNet gameplay UDP range, so prefer the direct
VIP for joins on BDS ≥1.26.60.

LAN discovery (UDP broadcast) does **not** work through pod networking. Opt into
`spec.hostNetworkDiscovery: true` for LAN, or point consoles at a BedrockConnect
companion (Phase 3 / docs note). Direct IP:port via `.status` always works.

## Owned resources

| Kind | Purpose |
|------|---------|
| StatefulSet | Single-instance BDS pod (`replicas` hard-capped at 1; Recreate semantics) |
| PersistentVolumeClaim | `/data` — worlds + runtime-downloaded server package |
| Secret | Optional `operators.json` / `allowlist.json` when `secrets.generateSecrets` |
| Service | LoadBalancer/ClusterIP: NetherNet TCP 19132 + UDP gameplay range (or legacy RakNet UDP) |
| Service (Envoy backend) | `{name}-envoy` ClusterIP (optional Gateway path) |
| Gateway + EnvoyProxy | External VIP binding (optional; 19132 only today) |
| UDPRoute | Game `19132/UDP` (+ `19133/UDP` when `settings.ipv6Enabled`) — not the 19140+ range yet |
| TCPRoute | NetherNet TCP signaling on the game port |

If UDPRoute support is unavailable in the cluster, set `gateway.enabled: false` and use
`gateway.serviceType: LoadBalancer` with `gateway.address`.

**Envoy vs direct VIP:** BDS ≥1.26.60 requires NetherNet (`transport=nethernet`) —
TCP `GET /v1/join` on the game port plus a UDP gameplay range (`SERVER_UDP_PORTS`,
default `19140`…`19140+maxPlayers-1`). The operator defaults to NetherNet and
exposes that UDP range on the Service. Prefer the Dragonwilds pattern for
gameplay UDP: `gateway.enabled: false` + `serviceType: LoadBalancer` + `address`
so kube-vip hits the pod without Envoy UDP source-IP rewrite. Envoy TCPRoute alone
is not enough for joins once NetherNet needs the UDP range behind the VIP.

## Design rules

| Rule | Why |
|------|-----|
| EULA hard gate | Image exits without it; operator refuses children |
| StatefulSet, replicas=1 | BDS is single-instance + stateful; UDP bind |
| Env-var settings allow-list | Typed CRD → itzg env; unknown keys rejected |
| World on PVC | No worldguid seeding needed; Recreate keeps world if PVC survives |
| Update watcher notify-only by default | Status flips; never auto-apply unless `notifyOnly: false` |
| Runtime download | Version bumps re-download at pod start (`VERSION` / `DIRECT_DOWNLOAD_URL`) |

## Images

| Choice | Detail |
|--------|--------|
| **Default game image** | `itzg/minecraft-bedrock-server:latest` |
| Operator image (CI) | `harbor.dataknife.net/library/bedrock-operator` — see [GITLAB_MIRROR.md](GITLAB_MIRROR.md) |

## Bedrock vs Palworld (deltas)

| | bedrock-operator | palworld-operator |
|--|------------------|-------------------|
| Image | Community itzg (runtime-download) | Official Pocketpair |
| CRD | `BedrockServer` | `PalworldServer` |
| Primary port | NetherNet: TCP `19132` + UDP `19140…` | `8211/UDP` |
| Config | Env → `server.properties` | ConfigMap → `PalWorldSettings.ini` |
| Admin API | Console only (stdin / exec) | REST |
| Updates | Mojang feed + runtime download | Image-level `autoUpdateImage` |
| Secrets | operators/allowlist files | join/admin passwords |

## Prerequisites

- Kubernetes 1.28+
- [Envoy Gateway](https://gateway.envoyproxy.io/) with GatewayClass `envoy` (or LB Service fallback)
- StorageClass suitable for game worlds (ReadWriteOnce)
- One dedicated external IP per server when Gateway is enabled (`spec.gateway.address`)
