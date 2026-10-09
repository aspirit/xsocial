// Package wire provides the Wire providers and provider sets shared by every
// service (api, worker). Services compose these sets in their own injectors
// (services/*/wire/wire.go); `task gen:wire` generates the constructors and CI
// fails if wire_gen.go is stale.
package wire

import (
	"fmt"
	"time"

	"github.com/google/wire"

	"github.com/aspirit/xsocial/internal/config"
	"github.com/aspirit/xsocial/internal/dep"
	"github.com/aspirit/xsocial/internal/domains/auth"
	"github.com/aspirit/xsocial/internal/domains/notification"
	"github.com/aspirit/xsocial/internal/domains/post"
	storageeventbroker "github.com/aspirit/xsocial/internal/storage/eventbroker"
	"github.com/aspirit/xsocial/internal/storage/interfaces"
	mongostorage "github.com/aspirit/xsocial/internal/storage/mongo"
	"github.com/aspirit/xsocial/internal/storage/mongo/repository"
	"github.com/aspirit/xsocial/pkg/cache"
	pkgeventbroker "github.com/aspirit/xsocial/pkg/eventbroker"
	"github.com/aspirit/xsocial/pkg/idgen"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// InfraSet provides configuration, IDs and the shared cache.
var InfraSet = wire.NewSet(
	ProvideConfig,
	ProvideJWTConfig,
	ProvideSnowflakeIDGenerator,
	ProvideCache,
)

// RepositorySet provides the MongoDB repositories bound to their interfaces, so
// domains only ever see interfaces.* (swap storage or fake it in tests).
var RepositorySet = wire.NewSet(
	ProvideCoreDatabase,
	repository.NewUserInfoRepository,
	repository.NewPostRepository,
	repository.NewPostLikeRepository,
	repository.NewNotificationRepository,
	repository.NewJobLeaseRepository,
	wire.Bind(new(interfaces.IUserInfoRepository), new(*repository.UserInfoRepository)),
	wire.Bind(new(interfaces.IPostRepository), new(*repository.PostRepository)),
	wire.Bind(new(interfaces.IPostLikeRepository), new(*repository.PostLikeRepository)),
	wire.Bind(new(interfaces.INotificationRepository), new(*repository.NotificationRepository)),
	wire.Bind(new(interfaces.IJobLeaseRepository), new(*repository.JobLeaseRepository)),
)

// DomainSet provides the domain services.
var DomainSet = wire.NewSet(
	auth.NewAuthBiz,
	post.NewPostBiz,
	notification.NewNotificationEventBiz,
	ProvideNotificationMergeConfig,
	notification.NewNotificationBiz,
)

// ProvideConfig returns the application config from BaseDeps.
func ProvideConfig(baseDeps *dep.BaseDeps) *config.Config {
	return baseDeps.Config
}

// ProvideJWTConfig exposes the JWT section to the auth domain.
func ProvideJWTConfig(cfg *config.Config) config.JWTConfig {
	return cfg.Secrets.JWT
}

// ProvideCache returns the shared cache (nil when no cache is configured; every
// consumer treats nil as "caching disabled").
func ProvideCache(baseDeps *dep.BaseDeps) cache.Cache {
	return baseDeps.Cache
}

// ProvideCoreDatabase returns the core MongoDB database.
func ProvideCoreDatabase(baseDeps *dep.BaseDeps) (*mongo.Database, error) {
	db, err := baseDeps.GetDatabase(mongostorage.DBCore)
	if err != nil {
		return nil, fmt.Errorf("core database: %w", err)
	}

	return db, nil
}

// ProvideEventPublisher creates the process-wide event publisher (Kafka or
// Pub/Sub, selected by storage.notification_broker.type). One producer per
// process: it is goroutine-safe and must be closed on shutdown.
func ProvideEventPublisher(baseDeps *dep.BaseDeps) (pkgeventbroker.Publisher, error) {
	pub, err := storageeventbroker.NewEventBrokerPublisher(baseDeps.Config, baseDeps.Logger)
	if err != nil {
		return nil, fmt.Errorf("create event publisher: %w", err)
	}

	return pub, nil
}

// ProvideNotificationMergeConfig builds the inbox merge settings from config.
func ProvideNotificationMergeConfig(cfg *config.Config) *notification.MergeConfig {
	return &notification.MergeConfig{WhoMaxSize: cfg.WorkerJobs.Notification.WhoMaxSize}
}

// ProvideSnowflakeIDGenerator provides the Snowflake ID generator.
func ProvideSnowflakeIDGenerator(baseDeps *dep.BaseDeps) (idgen.Generator, error) {
	cfg := baseDeps.Config
	snowflakeConfig := cfg.IDGen.Snowflake

	var startTime time.Time
	if snowflakeConfig.StartTime != "" {
		var err error
		startTime, err = time.Parse(time.RFC3339, snowflakeConfig.StartTime)
		if err != nil {
			return nil, fmt.Errorf("invalid start_time format: %w", err)
		}
	} else if snowflakeConfig.Epoch > 0 {
		startTime = time.Unix(snowflakeConfig.Epoch/1000, (snowflakeConfig.Epoch%1000)*1000000)
	}

	if snowflakeConfig.MachineID > 0 {
		gen, err := idgen.NewGenerator(&idgen.Config{
			MachineID: snowflakeConfig.MachineID,
			StartTime: startTime,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create snowflake generator: %w", err)
		}

		return gen, nil
	}

	datacenterID := snowflakeConfig.DatacenterID
	workerID := snowflakeConfig.WorkerID
	if datacenterID == 0 && workerID == 0 {
		datacenterID = 1
		workerID = 1
	}

	var epoch int64
	if !startTime.IsZero() {
		epoch = startTime.UnixMilli()
	}

	gen, err := idgen.NewSnowflakeGeneratorFromDatacenterAndWorker(
		datacenterID,
		workerID,
		epoch,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create snowflake generator: %w", err)
	}

	return gen, nil
}
