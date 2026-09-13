# EasyTier outbound via FFI data plane

Mihomo can join an EasyTier mesh as an outbound (`type: easytier`) and dial
overlay peers through EasyTier's userspace data plane. No TUN device is
created (`flags.no_tun = true` is forced).

There are two ways to start the outbound:

1. **FFI data plane** (this document): load `libeasytier_ffi` at runtime and
   use the current EasyTier session + submit/wait/take ABI.
2. **Embedded `easytier-go`**: structured YAML fields (`network-name`,
   `peers`, …) without `ffi-library`. That path is the Alpha `#3194`
   implementation and does not need a shared library.

`ffi-library` selects the FFI path. FlClash is out of scope.

## Build mihomo (linux amd64, EasyTier enabled)

EasyTier is compiled in unless you pass the `no_easytier` build tag.
The default Makefile already uses `CGO_ENABLED=0`.

```bash
# from the repository root
CGO_ENABLED=0 go build -tags with_gvisor -o mihomo .
# or
make linux-amd64
```

Disable EasyTier (and skip loading the FFI library) with:

```bash
CGO_ENABLED=0 go build -tags 'with_gvisor no_easytier' -o mihomo .
```

## Build `libeasytier_ffi`

Use a current EasyTier tree that exports the **session-based** data-plane ABI
(`data_plane_session_open`, `data_plane_*_submit`, `data_plane_completion_wait`,
`data_plane_*_result_take`). The older PoC symbols
(`data_plane_tcp_connect` / `*_start` / `*_finish`) are stale and are not used.

```bash
git clone https://github.com/EasyTier/EasyTier.git
cd EasyTier
cargo build --release -p easytier-ffi --features 'c-abi,ffi-dataplane'
# artifact, typically:
#   target/release/libeasytier_ffi.so
```

The `easytier-ffi` crate defaults already include `c-abi` and `ffi-dataplane`.
Copy the `.so` into the mihomo home directory (or another path listed in
`SAFE_PATHS`). Mihomo refuses to load libraries outside those directories.

```bash
# example: allow a system library directory
export SAFE_PATHS=/usr/local/lib
```

## Example YAML (placeholders only)

See `docs/examples/easytier-ffi.yaml`. Do not commit real network names,
secrets, or peer URIs.

```yaml
proxies:
  - name: easytier
    type: easytier
    ffi-library: ./libeasytier_ffi.so
    instance-name: mihomo-easytier
    # exactly one of config / config-file
    config: |
      instance_name = "mihomo-easytier"
      ipv4 = "10.77.0.10"
      [network_identity]
      network_name = "CHANGE_ME_NETWORK"
      network_secret = "CHANGE_ME_SECRET"
      [[peer]]
      uri = "tcp://203.0.113.10:11010"
    udp: true

rules:
  - IP-CIDR,10.77.0.0/24,easytier
  - MATCH,DIRECT
```

`instance-name` must match EasyTier `instance_name` / `inst_name` in the TOML.
Mihomo injects `instance_name` when the TOML omits it, and always rewrites
`[flags] no_tun = true` and `bind_device = false`.

The FFI data plane (ABI v3) accepts **IPv4 overlay targets only**. Hostnames
are resolved with mihomo's default resolver before dial.

`interface-name`, `routing-mark`, and `dialer-proxy` do not apply to sockets
created inside `libeasytier_ffi`.

## Manual mesh smoke test

This environment cannot join a real EasyTier mesh. After you have a second
node already on the same network:

1. Build `libeasytier_ffi` and mihomo as above.
2. Put the `.so` on a safe path and fill `docs/examples/easytier-ffi.yaml`
   with **your** `network_name`, `network_secret`, and peer URI.
3. On the remote mesh node, run a listener (example: `nc -l 10.77.0.2 8080`).
4. Start mihomo with the example config and:

```bash
curl -x socks5h://127.0.0.1:7890 http://10.77.0.2:8080/
```

or `curl` against whatever HTTP service is reachable on the overlay CIDR
matched by `IP-CIDR`. Success is a completed TCP response from the peer,
not merely "instance started" in the log.

On linux/amd64, `DataPlaneSocketAddr` (20 bytes) is a SysV MEMORY argument
and is passed on the stack, not as a register pointer. The binding uses
`purego.SyscallN` for `tcp_connect_submit` / `udp_send_submit` for that
reason.

Unit tests compile a tiny C stub (not EasyTier) and only prove the ABI
binding on linux/amd64:

```bash
go test ./component/easytierffi ./adapter/outbound ./component/easytier
```
