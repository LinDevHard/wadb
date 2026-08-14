# bash completion for wadb

_wadb_interfaces() {
    if [ -d /sys/class/net ]; then
        command ls /sys/class/net 2>/dev/null
    else
        command ifconfig -l 2>/dev/null
    fi
}

_wadb() {
    local cur prev commands flags
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    commands="connect doctor"
    flags="--adb --iface --pair-only --qr-ascii --qr-invert --qr-sixel
           --verbose --pair-timeout --connect-timeout --version -v --help"

    case "$prev" in
        --adb)
            COMPREPLY=($(compgen -f -- "$cur"))
            return
            ;;
        --iface)
            COMPREPLY=($(compgen -W "$(_wadb_interfaces)" -- "$cur"))
            return
            ;;
        --pair-timeout|--connect-timeout)
            COMPREPLY=($(compgen -W "30s 60s 2m 5m" -- "$cur"))
            return
            ;;
    esac

    if [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "$flags" -- "$cur"))
    else
        COMPREPLY=($(compgen -W "$commands" -- "$cur"))
    fi
}

complete -F _wadb wadb
