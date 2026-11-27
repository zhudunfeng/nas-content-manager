# Clash DNS configuration and narrow UAV repair

## Verified local state

- Clash Verge Rev is installed at `D:/adunsoftware/clash_verge/clash-verge.exe`, version 2.4.3.
- Its running Mihomo core answers the local Windows named pipe `\\.\pipe\verge-mihomo`; read-only `GET /version` returned HTTP 200 and `v1.19.15`. `GET /configs` returned HTTP 200 and TUN enabled.
- The effective generated file is `clash-verge.yaml` in the original Clash configuration directory. Its ordinary external controller binding is empty; the named pipe is the supported active interface. The configured API secret was used only in child-process memory and never reported.
- DNS uses Fake-IP and has no explicit `fake-ip-filter` entries. The main agent independently observed the two search domains resolving into the reserved `198.18.0.0/15` range. The worker's public-IP checks must remain unchanged.
- The separate `dns_config.yaml` is inactive because `verge.yaml` has `enable_dns_settings: false`. Enabling that feature would replace the whole DNS section, so it is unsuitable for this repair.
- Global `profiles/Script.js` is the stock identity template. Global Merge only retains selected profiles; current per-profile merge and script are empty/identity and do not override the proposed addition.

## Installed-version source evidence

Official 2.4.3 source archive: `https://codeload.github.com/clash-verge-rev/clash-verge-rev/tar.gz/refs/tags/v2.4.3`.

The public source archive and relevant extracted source are retained under `.runtime/verify/windows-uav-fake-ip-repair/research/`. This archive contains upstream public code only, not the user's Clash configuration.

- `upstream/src-tauri/src/enhance/mod.rs:317-343`: global Merge then global Script are applied to the subscription configuration.
- `upstream/src-tauri/src/enhance/mod.rs:348` and `:553-556`: per-profile processing follows global processing. Existing local transforms do not replace the target DNS property.
- `upstream/src-tauri/src/enhance/mod.rs:490-516` and `:588`: independent DNS settings are applied only when enabled, as a section-wide replacement.
- `upstream/src-tauri/src/enhance/merge.rs:6-14`: deep merge recursively combines maps but replaces non-map values, including arrays. A YAML Merge array would replace future subscription filter entries.
- `upstream/src-tauri/src/core/manager/config.rs:119-123`: the official update route reloads a configuration path with force enabled after generating and validating it.

## Chosen minimal change

Append only these blacklist exclusions using the existing global identity Script, preserving pre-existing and future subscription entries:

```text
+.jable.tv
+.huangguoai.com
+.fs1.app
```

The main agent verified the installed `uav_downloader/core/config.py:17` declares Jable mirrors `jable.tv` and `fs1.app`; the third entry therefore covers an actual configured mirror rather than speculative expansion. The helper refuses a custom existing global script, whitelist/rule/unknown filter modes, malformed filter arrays, an enabled independent DNS override, or configuration hashes that changed after planning.

All original configurations, secret-bearing backups and the generated candidate remain inside the original Clash configuration directory. Only paths, SHA-256 hashes, status codes, flags, domain membership and sanitized results are written to the repository. No subscription URL, node identity, credential or original configuration body is printed or copied there.

## Helper and preflight results

Helper: `.runtime/verify/windows-uav-fake-ip-repair/repair_dns.py`.

Completed read-only `probe` and `plan`. Node at `D:/adunsoftware/nodejs/node.exe` evaluated the generated global Script in a `vm` context without external capabilities. The actual current configuration was passed through stdin and never written into the repository. Nine checks passed: exact target-only append on the real config; existing filter preservation; explicit blacklist; missing DNS object; three incompatible filter modes; two malformed filter arrays. All successful configurations were checked for idempotence.

`plan.json` reports `currentConfigOnlyAppend: true`, `idempotent: true`, HTTP 200 for version/config queries and TUN enabled. No Clash configuration write or reload was performed by this research agent.

Application is owned by the main agent:

```powershell
python -B scripts/project_env.py -- python -B .runtime/verify/windows-uav-fake-ip-repair/repair_dns.py apply
```

The action checks plan hashes, creates the task-specific backup under the original Clash root, writes a candidate there, runs installed `verge-mihomo.exe -t -d <root> -f <candidate>`, verifies that only the filter append differs semantically, writes Script/runtime and calls `PUT /configs?force=true` over the named pipe. Responses are captured in memory; only the reload status is reported. Core command diagnostics are captured without printing secret-bearing content.

After applying:

```powershell
python -B scripts/project_env.py -- python -B .runtime/verify/windows-uav-fake-ip-repair/repair_dns.py verify
```

This confirms files and API availability, not DNS/search success. The main agent must independently verify real Windows DNS, actual worker searches, readiness and protected app data. No claim of end-to-end success is made by preflight alone.

## Rollback

Restore `Script.js.before` to global `profiles/Script.js` and `clash-verge.yaml.before` to `clash-verge.yaml` from `uav-fake-ip-repair-20261127` under the original Clash directory, then reload that restored runtime path through the same named pipe. A single recovery command, which prints only the status, was provided to the main agent:

```powershell
python -B scripts/project_env.py -- python -B -c "import runpy,shutil,json; m=runpy.run_path(r'.runtime/verify/windows-uav-fake-ip-repair/repair_dns.py'); shutil.copy2(m['BACKUP']/'Script.js.before',m['SCRIPT']); shutil.copy2(m['BACKUP']/'clash-verge.yaml.before',m['RUNTIME']); r=m['request_pipe']('PUT','/configs?force=true',{'path':str(m['RUNTIME'])}); print(json.dumps({'rollbackReloadStatus':r['status']}))"
```

The helper intentionally refuses to overwrite an existing task backup. If apply fails after writing, restore first; do not blindly rerun apply.

## Remaining verification boundaries

- The main agent owns live reload, Windows resolver checks and real UAV search receipts.
- A hot reload normally preserves selected proxies through existing `profile.store-selected=true`; preservation should be checked without printing node names. This helper does not enumerate proxies or broaden its API scope.
- Later user customization, local profile transforms or enabling the independent DNS override can supersede this narrow global append and requires a new review.
- No production source, app binary, environment file, data store, SSRF boundary or existing UAV runtime materials were changed by this research agent.
