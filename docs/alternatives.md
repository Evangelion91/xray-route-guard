# Existing projects and this project's scope

Automatic node selection is an established feature, not an invention of this
project. Prefer an existing manager if it fits your setup.

| Project | Existing capability | Why this add-on is different |
|---|---|---|
| [PassWall2](https://github.com/Openwrt-Passwall/openwrt-passwall2) | LuCI, subscriptions, isolated SOCKS candidate tests, automatic backup selection | Integrated manager. Its inspected switch path verifies listener startup; this add-on journals an existing JSON file and checks HTTPS after restart. |
| [Podkop](https://podkop.net/docs/sections/) | Selective routing and URLTest groups using sing-box | Owns a wider routing/DNS setup and a different core. |
| [RouteFlux](https://github.com/Alaxay8/routeflux) | Xray subscriptions, scoring, health-based selection, config validation and rollback; installable releases | Broader manager that generates Xray config and manages routing/DNS. |
| [OpenWrt VPN Guardian](https://github.com/Yakwilik/openwrt-vpn-guardian) | v2rayA failover, health supervision, declarative stack and rollback | Requires its v2rayA/front/policy stack; a different integration boundary. |

Research date: 2026-11-05. These observations come from the projects' own
documentation and code, not comparative hardware benchmarks. In particular,
[PassWall2's candidate loop](https://github.com/Openwrt-Passwall/openwrt-passwall2/blob/main/luci-app-passwall2/root/usr/share/passwall2/socks_auto_switch.sh)
and [switch implementation](https://github.com/Openwrt-Passwall/openwrt-passwall2/blob/main/luci-app-passwall2/root/usr/share/passwall2/app.sh),
and [RouteFlux's runtime writer](https://github.com/Alaxay8/routeflux/blob/master/internal/backend/xray/writer.go)
were inspected. Do not infer feature absence across every version from this table.

The useful boundary here is an independently configured Xray service: one
outbound tag, an explicit restart command, and no automatic firewall/DNS changes.
The decision loop, subscription decoder and runtime backend are separate modules.
This is a small add-on, not a replacement router distribution or LuCI manager.

The router-help section shown by the Durev VPN Telegram bot on the research date
directed users to model-specific VLESS setup instructions. That specific section
did not offer an installer or describe failover. This is not a claim that the
provider has no other guidance. The project is independent and not endorsed by
Durev VPN, GL.iNet or the Xray project.
