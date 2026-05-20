// Package ara is the Ara mesh sync SDK for Go.
//
// It wraps a pre-compiled Ara engine (libaraengine.a) and exposes the same
// API as the open-source sdk/v1 package. Applications that later obtain the
// full source can switch the import path with no other changes.
//
// # Quick start
//
//	import ara "github.com/ara-mesh/ara-sdk-go"
//
//	var migrations = []ara.Migration{
//	    {
//	        Version:     1,
//	        Description: "create items table",
//	        SQL:         `CREATE TABLE items (id TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '') STRICT;`,
//	        Sync:        []string{"items"},
//	    },
//	}
//
//	node, err := ara.Open(ctx, ara.Config{
//	    Path:       "./myapp.db",
//	    Migrations: migrations,
//	})
//	defer node.Close()
//
//	node.AddTransportUDP(7946)
//	node.Exec(ctx, `INSERT INTO items (id, name) VALUES (?, ?)`, "a1", "hello")
//
// CGO_ENABLED=1 is required at build time.
package ara

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unsafe"
)

/*
#include "include/libaraengine.h"
#include <stdlib.h>
*/
import "C"

// ── public types ─────────────────────────────────────────────────────────────

// Config holds the configuration for opening an Ara node.
type Config struct {
	// Path is the SQLite database file path. Use ":memory:" for an in-memory node.
	Path string

	// Migrations is the ordered list of schema migrations to apply on open.
	Migrations []Migration

	// NetworkID scopes this node to a logical mesh. Only nodes sharing the same
	// NetworkID exchange handshakes and changesets. Empty string (default) is a
	// valid network — all empty-NetworkID nodes sync with each other.
	NetworkID string

	// Encryption enables per-node X25519 keypairs, a CRDT-synced peer allowlist,
	// and AES-256-GCM encryption on all transport messages.
	// When false (default) nodes operate in plaintext mode.
	Encryption bool

	// SyncInterval overrides the periodic handshake broadcast interval.
	// Default is 30s. Set higher (e.g. 60–120s) on LoRa-only nodes to preserve
	// duty cycle budget; set lower (e.g. 10s) on WiFi/LAN-only nodes for faster
	// convergence. Zero uses the default.
	SyncInterval time.Duration

	// OTLPAddr is an optional OpenTelemetry collector gRPC endpoint ("host:port").
	OTLPAddr string

	// OTLPServiceName is the OTel service name. Defaults to "ara-go".
	OTLPServiceName string

	// LicenseKey is an Ed25519-signed license key issued by Ara. Leave empty
	// to run in evaluation mode (10-node mesh limit, all features enabled).
	LicenseKey string
}

// Migration declares a versioned schema change and which tables to replicate.
type Migration struct {
	Version     int
	Description string
	// SQL is the DDL to execute (CREATE TABLE, ALTER TABLE, etc.).
	SQL string
	// Sync lists the table names to register as CRDT-synced after this migration.
	Sync []string
	// AlterSync names a table whose CRDT triggers should be rebuilt after an
	// ALTER TABLE in SQL (for adding columns to an existing synced table).
	AlterSync string
}

// MQTTConfig configures an MQTT transport.
type MQTTConfig struct {
	BrokerURL string // e.g. "tcp://192.168.1.1:1883"
	NetworkID string // logical mesh network identifier
}

// BlobSyncMode controls whether a node automatically fetches blob bytes from peers.
type BlobSyncMode int

const (
	BlobSyncNone      BlobSyncMode = 0 // metadata only; default
	BlobSyncThumbOnly BlobSyncMode = 1 // thumbnails ≤ 2 KB only
	BlobSyncFull      BlobSyncMode = 2 // pull full blobs when transport allows
)

// BlobPolicy configures automatic blob replication.
type BlobPolicy struct {
	Mode        BlobSyncMode
	MaxBytes    int64 // total storage cap; 0 = unlimited
	MaxBlobSize int64 // skip blobs larger than this; 0 = unlimited
}

// PeerInfo describes a discovered peer node.
type PeerInfo struct {
	ID            string   `json:"ID"`
	SchemaVersion int      `json:"SchemaVersion"`
	Transports    []string `json:"Transports"`
	Health        string   `json:"Health"`
}

// ── node ─────────────────────────────────────────────────────────────────────

// Node is an open Ara sync node. Create one with [Open]; close with [Close].
// All methods are safe to call from multiple goroutines.
type Node struct {
	h C.longlong
}

// Open opens or creates an Ara node at the given path. The run loop starts
// automatically in the background; there is no separate Run call.
func Open(_ context.Context, cfg Config) (*Node, error) {
	migrationsJSON, err := marshalMigrations(cfg.Migrations)
	if err != nil {
		return nil, fmt.Errorf("ara: marshal migrations: %w", err)
	}

	pathC := C.CString(cfg.Path)
	defer C.free(unsafe.Pointer(pathC))
	crsqlC := C.CString("")
	defer C.free(unsafe.Pointer(crsqlC))
	migrC := C.CString(migrationsJSON)
	defer C.free(unsafe.Pointer(migrC))
	networkIDC := C.CString(cfg.NetworkID)
	defer C.free(unsafe.Pointer(networkIDC))
	encryptionC := C.int(0)
	if cfg.Encryption {
		encryptionC = 1
	}
	licenseKeyC := C.CString(cfg.LicenseKey)
	defer C.free(unsafe.Pointer(licenseKeyC))

	h := C.AraOpen(pathC, crsqlC, migrC, networkIDC, encryptionC, licenseKeyC)
	if h < 0 {
		return nil, errors.New("ara: failed to open node")
	}
	n := &Node{h: h}

	if cfg.SyncInterval > 0 {
		C.AraSetSyncInterval(h, C.int(int(cfg.SyncInterval.Seconds())))
	}

	if cfg.OTLPAddr != "" {
		svcName := cfg.OTLPServiceName
		if svcName == "" {
			svcName = "ara-go"
		}
		addrC := C.CString(cfg.OTLPAddr)
		defer C.free(unsafe.Pointer(addrC))
		svcC := C.CString(svcName)
		defer C.free(unsafe.Pointer(svcC))
		if errC := C.AraInitOTLP(h, addrC, svcC); errC != nil {
			msg := C.GoString(errC)
			C.AraFree(errC)
			return nil, fmt.Errorf("ara: init OTLP: %s", msg)
		}
	}

	return n, nil
}

// Close stops the run loop and closes the database.
func (n *Node) Close() error {
	C.AraClose(n.h)
	return nil
}

// NodeID returns this node's UUID string.
func (n *Node) NodeID() string {
	c := C.AraNodeID(n.h)
	s := C.GoString(c)
	C.AraFree(c)
	return s
}

// SchemaVersion returns the highest applied migration version.
func (n *Node) SchemaVersion() int {
	return int(C.AraSchemaVersion(n.h))
}

// Exec executes a write SQL statement with optional bind args.
func (n *Node) Exec(_ context.Context, query string, args ...any) error {
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("ara: marshal args: %w", err)
	}
	queryC := C.CString(query)
	defer C.free(unsafe.Pointer(queryC))
	argsC := C.CString(string(argsJSON))
	defer C.free(unsafe.Pointer(argsC))

	if errC := C.AraExec(n.h, queryC, argsC); errC != nil {
		msg := C.GoString(errC)
		C.AraFree(errC)
		return errors.New(msg)
	}
	return nil
}

// Query executes a read SQL statement and returns all rows as a slice of
// column-name → value maps. JSON numbers appear as float64; use [Row.Get]
// for type-safe access to individual values.
func (n *Node) Query(_ context.Context, query string, args ...any) ([]map[string]any, error) {
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("ara: marshal args: %w", err)
	}
	queryC := C.CString(query)
	defer C.free(unsafe.Pointer(queryC))
	argsC := C.CString(string(argsJSON))
	defer C.free(unsafe.Pointer(argsC))

	c := C.AraQuery(n.h, queryC, argsC)
	s := C.GoString(c)
	C.AraFree(c)

	if strings.HasPrefix(s, `{"error"`) {
		var e struct{ Error string `json:"error"` }
		json.Unmarshal([]byte(s), &e) //nolint:errcheck
		return nil, errors.New(e.Error)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(s), &rows); err != nil {
		return nil, fmt.Errorf("ara: decode query result: %w", err)
	}
	return rows, nil
}

// QueryRow executes a query and returns the first row. Call [Row.Get] to
// read individual columns, or [Row.Map] for the full row map.
// If no rows are returned, [Row.Err] returns [ErrNoRows].
func (n *Node) QueryRow(_ context.Context, query string, args ...any) *Row {
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return &Row{err: fmt.Errorf("ara: marshal args: %w", err)}
	}
	queryC := C.CString(query)
	defer C.free(unsafe.Pointer(queryC))
	argsC := C.CString(string(argsJSON))
	defer C.free(unsafe.Pointer(argsC))

	c := C.AraQueryRow(n.h, queryC, argsC)
	s := C.GoString(c)
	C.AraFree(c)

	if s == "null" {
		return &Row{err: ErrNoRows}
	}
	if strings.HasPrefix(s, `{"error"`) {
		var e struct{ Error string `json:"error"` }
		json.Unmarshal([]byte(s), &e) //nolint:errcheck
		return &Row{err: errors.New(e.Error)}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return &Row{err: fmt.Errorf("ara: decode row: %w", err)}
	}
	return &Row{m: m}
}

// Sync triggers an immediate sync broadcast to all reachable peers.
func (n *Node) Sync(_ context.Context) error {
	if errC := C.AraSync(n.h); errC != nil {
		msg := C.GoString(errC)
		C.AraFree(errC)
		return errors.New(msg)
	}
	return nil
}

// Peers returns the set of peers this node currently knows about.
func (n *Node) Peers(_ context.Context) ([]PeerInfo, error) {
	c := C.AraPeers(n.h)
	s := C.GoString(c)
	C.AraFree(c)

	if strings.HasPrefix(s, `{"error"`) {
		var e struct{ Error string `json:"error"` }
		json.Unmarshal([]byte(s), &e) //nolint:errcheck
		return nil, errors.New(e.Error)
	}
	var peers []PeerInfo
	if err := json.Unmarshal([]byte(s), &peers); err != nil {
		return nil, fmt.Errorf("ara: decode peers: %w", err)
	}
	return peers, nil
}

// AddTransportUDP adds a UDP LAN transport. seeds is an optional list of
// "host:port" addresses for peers on networks where multicast does not work
// (e.g. macOS loopback, cross-subnet). port must be unique per node on localhost.
func (n *Node) AddTransportUDP(port int, seeds ...string) error {
	if len(seeds) == 0 {
		if errC := C.AraAddTransportUDP(n.h, C.int(port)); errC != nil {
			msg := C.GoString(errC)
			C.AraFree(errC)
			return errors.New(msg)
		}
		return nil
	}
	seedsJSON, err := json.Marshal(seeds)
	if err != nil {
		return fmt.Errorf("ara: marshal seeds: %w", err)
	}
	seedsC := C.CString(string(seedsJSON))
	defer C.free(unsafe.Pointer(seedsC))
	if errC := C.AraAddTransportUDPSeeds(n.h, C.int(port), seedsC); errC != nil {
		msg := C.GoString(errC)
		C.AraFree(errC)
		return errors.New(msg)
	}
	return nil
}

// AddTransportMQTT adds an MQTT transport.
func (n *Node) AddTransportMQTT(cfg MQTTConfig) error {
	b, err := json.Marshal(map[string]string{
		"broker_url": cfg.BrokerURL,
		"network_id": cfg.NetworkID,
	})
	if err != nil {
		return fmt.Errorf("ara: marshal mqtt config: %w", err)
	}
	cfgC := C.CString(string(b))
	defer C.free(unsafe.Pointer(cfgC))
	if errC := C.AraAddTransportMQTT(n.h, cfgC); errC != nil {
		msg := C.GoString(errC)
		C.AraFree(errC)
		return errors.New(msg)
	}
	return nil
}

// SetBlobStore configures a local blob store directory and sync policy.
// Call before the node starts producing or receiving blobs.
func (n *Node) SetBlobStore(dir string, policy BlobPolicy) error {
	dirC := C.CString(dir)
	defer C.free(unsafe.Pointer(dirC))
	if errC := C.AraSetBlobDir(n.h, dirC, C.int(policy.Mode), C.longlong(policy.MaxBytes), C.longlong(policy.MaxBlobSize)); errC != nil {
		msg := C.GoString(errC)
		C.AraFree(errC)
		return errors.New(msg)
	}
	return nil
}

// IngestBlob copies a local file into the blob store and returns its SHA-256
// content id. mimeType may be empty (defaults to "application/octet-stream").
func (n *Node) IngestBlob(_ context.Context, path, mimeType string) (string, error) {
	pathC := C.CString(path)
	defer C.free(unsafe.Pointer(pathC))
	mimeC := C.CString(mimeType)
	defer C.free(unsafe.Pointer(mimeC))

	c := C.AraBlobIngest(n.h, pathC, mimeC)
	s := C.GoString(c)
	C.AraFree(c)

	if strings.HasPrefix(s, `{"error"`) {
		var e struct{ Error string `json:"error"` }
		json.Unmarshal([]byte(s), &e) //nolint:errcheck
		return "", errors.New(e.Error)
	}
	return s, nil
}

// BlobPath returns the local filesystem path of a stored blob, or "" if the
// blob is not yet available on this node.
func (n *Node) BlobPath(id string) string {
	idC := C.CString(id)
	defer C.free(unsafe.Pointer(idC))
	c := C.AraBlobPath(n.h, idC)
	s := C.GoString(c)
	C.AraFree(c)
	return s
}

// ── internal helpers ─────────────────────────────────────────────────────────

type migrationJSON struct {
	Version     int      `json:"version"`
	Description string   `json:"description"`
	SQL         string   `json:"sql,omitempty"`
	Sync        []string `json:"sync,omitempty"`
	AlterSync   string   `json:"alter_sync,omitempty"`
}

func marshalMigrations(ms []Migration) (string, error) {
	if len(ms) == 0 {
		return "[]", nil
	}
	out := make([]migrationJSON, len(ms))
	for i, m := range ms {
		out[i] = migrationJSON{
			Version:     m.Version,
			Description: m.Description,
			SQL:         m.SQL,
			Sync:        m.Sync,
			AlterSync:   m.AlterSync,
		}
	}
	b, err := json.Marshal(out)
	return string(b), err
}
