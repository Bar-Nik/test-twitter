package main

import (
	"context"
	"database/sql"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"twitter/cmd/back/internal/api"
	"twitter/cmd/back/internal/cache"
	"twitter/cmd/back/internal/producer"
	"twitter/cmd/back/internal/repo"
	"twitter/internal/logger"
	"twitter/internal/metrics"
	"twitter/internal/rabbitmq"

	pb "twitter/api/proto/v1"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpc_run "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
)

type Config struct {
	DSN               string        `yaml:"dsn"`
	Host              string        `yaml:"host"`
	HostGRPC          string        `yaml:"host_grpc"`
	MigrateDir        string        `yaml:"migrate_dir"`
	Driver            string        `yaml:"driver"`
	LogLevel          int           `yaml:"loglevel"`
	TimeOut           time.Duration `yaml:"timeout"`
	TokenJwtTTl       time.Duration `yaml:"token_jwt_ttl"`
	JwtSecret         string        `yaml:"jwt_secret"`
	AddrCache         string        `yaml:"addr_cache"`
	PasswordCache     string        `yaml:"password_cache"`
	DBCacheTweet      int           `yaml:"db_cache_tweet"`
	DBCacheUserTweets int           `yaml:"db_cache_user_tweets"`
	HostRBMQ          string        `yaml:"host_rbmq"`
	PortRBMQ          string        `yaml:"port_rbmq"`
	UserNameRBMQ      string        `yaml:"username_rbmq"`
	PasswordRBMQ      string        `yaml:"password_rbmq"`
	VHostRBMQ         string        `yaml:"vhost_rbmq"`
}

func main() {

	yamlConfig, err := os.ReadFile("./config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	var cfg Config
	err = yaml.Unmarshal(yamlConfig, &cfg)
	if err != nil {
		log.Fatal(err)
	}

	rabbit, err := rabbitmq.NewRabbitMQClient(cfg.HostRBMQ, cfg.PortRBMQ, cfg.UserNameRBMQ, cfg.PasswordRBMQ, cfg.VHostRBMQ)
	if err != nil {
		log.Fatal(err)
	}
	defer rabbit.Close()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.Level(cfg.LogLevel),
	}))

	ctxParent := logger.NewContext(context.Background(), log)

	ctx, cancel := signal.NotifyContext(ctxParent, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGABRT, syscall.SIGTERM)
	defer cancel()
	go forceShutdown(ctx)

	producer := producer.NewProducer(rabbit.Ch)

	migrator, err := migrate.New(cfg.MigrateDir, cfg.DSN)
	if err != nil {
		// fmt.Println(err)
		log.Error(err.Error())
	}

	if err := migrator.Up(); err != nil && err != migrate.ErrNoChange {
		// fmt.Println(err)
		log.Error(err.Error())
	}

	rowSQLConn, err := sql.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		// fmt.Println(err)
		log.Error(err.Error())
	}

	repo := repo.NewRepository(rowSQLConn)

	redisClientTweets := cache.NewRedisClient(cfg.AddrCache, cfg.PasswordCache, cfg.DBCacheTweet)

	if err := redisClientTweets.Connect(ctx); err != nil {
		log.Error("RedisTweet - not connected")
	} else {
		log.Warn("RedisTweet - connected")
	}

	defer redisClientTweets.Close()

	redisClientUserTweets := cache.NewRedisClient(cfg.AddrCache, cfg.PasswordCache, cfg.DBCacheUserTweets)

	if err := redisClientUserTweets.Connect(ctx); err != nil {
		log.Error("RedisUserTweets - not connected")
	} else {
		log.Warn("RedisUserTweets - connected")
	}

	defer redisClientUserTweets.Close()

	twitterGrpcServer := api.GrpcServer{
		Database:          repo,
		JwtSecret:         cfg.JwtSecret,
		CacheDBTweets:     redisClientTweets,
		CacheDBUserTweets: redisClientUserTweets,
		Producer:          producer,
	}
	ln, err := net.Listen("tcp", cfg.HostGRPC)
	if err != nil {
		// fmt.Println(err)
		log.Error(err.Error())
	}

	loggingOpts := []logging.Option{
		logging.WithLogOnEvents(
			logging.StartCall,       // --
			logging.FinishCall,      // --
			logging.PayloadReceived, // --
			logging.PayloadSent,     // --
		),
	}

	// Запускаем сервер метрик на порту 9090
	StartMetricsServer(":9090")

	server := grpc.NewServer(
		grpc.Creds(insecure.NewCredentials()),
		grpc.ChainUnaryInterceptor(
			logging.UnaryServerInterceptor(interceptorLogger(log), loggingOpts...),
			MetricsInterceptor(),
			api.AuthInterceptor(cfg.JwtSecret),
		),
	)
	pb.RegisterTwitterAPIServer(server, &twitterGrpcServer)

	log.Warn("GRPC server - started")
	go func() {
		if err = server.Serve(ln); err != nil {
			// fmt.Println(err)
			log.Error(err.Error())
		}
	}()

	conn, err := grpc.NewClient(cfg.HostGRPC,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		// fmt.Println(err)
		log.Error(err.Error())
	}
	defer conn.Close()

	// HTTP сервер (gRPC-gateway) с middleware
	gw := grpc_run.NewServeMux()
	err = pb.RegisterTwitterAPIHandler(context.TODO(), gw, conn)
	if err != nil {
		// fmt.Println(err)
		log.Error(err.Error())
	}

	wrappedMux := api.MetricsMiddleware(gw)
	gwServer := &http.Server{
		Addr:    cfg.Host,
		Handler: wrappedMux,
	}

	log.Warn("GRPC-GW server - started")
	err = gwServer.ListenAndServe()
	if err != nil {
		// fmt.Println(err)
		log.Error(err.Error())
	}

}

func interceptorLogger(l *slog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		l.Log(ctx, slog.Level(lvl), msg, fields...)
	})
}

func MetricsInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		// Выполняем запрос
		resp, err := handler(ctx, req)

		duration := time.Since(start).Seconds()
		statusCode := status.Code(err).String()

		// Записываем метрики
		metrics.GrpcRequestsTotal.WithLabelValues(info.FullMethod, statusCode).Inc()
		metrics.GrpcRequestDuration.WithLabelValues(info.FullMethod).Observe(duration)

		return resp, err
	}
}

func StartMetricsServer(addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	go func() {
		log.Printf("Starting metrics server on %s", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatalf("Failed to start metrics server: %v", err)
		}
	}()
}

func forceShutdown(ctx context.Context) {
	log := logger.FromContext(ctx)
	const shutdownDelay = 15 * time.Second

	<-ctx.Done()
	time.Sleep(shutdownDelay)

	log.Error("failed to graceful shutdown")
	os.Exit(1)
}
