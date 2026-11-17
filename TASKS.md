# Tasks

## Phase 1 — MVP (current)

- [x] CRD `BedrockServer` + markers
- [x] StatefulSet / PVC / Service / Envoy Gateway (UDPRoute + TCPRoute)
- [x] EULA gate, settings→env, update watcher (notify-only default)
- [x] envtest suite + sample CR + docs

## Phase 2 — Operational polish

- [ ] Annotation-based console commands (`games…/console-command`)
- [ ] Graceful-stop hooks + backup CronJob
- [ ] `hostNetworkDiscovery` docs + BedrockConnect pointer
- [ ] `notifyOnly: false` apply flow UX + old-version advisory

## Phase 3 — Parity

- [ ] Server Manager UI (proxied console + saves + updates)
- [ ] `spec.mods` packs/resources PVC
- [ ] Helm chart + docs site section
