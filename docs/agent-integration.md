# Agent integration

`wadb` exposes a stable, non-interactive JSON contract for scripts and AI agents that have access to the user's local shell. It still requires `adb`, local network access to the Android device, and a physical action on the device for first-time pairing.

The default output format is `text`. Pass `--output json` or set `WADB_OUTPUT=json` for the structured contract below. Agent invocations should also pass `--non-interactive` or set `WADB_NON_INTERACTIVE=true` so wadb never waits for terminal input.

Discover the contract without starting adb or scanning the network:

```sh
wadb capabilities --non-interactive --output json
```

The response describes commands, side effects, user interaction, stdin requirements, stable errors, and the published schema at [`schemas/wadb-output-v1.schema.json`](../schemas/wadb-output-v1.schema.json). The same schema is embedded in installed binaries and can be retrieved with `wadb schema`; use `wadb schema --output json` when the schema itself must be carried inside the standard response envelope.

## Recommended workflow

Inspect the environment and available devices before changing connection state:

```sh
wadb doctor --non-interactive --output json
wadb devices --non-interactive --output json
wadb connect --device RF8M1234ABC --non-interactive --output json
```

If Android shows a pairing address and six-digit code, start:

```sh
wadb pair 192.168.1.20:37123 --non-interactive --output json
```

Provide the code through redirected standard input. Do not place it in command arguments, environment variables, shell history, logs, or chat. With `--non-interactive`, wadb rejects terminal stdin instead of prompting. QR pairing remains intentionally interactive: run `wadb` without structured output and ask the user to scan the displayed QR code.

Disconnecting changes device state and should be explicit:

```sh
wadb disconnect --device RF8M1234ABC --non-interactive --output json
wadb disconnect --all --non-interactive --output json
```

An agent should use `--all` only when the user explicitly requests every wireless device.

## JSON contract

Every `--output json` invocation writes exactly one object to stdout. Human-readable progress and verbose diagnostics go to stderr.

Successful response:

```json
{
  "schema_version": 1,
  "ok": true,
  "action": "connect",
  "result": {
    "connections": [
      {
        "address": "192.168.1.20:40002",
        "instance": "adb-RF8M1234ABC-xYz",
        "status": "connected",
        "device": "Pixel 8"
      }
    ]
  }
}
```

Failed response:

```json
{
  "schema_version": 1,
  "ok": false,
  "action": "connect",
  "error": {
    "code": "device_not_found",
    "message": "no discovered device matches \"RF8M1234ABC\"",
    "suggestion": "Run wadb devices --output json and select an id or address from the result.",
    "retryable": false,
    "requires_user_action": true
  }
}
```

The top-level envelope is stable within schema version `1`. Consumers must ignore unknown fields so compatible fields can be added later. Validate complete responses with [`schemas/wadb-output-v1.schema.json`](../schemas/wadb-output-v1.schema.json). The older `--json` flag retains the raw array shapes introduced in v1.2 for backward compatibility; new integrations should use `--output json`.

`retryable` means repeating the operation may succeed after a transient adb, discovery, or connection failure. `requires_user_action` means an agent should stop and ask the user for a physical action, explicit selection, corrected input, or local installation rather than retrying automatically.

Known error codes:

| Code | Meaning |
| --- | --- |
| `invalid_arguments` | The command, flags, or environment values are invalid. |
| `interactive_required` | The requested flow requires a person, such as scanning a QR code. |
| `invalid_pairing_code` | Standard input did not contain exactly six digits. |
| `adb_not_found` | Android platform-tools could not be located. |
| `adb_error` | The local adb installation or server failed. |
| `pairing_failed` | Android rejected or could not complete pairing. |
| `discovery_timeout` | No suitable Wireless debugging mDNS announcement appeared. |
| `device_not_found` | No device matches the supplied selector. |
| `multiple_devices` | Structured mode refused to choose between multiple devices. |
| `connection_failed` | adb could not connect to the selected endpoint. |
| `disconnect_failed` | adb could not disconnect the selected endpoint. |
| `internal_error` | An unexpected failure has no more specific classification. |

Exit status `0` means success, `1` means an operational failure, and `2` means invalid input or environment configuration. The structured error remains available on stdout for nonzero exits.

## Companion skill

The repository includes [`skills/wadb/SKILL.md`](../skills/wadb/SKILL.md). Install or package that directory as an Agent Skill to teach a compatible coding agent when to use `wadb`, how to handle device ambiguity, and which operations require explicit user intent.

### Claude Code

The repository and release archives are also valid Claude Code plugins through [`.claude-plugin/plugin.json`](../.claude-plugin/plugin.json). After installing `wadb` and Android platform-tools, load an extracted archive or clone for one session:

```sh
claude --plugin-dir /path/to/wadb
```

Invoke the plugin skill explicitly with `/wadb:wadb`, or let Claude select it when the request matches the skill description.

For a personal skill without plugin namespacing, copy the same directory into Claude's user-level skill location:

```sh
mkdir -p ~/.claude/skills
cp -R /path/to/wadb/skills/wadb ~/.claude/skills/
```

It is then available as `/wadb` in local Claude Code sessions. The skill teaches Claude how to call the CLI; it does not provide local shell, network, or Android device access to a remote chat session.
