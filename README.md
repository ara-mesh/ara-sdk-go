# ara-sdk-go

Go SDK for [Ara](https://github.com/ara-mesh/ara) — a delay-tolerant, offline-first mesh sync library for applications that need shared state without central infrastructure.

## Install

```bash
go get github.com/ara-mesh/ara-sdk-go
```

Requires `CGO_ENABLED=1` and `gcc`. The CRDT engine and SQLite extension are bundled as pre-built platform libraries — no separate downloads needed.

**Supported platforms:** `linux/amd64`, `linux/arm64`, `darwin/arm64`

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

    node.AddTransportUDP(7946)

    node.Exec(ctx,
        "INSERT INTO messages (id, content, created_at) VALUES (?, ?, ?)",
        "msg-1", "Hello mesh", 1000,
    )

    rows, _ := node.Query(ctx, "SELECT id, content FROM messages")
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
| Meshtastic | `AddTransportMeshtastic(portPath, channel)` | LoRa off-grid (220 B packets) |

## Config

```go
ara.Open(ctx, ara.Config{
    Path:            "./ara.db",
    Migrations:      migrations,
    NetworkID:       "hawkesbay-sar",   // scope to a logical mesh; default ""
    Encryption:      true,              // X25519 + AES-256-GCM; default false
    SyncInterval:    60 * time.Second,  // widen on LoRa to preserve duty cycle
    OTLPAddr:        "192.168.1.1:4317",
    OTLPServiceName: "my-app",
    LicenseKey:      "...",             // empty = 10-node evaluation limit
})
```

## Encryption

When `Encryption: true`, each node generates an X25519 keypair. Nodes must be mutually allowlisted before they will exchange data.

```go
// Print this node's public key — share it with operators of peer nodes
fmt.Println(node.PublicKey())

// Add a trusted peer (propagates to all nodes via CRDT sync)
node.AllowPeer(ctx, "<64-char hex key>", "Team 3 tablet")

// Revoke a peer (propagates to all nodes via CRDT sync)
node.RevokePeer(ctx, "<64-char hex key>")
```

## Blobs

Ara can sync binary attachments (photos, files) alongside CRDT data. Blob metadata syncs immediately; bytes are delivered asynchronously based on the sync policy.

```go
// Configure blob storage (call before nodes start exchanging data)
node.SetBlobStore("./blobs", ara.BlobPolicy{
    Mode:        ara.BlobSyncThumbOnly, // BlobSyncNone / BlobSyncThumbOnly / BlobSyncFull
    MaxBytes:    500 << 20,             // 500 MB cap; 0 = unlimited
    MaxBlobSize: 10 << 20,              // skip blobs > 10 MB; 0 = unlimited
})

// Ingest a local file — returns its SHA-256 content ID
id, err := node.IngestBlob(ctx, "/tmp/photo.jpg", "image/jpeg")

// Retrieve the local path once available (empty if not yet fetched)
path := node.BlobPath(id)
```

## Mesh topology

```go
// All known peers with health state
peers, _ := node.Peers(ctx)

// Full topology graph (nodes + directed edges)
graph, _ := node.PeerGraph(ctx)
for _, n := range graph.Nodes {
    log.Printf("node %s health=%s self=%v", n.ID, n.Health, n.Self)
}
for _, e := range graph.Edges {
    log.Printf("%s → %s direct=%v", e.Source, e.Target, e.Direct)
}
```

## OpenTelemetry

```go
// Set at open time via Config.OTLPAddr, or re-point at runtime:
node.InitOTLP(ctx, "192.168.1.1:4317", "my-app")
```

## Schema migrations

Migrations are additive-only. Use `Sync` when creating a new table, `AlterSync` when adding columns to an existing synced table.

```go
[]ara.Migration{
    {
        Version:     1,
        Description: "create items table",
        SQL:         `CREATE TABLE items (id TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '') STRICT;`,
        Sync:        []string{"items"},
    },
    {
        Version:     2,
        Description: "add priority column",
        SQL:         `ALTER TABLE items ADD COLUMN priority INTEGER NOT NULL DEFAULT 0;`,
        AlterSync:   "items",
    },
}
```

## Documentation

Full API reference and guides: [ara-mesh.github.io/ara-docs](https://ara-mesh.github.io/ara-docs)

## License

Proprietary — All Rights Reserved. See [LICENSE](LICENSE).
