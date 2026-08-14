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
        '--pair-timeout=[time to wait for the pairing mDNS announce]:duration:(30s 60s 2m 5m)' \
        '--connect-timeout=[time to wait for the connect mDNS announce]:duration:(15s 30s 60s 2m)' \
        '--version[print version and exit]' \
        '-v[shorthand for --version]' \
        '--help[print usage and exit]' \
        '1:command:((
            connect\:"reconnect a device already paired with this host"
            doctor\:"report the local adb, its version, and visible mDNS services"
        ))'
}

_wadb "$@"
