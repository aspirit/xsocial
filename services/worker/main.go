// Package main provides the main entry point for the Worker service.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/aspirit/xsocial/internal/config"
	"github.com/aspirit/xsocial/internal/dep"
	internalerr "github.com/aspirit/xsocial/internal/errors"
	storageeventbroker "github.com/aspirit/xsocial/internal/storage/eventbroker"
	pkgeventbroker "github.com/aspirit/xsocial/pkg/eventbroker"
	"github.com/aspirit/xsocial/pkg/otelutil"
	"github.com/aspirit/xsocial/services/worker/jobs"
	workerwire "github.com/aspirit/xsocial/services/worker/wire"
)

const (
	reconcileDefaultInterval = time.Hour
	// schedulerStopTimeout bounds how long shutdown waits for a running job to
	// reach its next cancellation checkpoint.
	schedulerStopTimeout = 10 * time.Second
)

var errMessageHandlerMissing = errors.New("message handler not configured")

func main() {
	cfg := config.GetConfig()

	baseDeps, err := dep.NewBaseDeps(config.ServiceWorker, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing base dependencies: %v\n", err)
		os.Exit(1)
	}

	workerDeps, err := workerwire.InitializeWorkerDeps(baseDeps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing worker dependencies: %v\n", err)
		baseDeps.Cancel()
		_ = baseDeps.Close()
		os.Exit(1)
	}

	// Initialize OpenTelemetry so background jobs can emit metrics (e.g.
	// reconciliation drift). Best-effort: log and continue if it fails.
	otelCfg := config.GetOtelConfig()
	otelCfg.Logger = baseDeps.Logger
	if otelProvider, otelErr := otelutil.NewOtelProvider(context.Background(), otelCfg); otelErr != nil {
		baseDeps.Logger.Error("Failed to initialize OTel provider; metrics disabled", zap.Error(otelErr))
	} else {
		otelutil.DefaultTelemetry = otelProvider
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			otelProvider.Shutdown(shutdownCtx)
			cancel()
		}()
	}

	// Start periodic reconciliation alongside the event consumer.
	schedulerDone := startReconciliation(baseDeps, workerDeps)

	brokerType := strings.ToLower(strings.TrimSpace(cfg.Storage.NotificationBroker.Type))
	if brokerType == "" {
		brokerType = string(pkgeventbroker.ProviderKafka)
	}

	// Listen for shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		baseDeps.Logger.Info("Received shutdown signal")
		baseDeps.Cancel()
	}()

	var runErr error
	switch brokerType {
	case string(pkgeventbroker.ProviderKafka):
		runErr = runKafkaConsumer(workerDeps, baseDeps)
	case string(pkgeventbroker.ProviderPubSub):
		runErr = runPubSubPull(workerDeps, baseDeps)
	default:
		runErr = fmt.Errorf("%w: notification_broker.type %q (use %s or %s)",
			internalerr.ErrUnsupportedNotificationBrokerType, brokerType, pkgeventbroker.ProviderKafka, pkgeventbroker.ProviderPubSub)
	}

	// Graceful shutdown: stop the scheduler first so no job is still writing when
	// MongoDB and the cache close. A consumer that could not start or died must
	// leave a non-zero exit status: that is what restart policies and deploy
	// gates read.
	baseDeps.Cancel()
	select {
	case <-schedulerDone:
	case <-time.After(schedulerStopTimeout):
		baseDeps.Logger.Warn("Scheduler did not stop in time; closing dependencies anyway")
	}
	if err := baseDeps.Close(); err != nil {
		baseDeps.Logger.Error("Error closing dependencies", zap.Error(err))
		os.Exit(1)
	}
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		baseDeps.Logger.Error("Worker failed", zap.Error(runErr))
		os.Exit(1)
	}

	baseDeps.Logger.Info("Worker stopped")
}

// startReconciliation runs the like-count reconciler on the configured interval
// in the background; it stops when the shutdown context is cancelled.
// The returned channel is closed once the scheduler has stopped (immediately
// when reconciliation is disabled).
func startReconciliation(baseDeps *dep.BaseDeps, deps *workerwire.WorkerDeps) <-chan struct{} {
	done := make(chan struct{})
	rc := baseDeps.Config.WorkerJobs.Reconcile
	if !rc.Enabled {
		baseDeps.Logger.Info("Reconciliation disabled (worker_jobs.reconcile.enabled=false)")
		close(done)

		return done
	}
	interval := rc.Interval
	if interval <= 0 {
		interval = reconcileDefaultInterval
	}

	source := jobs.NewLikeCountSource(deps.Posts, deps.Likes, baseDeps.Cache)
	reconciler := jobs.NewLikeCountReconciler(source, jobs.NewOtelDriftRecorder(otelutil.DefaultTelemetry), baseDeps.Logger, rc.Correct)

	// The lease keeps the job on one replica per interval however many workers run.
	scheduler := jobs.NewScheduler(baseDeps.Logger).WithLeases(deps.Leases, jobs.HolderID())
	scheduler.Register(reconciler, interval)
	go func() {
		defer close(done)
		scheduler.Run(baseDeps.ShutdownCtx)
	}()

	baseDeps.Logger.Info("Reconciliation scheduler started",
		zap.Duration("interval", interval), zap.Bool("correct", rc.Correct))

	return done
}

// runKafkaConsumer runs the Kafka consumer loop until shutdown. It returns an
// error when the consumer cannot be created or stops on its own.
func runKafkaConsumer(deps *workerwire.WorkerDeps, baseDeps *dep.BaseDeps) error {
	if deps.MessageHandler == nil {
		return fmt.Errorf("start consumer: %w", errMessageHandlerMissing)
	}
	if len(baseDeps.Config.Storage.Kafka.Brokers) == 0 {
		return internalerr.ErrKafkaBrokersNotConfigured
	}

	consumer, err := storageeventbroker.NewKafkaConsumer(
		baseDeps.Config.Storage.Kafka,
		deps.MessageHandler,
		baseDeps.Logger,
	)
	if err != nil {
		return fmt.Errorf("create kafka consumer: %w", err)
	}
	defer func() { _ = consumer.Close() }()

	baseDeps.Logger.Info("Worker starting",
		zap.String("mode", string(pkgeventbroker.ProviderKafka)),
		zap.String("topic", storageeventbroker.TopicNotificationEvent),
	)

	if err := consumer.Run(baseDeps.ShutdownCtx); err != nil {
		return fmt.Errorf("kafka consumer: %w", err)
	}

	return nil
}

// runPubSubPull runs the Pub/Sub pull consumer until shutdown (no HTTP endpoint
// needed). It returns an error when the consumer cannot be created or stops on
// its own.
func runPubSubPull(deps *workerwire.WorkerDeps, baseDeps *dep.BaseDeps) error {
	if deps.MessageHandler == nil {
		return fmt.Errorf("start consumer: %w", errMessageHandlerMissing)
	}
	if baseDeps.Config.Storage.NotificationBroker.PubSubProjectID == "" {
		return internalerr.ErrPubSubProjectIDRequired
	}

	consumer, err := storageeventbroker.NewPubSubPullConsumer(
		baseDeps.Config.Storage.NotificationBroker,
		deps.MessageHandler,
		baseDeps.Logger,
	)
	if err != nil {
		return fmt.Errorf("create pub/sub consumer: %w", err)
	}
	defer func() { _ = consumer.Close() }()

	baseDeps.Logger.Info("Worker starting",
		zap.String("mode", string(pkgeventbroker.ProviderPubSub)),
		zap.String("topic", storageeventbroker.TopicNotificationEvent),
	)

	if err := consumer.Run(baseDeps.ShutdownCtx); err != nil {
		return fmt.Errorf("pub/sub consumer: %w", err)
	}

	return nil
}
