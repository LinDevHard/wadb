# How ADB Wi-Fi pairing works: QR codes, mDNS, and two different ports

I ran into this while testing an app on a physical Pixel over Wi-Fi. Android
Studio's QR pairing worked, but in my setup getting the code meant opening a
project and sometimes waiting for Gradle sync, even though I was doing the rest
of the work outside the IDE.

`adb pair` was not quite equivalent. Android exposes a temporary endpoint for
pairing, while the later ADB connection uses another dynamically selected
port. After a reboot or network change, that connection endpoint may change
again.

I wanted one command, one QR scan, and the phone ready in `adb devices`. That
annoyance led me to build [wadb](https://github.com/LinDevHard/wadb), but the
useful part was tracing what Android Studio puts in the QR code and how the
phone becomes discoverable afterward.

## It is not one connection

The flow has two services and two different jobs:

```text
Host                                        Android device
----                                        --------------
                                            Wireless debugging enabled
                            <-------------- advertise _adb-tls-connect._tcp
generate service name + pairing secret
render them as a QR code  ----------------> scan QR
                                            start pairing server
                            <-------------- advertise _adb-tls-pairing._tcp
find the matching pairing service
run adb pair <pairing endpoint> ----------> authenticate secret, exchange keys
find the already-active connect service
run adb connect <connect endpoint> --------> authenticated ADB transport
```

The ordering matters. `_adb-tls-connect._tcp` is normally advertised while
Wireless debugging is active, so it may exist before pairing begins. Pairing
authorizes the workstation; it does not create the regular ADB transport or
guarantee that the host will receive a fresh mDNS event for it afterward.

## The QR carries a secret and an mDNS name

The QR code shown by Android Studio contains a string shaped like this:

```text
WIFI:T:ADB;S:studio-AbCdEf1234;P:a1B2c3D4e5F6;;
```

ADB reuses the Wi-Fi QR envelope with ADB-specific meanings:

- `T:ADB` identifies an ADB pairing payload.
- `S:` requests an mDNS service instance name.
- `P:` contains the temporary pairing secret.

Android Studio conventionally uses `studio-<random-10>` for the service name,
and wadb follows that convention. Android recognizes the payload from `T:ADB`;
the `studio-` prefix itself is not required by the parser.

After scanning, Android starts a pairing server and publishes the requested
name as an `_adb-tls-pairing._tcp` instance:

```text
studio-AbCdEf1234._adb-tls-pairing._tcp.local
```

This turns the `S:` value into a correlation ID. If several phones expose
pairing services on the same LAN, the host can wait for the exact random name
from its QR instead of choosing the first announcement it sees.

In wadb, the service suffix and pairing secret come from `crypto/rand`. The
random-byte mapping uses rejection sampling rather than byte modulo, avoiding
modulo bias in the generated strings.

## Pairing is the trust step

Once the matching endpoint is found, the host runs:

```text
adb pair <host>:<pairing-port>
```

wadb sends the shared secret through standard input instead of placing it in
the command arguments. The installed `adb` establishes TLS, authenticates the
shared secret using SPAKE2, and exchanges the peer information needed for
future connections, including the host public key and device GUID.

The pairing server is temporary. Normal ADB traffic uses the separately
advertised `_adb-tls-connect._tcp` endpoint:

```text
adb connect <host>:<connection-port>
```

This explains a confusing result: `adb pair` can succeed while the phone still
does not appear in `adb devices`. Trust has been established, but the host has
not found or connected to the regular TLS transport.

It also creates a small discovery race. Because the connect service may have
been active before pairing, waiting only for a new mDNS "create" event after
`adb pair` is not enough.

## The awkward part is discovery

A development machine often has several interfaces: Wi-Fi, Ethernet, a VPN,
or container bridges. Multicast can take the wrong path, and link-local IPv6
addresses are unusable unless they include an interface zone such as `%en0`.
An access point can also block peer-to-peer multicast entirely.

There is a timing problem too. A live browse can miss a short-lived
announcement or start after a service is already active. The adb server runs
its own mDNS implementation and may have a different view of the same network.

The implementation therefore combines two observations:

1. a direct DNS-SD browse for `_adb-tls-connect._tcp`;
2. the current service snapshot reported by `adb mdns services`.

The results are merged and deduplicated by exact `host:port`. The second source
is useful, but its human-readable output is not a stable machine contract and
has changed between platform-tools versions. The parser treats it as a hint
and skips rows it cannot understand.

When the pairing address is known, endpoints on that same host are tried first.
The separate `wadb connect` command also accepts `--device` when explicit
selection is needed on a network with several paired phones.

For multi-interface hosts, `--iface` binds the direct browse to one interface.
The interface is validated first, so a typo or inactive interface fails
immediately instead of looking like a long discovery timeout. With an explicit
interface, a discovered link-local IPv6 address can be given the required zone;
without one, an unscoped link-local address is skipped.

## Where the helper stops

I stopped at orchestration: the tool generates an ephemeral service identifier
and pairing secret, renders the QR code, and locates endpoints. Key exchange,
TLS, host keys, and the ADB transport remain inside the installed `adb`.
Reimplementing that layer would add security and compatibility risk without
improving the workflow I was trying to fix.

Pairing trust can survive a reboot or Wi-Fi change even when the connection
port changes. The connect service instance normally looks like:

```text
adb-<device-serial>-<random-suffix>
```

When that convention exposes the serial, it can be used to associate the
changing network endpoint with the same physical phone. Otherwise the address
has to remain the fallback identity.

## Android 17 changes the reconnect story

Android 17 and platform-tools 37 introduce ADB Wi-Fi 2.0. On a trusted wireless
debugging network, a paired device can automatically reconnect to the
workstation. That removes much of the need for a separate rediscovery command
on new devices, although the QR pairing flow and the two-service architecture
remain.

## What I verified

The part I had originally missed was that a successful `adb pair` is only the
trust step. A usable wireless device still depends on finding a separate
`_adb-tls-connect._tcp` endpoint, often on another port.

I tested this workflow on macOS with a physical Pixel. The repository also has
unit and command-path tests, but its real mDNS, pairing, and connection flow is
not exercised in CI. Linux is supported and expected to work; Windows remains
experimental and unverified. Automatic wireless discovery requires working
mDNS, although direct `adb pair` and `adb connect` endpoints do not.

For platform scope, Wireless debugging requires Android 11/API 30 or newer on
phones and Android 13/API 33 or newer on TV and Wear OS. Pairing authorizes the
workstation's host key, while actual ADB access remains conditional on Wireless
debugging being enabled on an allowed network. The authorization persists
until the workstation is forgotten or ADB authorizations are revoked.

## References

- [Architecture of ADB Wi-Fi](https://android.googlesource.com/platform/packages/modules/adb/+/HEAD/docs/dev/adb_wifi.md)
- [Android's ADB QR parser](https://android.googlesource.com/platform/packages/apps/Settings/+/refs/heads/main/src/com/android/settings/wifi/dpp/AdbQrCode.java)
- [ADB pairing connection API](https://android.googlesource.com/platform/packages/modules/adb/+/refs/heads/main/pairing_connection/include/adb/pairing/pairing_connection.h)
- [ADB pairing and SPAKE2 implementation](https://android.googlesource.com/platform/packages/modules/adb/+/refs/heads/main/pairing_connection/pairing_connection.cpp)
- [Android Debug Bridge documentation](https://developer.android.com/tools/adb#connect-to-a-device-over-wi-fi)
- [wadb source code](https://github.com/LinDevHard/wadb)
