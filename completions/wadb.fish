# fish completion for wadb

function __wadb_interfaces
    if test -d /sys/class/net
        command ls /sys/class/net 2>/dev/null
    else
        command ifconfig -l 2>/dev/null | string split ' '
    end
end

complete -c wadb -f

complete -c wadb -n __fish_use_subcommand -a pair -d 'Pair using the address and six-digit code shown by the device'
complete -c wadb -n __fish_use_subcommand -a connect -d 'Reconnect a device already paired with this host'
complete -c wadb -n __fish_use_subcommand -a devices -d 'List devices known to adb and wireless debugging announces'
complete -c wadb -n __fish_use_subcommand -a disconnect -d 'Disconnect one or all wireless devices'
complete -c wadb -n __fish_use_subcommand -a doctor -d 'Report the local adb, its version, and visible mDNS services'
complete -c wadb -n __fish_use_subcommand -a capabilities -d 'Describe the agent-facing CLI contract'
complete -c wadb -n __fish_use_subcommand -a schema -d 'Print the embedded structured-output JSON Schema'

complete -c wadb -l adb -r -F -d 'Path to adb binary'
complete -c wadb -l iface -r -a '(__wadb_interfaces)' -d 'Network interface to browse for mDNS'
complete -c wadb -l pair-only -d 'Pair the device, then exit without running adb connect'
complete -c wadb -l qr-ascii -d 'Render the QR code with plain ASCII blocks'
complete -c wadb -l qr-invert -d 'Invert the QR code for terminals with a light background'
complete -c wadb -l qr-sixel -d 'Render the QR code as a sixel image'
complete -c wadb -l verbose -d 'Print discovered mDNS service entries to stderr'
complete -c wadb -l non-interactive -d 'Never prompt on a terminal; accept a pairing code only from redirected stdin'
complete -c wadb -l pair-timeout -r -a '30s 60s 2m 5m' -d 'Time to wait for the pairing mDNS announce'
complete -c wadb -l connect-timeout -r -a '15s 30s 60s 2m' -d 'Time to wait for the connect mDNS announce'
complete -c wadb -l scan-timeout -r -a '1s 3s 5s 10s' -d 'Time to scan for devices'
complete -c wadb -l device -r -d 'Device serial, address, host, or mDNS instance'
complete -c wadb -l all -d 'Operate on every matching wireless device'
complete -c wadb -l json -d 'Write legacy v1.2 JSON array output'
complete -c wadb -l output -r -a 'text json' -d 'Select text or versioned JSON output'
complete -c wadb -l version -d 'Print version and exit'
complete -c wadb -s v -d 'Print version and exit'
complete -c wadb -l help -d 'Print usage and exit'
