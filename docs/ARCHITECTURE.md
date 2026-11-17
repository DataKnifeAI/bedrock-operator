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

## Data path

```
Clients → spec.gateway.address (Kube-VIP / MetalLB)
              ↓
      {base}-gateway  (GatewayClass: envoy)
              ↓
   UDPRoute (19132, optional 19133) + TCPRoute (NetherNet TCP 19132)
              ↓
      {name}-envoy  (ClusterIP)  →  {name} (ClusterIP)
              ↓
      StatefulSet  (itzg/minecraft-bedrock-server)  replicas=1
              ↓
      PVC (/data)  — worlds, server package, properties, permissions, packs
```

LAN discovery (UDP broadcast) does **not** work through pod networking. Opt into
`spec.hostNetworkDiscovery: true` for LAN, or point consoles at a BedrockConnect
companion (Phase 3 / docs note). Direct IP:port via `.status` always works.

## Owned resources

| Kind | Purpose |
|------|---------|
| StatefulSet | Single-instance BDS pod (`replicas` hard-capped at 1; Recreate semantics) |
| PersistentVolumeClaim | `/data` — worlds + runtime-downloaded server package |
| Secret | Optional `operators.json` / `allowlist.json` when `secrets.generateSecrets` |
| Service (ClusterIP) | Game UDP + NetherNet TCP; console/exec target |
| Service (Envoy backend) | `{name}-envoy` ClusterIP |
| Gateway + EnvoyProxy | External VIP binding |
| UDPRoute | Game `19132/UDP` (+ `19133/UDP` when `settings.ipv6Enabled`) |
| TCPRoute | NetherNet TCP signaling on the game port (Bedrock ≥ 1.26.50) |

If UDPRoute support is unavailable in the cluster, set `gateway.enabled: false` and use
`gateway.serviceType: LoadBalancer` with `gateway.address`.

**Envoy vs direct VIP:** Bedrock ≥1.26.50 image default is NetherNet
(`transport=nethernet`) — status/signaling is **TCP HTTP** on the game port
(`GET /v1/join`), not classic RakNet UDP. The operator defaults
`settings.transport=raknet` so mcpelauncher / Add Server IP:port joins work on UDP
19132; set `nethernet` only for NetherNet-capable clients. Envoy TCPRoute is fine for
the NetherNet path. If joins still fail after versions match (generic “connection
failed” / WebRTC ICE), prefer the Dragonwilds pattern: `gateway.enabled: false` +
`serviceType: LoadBalancer` + `address` so kube-vip hits the pod without Envoy UDP
source-IP rewrite. Trade-off: loses Gateway policy / shared Envoy dataplane; gains a
direct VIP like `dragonwilds-server`.

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
| Primary port | `19132/UDP` (+ TCP NetherNet) | `8211/UDP` |
| Config | Env → `server.properties` | ConfigMap → `PalWorldSettings.ini` |
| Admin API | Console only (stdin / exec) | REST |
| Updates | Mojang feed + runtime download | Image-level `autoUpdateImage` |
| Secrets | operators/allowlist files | join/admin passwords |

## Prerequisites

- Kubernetes 1.28+
- [Envoy Gateway](https://gateway.envoyproxy.io/) with GatewayClass `envoy` (or LB Service fallback)
- StorageClass suitable for game worlds (ReadWriteOnce)
- One dedicated external IP per server when Gateway is enabled (`spec.gateway.address`)
