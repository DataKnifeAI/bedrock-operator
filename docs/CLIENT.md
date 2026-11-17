# Obtain a Bedrock client

How to get **Minecraft Bedrock Edition** so you can join a world hosted by
**bedrock-operator**. The operator runs the dedicated server only — it does **not**
redistribute Mojang client binaries. Buy / install the game through official channels
(or a supported community launcher on Linux).

## Windows

1. Install [Minecraft Launcher](https://www.minecraft.net/en-us/download) (Java & Bedrock)
   or get **Minecraft** from the [Microsoft Store](https://www.microsoft.com/store/productId/9NBLGGH2JHXJ).
2. Sign in with a Microsoft account that owns Bedrock.
3. Launch **Minecraft** (Bedrock), not Java Edition.

## Xbox / PlayStation / Nintendo Switch

Console Bedrock is the same multiplayer edition as PC/mobile.

- **Friends / Xbox join** — invite or join through the platform friends list when the
  host is online and discoverable.
- **Direct IP** — many console builds can add a server by address; support varies by
  platform and storefront. Prefer Friends/Xbox join when direct IP is unavailable.
- **LAN browse** often fails through Kubernetes (UDP broadcast is not forwarded). Use
  the VIP:port from status, or a [BedrockConnect](https://github.com/Pugmatt/BedrockConnect)
  companion if your console needs a proxy list.

## iOS / Android

- **Android:** [Google Play — Minecraft](https://play.google.com/store/apps/details?id=com.mojang.minecraftpe)
- **iOS / iPadOS:** [App Store — Minecraft](https://apps.apple.com/app/minecraft/id479516143)

Sign in with the Microsoft account that owns the game, then **Add Server** with the
operator address (see below).

## Linux (CachyOS / Arch)

There is **no official** Mojang Bedrock client for Linux. Community option:
[mcpelauncher](https://mcpelauncher.readthedocs.io/).

### Helper script (diagnose + guided install)

From a clone of this repo:

```bash
./scripts/setup-mcpelauncher-cachyos.sh           # check only
./scripts/setup-mcpelauncher-cachyos.sh --install # check, then prompt for packages
```

The script detects Arch/CachyOS, AUR helpers (`paru`/`yay`), mcpelauncher packages,
`/etc/profile.d` PATH hints, common multilib libs ([upstream dep list](https://github.com/minecraft-linux/mcpelauncher-manifest/issues/796)),
Wayland/Qt hints, and prints Google Play ownership steps. It does **not** use
`--noconfirm` and never redistributes Mojang binaries.

### AUR (recommended on Arch / CachyOS)

```bash
paru -S mcpelauncher-linux-bin mcpelauncher-ui-bin
```

Also install multilib audio/GL deps if missing (script checks these):

```bash
sudo pacman -S lib32-mesa lib32-libpulse lib32-alsa-plugins
```

**PATH note:** the UI binary lives at `/opt/mcpelauncher-bin/bin/mcpelauncher-ui-qt`.
After install, either open a **new login session** or:

```bash
source /etc/profile.d/mcpelauncher-bin.sh
```

Then run `mcpelauncher-ui-qt` (or start it from your app menu once PATH is updated).

On **Wayland**, if the Qt UI fails to appear:

```bash
QT_QPA_PLATFORM=xcb mcpelauncher-ui-qt
```

### Flatpak alternative

```bash
flatpak install flathub io.mrarm.mcpelauncher
flatpak run io.mrarm.mcpelauncher
```

If the Flatpak UI hangs on Wayland:

```bash
flatpak run --nosocket=wayland io.mrarm.mcpelauncher
```

### Google Play ownership

mcpelauncher does not ship the game. You must own Minecraft on **Google Play** and
authenticate so the launcher can download the APK you already purchased. This is a
community project — not affiliated with Mojang / Microsoft. Neither the helper script
nor bedrock-operator can buy the game or bypass license checks.

## Java Edition (Prism) — not for Bedrock servers

On Arch / CachyOS you can install a Java launcher with:

```bash
pacman -S prismlauncher
```

**Java Edition cannot join Bedrock dedicated servers** without a proxy such as
[Geyser](https://geysermc.org/) (out of scope for this operator). Use a Bedrock client
(Windows / console / mobile / mcpelauncher) to join worlds run by bedrock-operator.

## Connect to this operator’s server

After the `BedrockServer` is Ready, admins share:

```text
status.connectionAddress:status.connectionPort
```

Default port is **19132** / UDP. Example: `192.168.14.190:19132`.

```bash
kubectl get bedrockservers -o wide
kubectl get bedrockserver <name> -n <ns> \
  -o jsonpath='{.status.connectionAddress}:{.status.connectionPort}{"\n"}'
```

### In-game (PC / mobile / Linux launcher)

1. Open **Minecraft** (Bedrock) → **Play** → **Servers** (or **Friends** → add server).
2. **Add Server** → enter Name + the `IP:port` from status.
3. Join. If the world uses a whitelist, an operator must allow your gamertag first.

### Why LAN discovery fails

Kubernetes does not forward UDP broadcasts. Friends/LAN browse usually will **not** see
the pod. Always use the Gateway / LoadBalancer VIP and port from status (or enable
`spec.hostNetworkDiscovery` — see [BEDROCK_SERVER.md](BEDROCK_SERVER.md)).

## License reminder

- bedrock-operator does **not** bundle or redistribute Mojang client or server binaries.
- The default image downloads Bedrock Dedicated Server at startup; you accept the EULA
  via `spec.eula.accepted: true`.
- Clients are obtained only through Microsoft / Mojang storefronts (or mcpelauncher’s
  Play-owned download path on Linux).

## Related

- [BEDROCK_SERVER.md](BEDROCK_SERVER.md) — CRD, ports, LAN / BedrockConnect notes
- [ARCHITECTURE.md](ARCHITECTURE.md) — Gateway / UDPRoute layout
- Sample CR: [`config/samples/bedrock_v1alpha1_bedrockserver.yaml`](../config/samples/bedrock_v1alpha1_bedrockserver.yaml)
