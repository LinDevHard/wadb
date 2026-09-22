#compdef wadb

_wadb() {
    _arguments -s -S \
        '--adb=[path to adb binary]:adb binary:_files' \
        '--iface=[network interface to browse for mDNS]:interface:_net_interfaces' \
        '--pair-only[pair the device, then exit without running adb connect]' \
        '--qr-ascii[render the QR code with plain ASCII blocks]' \
        '--qr-invert[invert the QR code for terminals with a light background]' \
        '--qr-sixel[render the QR code as a sixel image]' \
        '--verbose[print discovered mDNS service entries to stderr]' \
		'--non-interactive[never prompt on a terminal; accept a pairing code only from redirected stdin]' \
        '--pair-timeout=[time to wait for the pairing mDNS announce]:duration:(30s 60s 2m 5m)' \
		'--connect-timeout=[time to wait for the connect mDNS announce]:duration:(15s 30s 60s 2m)' \
		'--scan-timeout=[time to scan for devices]:duration:(1s 3s 5s 10s)' \
		'--device=[device serial, address, host, or mDNS instance]:device:' \
		'--all[operate on every matching wireless device]' \
		'--json[write legacy v1.2 JSON array output]' \
		'--output=[output format]:format:(text json)' \
        '--version[print version and exit]' \
        '-v[shorthand for --version]' \
        '--help[print usage and exit]' \
        '1:command:((
            pair\:"pair using the address and six-digit code shown by the device"
            connect\:"reconnect a device already paired with this host"
			devices\:"list devices known to adb and wireless debugging announces"
			disconnect\:"disconnect one or all wireless devices"
            doctor\:"report the local adb, its version, and visible mDNS services"
			capabilities\:"describe the agent-facing CLI contract"
			schema\:"print the embedded structured-output JSON Schema"
        ))' \
		'2:pairing address (host\:port):'
}

_wadb "$@"
