# bedrock-operator

<p align="center">
  <img src="docs/assets/hero-bedrock.jpg" alt="Bedrock Operator — cuboid world keeper" width="640">
</p>

Kubernetes operator for [Minecraft Bedrock Dedicated Server](https://www.minecraft.net/en-us/download/server/bedrock)
(BDS). Sibling to [palworld-operator](https://github.com/DataKnifeAI/palworld-operator) /
[dragonwilds-operator](https://github.com/DataKnifeAI/dragonwilds-operator): Go/kubebuilder,
CRD-first, Envoy Gateway (UDP + NetherNet TCP).

**Site:** [dataknifeai.github.io/bedrock-operator](https://dataknifeai.github.io/bedrock-operator/)  
**Repos:** [GitHub](https://github.com/DataKnifeAI/bedrock-operator)

**Licensing:** We neither bundle nor redistribute Mojang software. The default image
[`itzg/minecraft-bedrock-server`](https://github.com/itzg/docker-minecraft-bedrock-server)
downloads BDS at startup. Users accept the Minecraft EULA via `spec.eula.accepted`.

## Status

Phase 1 MVP: `BedrockServer` CRD, StatefulSet/PVC/Gateway reconcile, EULA gate,
settings→env mapping, Mojang update watcher, envtest suite, docs.

## Docs

- [CLIENT.md](docs/CLIENT.md) — obtain Bedrock clients (Windows, console, mobile, Linux)
- [BEDROCK_SERVER.md](docs/BEDROCK_SERVER.md) — user guide
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) — resource graph & design rules
- [GITLAB_MIRROR.md](docs/GITLAB_MIRROR.md) — CI / Harbor publish

## Develop

```bash
make generate manifests
make test
make build
```

## Install (dev)

```bash
make install          # CRDs
make deploy IMG=...   # controller
kubectl apply -f config/samples/bedrock_v1alpha1_bedrockserver.yaml
```

## Example

```yaml
apiVersion: bedrock.dataknife.ai/v1alpha1
kind: BedrockServer
metadata:
  name: survival-1
spec:
  eula:
    accepted: true
  version:
    mode: LATEST
  settings:
    levelName: survival-1
    gameMode: survival
    difficulty: normal
    maxPlayers: 10
  gateway:
    enabled: true
    address: "10.0.0.50"
  persistence:
    size: 8Gi
```

## License

Apache-2.0
