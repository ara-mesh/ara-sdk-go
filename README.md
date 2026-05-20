# ara-sdk-go

Go SDK for [Ara](https://github.com/ara-mesh/ara) — a delay-tolerant, offline-first mesh sync library for applications that need shared state without central infrastructure.

## Install

```bash
go get github.com/ara-mesh/ara-sdk-go
```

Requires `CGO_ENABLED=1` and `gcc`. The CRDT engine and SQLite extension are bundled as pre-built platform libraries — no separate downloads needed.

**Supported platforms:** `linux/amd64`, `linux/arm64`, `darwin/arm64`, `darwin/amd64`

## Quick start

```go
package main

import (
    "context"
    "log"

    ara "github.com/ara-mesh/ara-sdk-go"
)

var migrations = []ara.Migration{
    {
        Version:     1,
        Description: "messages table",
        SQL: `CREATE TABLE IF NOT EXISTS messages (
            id         TEXT PRIMARY KEY,
            content    TEXT NOT NULL DEFAULT '',
            created_at INTEGER NOT NULL DEFAULT 0
        ) STRICT;`,
        Sync: []string{"messages"},
    },
}

func main() {
    ctx := context.Background()

    node, err := ara.Open(ctx, ara.Config{
        Path:       "./ara.db",
        Migrations: migrations,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer node.Close()

    // Add a UDP transport for LAN peer discovery
    node.AddTransportUDP(7946)

    // Write
    node.Exec(ctx,
        "INSERT INTO messages (id, content, created_at) VALUES (?, ?, ?)",
        "msg-1", "Hello mesh", 1000,
    )

    // Read
    rows, err := node.Query(ctx, "SELECT id, content FROM messages")
    for _, row := range rows {
        log.Printf("%s: %s", row["id"], row["content"])
    }
}
```

## Build & test

```bash
CGO_ENABLED=1 go build ./...
CGO_ENABLED=1 go test ./...
```

## Transports

| Transport | Method | Use case |
|-----------|--------|----------|
| UDP LAN | `AddTransportUDP(port, seeds...)` | Local network / WiFi |
| MQTT | `AddTransportMQTT(MQTTConfig)` | WiFi or cellular via broker |
| Meshtastic | `AddTransportMeshtastic(port, channel)` | LoRa off-grid (220 B packets) |

## Documentation

Full API reference and guides: [ara-mesh.github.io/ara-docs](https://ara-mesh.github.io/ara-docs)

## License

Proprietary — All Rights Reserved. See [LICENSE](LICENSE).
