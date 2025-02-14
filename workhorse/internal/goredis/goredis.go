package goredis

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	redis "github.com/redis/go-redis/v9"

	"gitlab.com/gitlab-org/gitlab/workhorse/internal/config"
	_ "gitlab.com/gitlab-org/gitlab/workhorse/internal/helper"
	internalredis "gitlab.com/gitlab-org/gitlab/workhorse/internal/redis"
)

var (
	rdb *redis.Client
	// found in https://github.com/redis/go-redis/blob/c7399b6a17d7d3e2a57654528af91349f2468529/sentinel.go#L626
	errSentinelMasterAddr error = errors.New("redis: all sentinels specified in configuration are unreachable")
)

const (
	// Max Idle Connections in the pool.
	defaultMaxIdle = 1
	// Max Active Connections in the pool.
	defaultMaxActive = 1
	// Timeout for Read operations on the pool. 1 second is technically overkill,
	//  it's just for sanity.
	defaultReadTimeout = 1 * time.Second
	// Timeout for Write operations on the pool. 1 second is technically overkill,
	//  it's just for sanity.
	defaultWriteTimeout = 1 * time.Second
	// Timeout before killing Idle connections in the pool. 3 minutes seemed good.
	//  If you _actually_ hit this timeout often, you should consider turning of
	//  redis-support since it's not necessary at that point...
	defaultIdleTimeout = 3 * time.Minute
)

// createDialer references https://github.com/redis/go-redis/blob/b1103e3d436b6fe98813ecbbe1f99dc8d59b06c9/options.go#L214
// it intercepts the error and tracks it via a Prometheus counter
func createDialer(sentinels []string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		var isSentinel bool
		for _, sentinelAddr := range sentinels {
			if sentinelAddr == addr {
				isSentinel = true
				break
			}
		}

		dialTimeout := 5 * time.Second // go-redis default
		destination := "redis"
		if isSentinel {
			// This timeout is recommended for Sentinel-support according to the guidelines.
			//  https://redis.io/topics/sentinel-clients#redis-service-discovery-via-sentinel
			//  For every address it should try to connect to the Sentinel,
			//  using a short timeout (in the order of a few hundreds of milliseconds).
			destination = "sentinel"
			dialTimeout = 500 * time.Millisecond
		}

		netDialer := &net.Dialer{
			Timeout:   dialTimeout,
			KeepAlive: 5 * time.Minute,
		}

		conn, err := netDialer.DialContext(ctx, network, addr)
		if err != nil {
			internalredis.ErrorCounter.WithLabelValues("dial", destination).Inc()
		} else {
			if !isSentinel {
				internalredis.TotalConnections.Inc()
			}
		}

		return conn, err
	}
}

// implements the redis.Hook interface for instrumentation
type sentinelInstrumentationHook struct{}

func (s sentinelInstrumentationHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := next(ctx, network, addr)
		if err != nil && err.Error() == errSentinelMasterAddr.Error() {
			// check for non-dialer error
			internalredis.ErrorCounter.WithLabelValues("master", "sentinel").Inc()
		}
		return conn, err
	}
}

func (s sentinelInstrumentationHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		return next(ctx, cmd)
	}
}

func (s sentinelInstrumentationHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		return next(ctx, cmds)
	}
}

func GetRedisClient() *redis.Client {
	return rdb
}

// Configure redis-connection
func Configure(cfg *config.RedisConfig) error {
	if cfg == nil {
		return nil
	}

	var err error

	if len(cfg.Sentinel) > 0 {
		rdb = configureSentinel(cfg)
	} else {
		rdb, err = configureRedis(cfg)
	}

	return err
}

func configureRedis(cfg *config.RedisConfig) (*redis.Client, error) {
	if cfg.URL.Scheme == "tcp" {
		cfg.URL.Scheme = "redis"
	}

	opt, err := redis.ParseURL(cfg.URL.String())
	if err != nil {
		return nil, err
	}

	opt.DB = getOrDefault(cfg.DB, 0)
	opt.Password = cfg.Password

	opt.PoolSize = getOrDefault(cfg.MaxActive, defaultMaxActive)
	opt.MaxIdleConns = getOrDefault(cfg.MaxIdle, defaultMaxIdle)
	opt.ConnMaxIdleTime = defaultIdleTimeout
	opt.ReadTimeout = defaultReadTimeout
	opt.WriteTimeout = defaultWriteTimeout

	opt.Dialer = createDialer([]string{})

	return redis.NewClient(opt), nil
}

func configureSentinel(cfg *config.RedisConfig) *redis.Client {
	sentinelPassword, sentinels := sentinelOptions(cfg)
	client := redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:       cfg.SentinelMaster,
		SentinelAddrs:    sentinels,
		Password:         cfg.Password,
		SentinelPassword: sentinelPassword,
		DB:               getOrDefault(cfg.DB, 0),

		PoolSize:        getOrDefault(cfg.MaxActive, defaultMaxActive),
		MaxIdleConns:    getOrDefault(cfg.MaxIdle, defaultMaxIdle),
		ConnMaxIdleTime: defaultIdleTimeout,

		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,

		Dialer: createDialer(sentinels),
	})

	client.AddHook(sentinelInstrumentationHook{})

	return client
}

// sentinelOptions extracts the sentinel password and addresses in <host>:<port> format
// the order of priority for the passwords is: SentinelPassword -> first password-in-url
func sentinelOptions(cfg *config.RedisConfig) (string, []string) {
	sentinels := make([]string, len(cfg.Sentinel))
	sentinelPassword := cfg.SentinelPassword

	for i := range cfg.Sentinel {
		sentinelDetails := cfg.Sentinel[i]
		sentinels[i] = fmt.Sprintf("%s:%s", sentinelDetails.Hostname(), sentinelDetails.Port())

		if pw, exist := sentinelDetails.User.Password(); exist && len(sentinelPassword) == 0 {
			// sets password using the first non-empty password
			sentinelPassword = pw
		}
	}

	return sentinelPassword, sentinels
}

func getOrDefault(ptr *int, val int) int {
	if ptr != nil {
		return *ptr
	}
	return val
}
