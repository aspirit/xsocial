# pkg/idgen — distributed unique IDs

Thin wrapper around [sony/sonyflake](https://github.com/sony/sonyflake) that gives
every service the same `Generator` interface and a configuration vocabulary
(datacenter + worker) that maps onto Sonyflake's single machine id.

## Why Sonyflake

| | Standard Snowflake | Sonyflake |
|---|---|---|
| Lifespan from epoch | ~69 years | ~174 years |
| Machines | 1,024 | 65,536 |
| Time precision | 1 ms | 10 ms |
| IDs per machine per unit | 4,096 / ms | 256 / 10 ms |

An ID is 64 bits: 39 bits of time (10 ms units since the epoch), 8 bits of
sequence, 16 bits of machine id. IDs are time-ordered, so `_id` doubles as a
keyset-pagination cursor (see `internal/storage/mongo/repository/post.go`).

## API

```go
type Generator interface {
	NextID() (uint64, error)                              // the ID
	NextShortID() (string, error)                         // Base62-encoded, for URLs and media keys
	Parse(id uint64) (t time.Time, machineID uint16, seq uint16)
	ParseShortID(shortID string) (uint64, error)
}

// Explicit machine id (0-65535) and optional custom epoch.
gen, err := idgen.NewGenerator(&idgen.Config{MachineID: 33, StartTime: epoch})

// Or datacenter (0-31) + worker (0-31): machine id = datacenter*32 + worker.
gen, err := idgen.NewSnowflakeGeneratorFromDatacenterAndWorker(1, 1, 0)
```

The services build the generator in `internal/wire/providers.go` from
`idgen.snowflake.{datacenter_id, worker_id}` (environment variables
`SNOWFLAKE_DATACENTER_ID` / `SNOWFLAKE_WORKER_ID`).

## Operational rule

**Every running process needs a unique (datacenter, worker) pair.** Two
processes sharing a pair can emit the same ID. The deployment assigns the pair
per replica (the scale-out compose profile in `dev/docker` shows the pattern);
the defaults in `configs/config.default.yaml` are for a single local process.

64-bit IDs exceed JavaScript's safe-integer range, so the API serialises them as
strings.
