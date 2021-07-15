package main

import (
	"TUM-Live/api"
	"TUM-Live/dao"
	"TUM-Live/model"
	"TUM-Live/tools"
	"TUM-Live/tools/tum"
	"TUM-Live/web"
	"TUM-Live/api_grpc"
	"context"
	"fmt"
	"github.com/dgraph-io/ristretto"
	"github.com/droundy/goopt"
	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var VersionTag = "development"

const UserKey = "RBG-Default-User" // UserKey key used for storing User struct in context

// GinServer launch gin server
func GinServer() (err error) {
	router := gin.Default()
	// capture performance with sentry
	router.Use(sentrygin.New(sentrygin.Options{Repanic: true}))
	store := cookie.NewStore([]byte(tools.Cfg.CookieStoreSecret))
	router.Use(sessions.Sessions("TUMLiveSessionV5", store))

	router.Use(tools.InitContext)

	// event streams don't work with gzip, configure group without
	chat := router.Group("/api/chat")
	api.ConfigChatRouter(chat)

	router.Use(gzip.Gzip(gzip.DefaultCompression))
	api.ConfigGinRouter(router)
	web.ConfigGinRouter(router)
	err = router.Run(":8081")
	if err != nil {
		sentry.CaptureException(err)
		log.Fatalf("Error starting server, the error is '%v'", err)
	}
	return
}

var (
	OsSignal chan os.Signal
)

// User struct to store database related info in context
type User struct {
	Name string
}

func (u *User) String() string {
	return u.Name
}

func main() {
	web.VersionTag = VersionTag
	OsSignal = make(chan os.Signal, 1)

	goopt.Parse(nil)
	env := "production"
	if VersionTag == "development" {
		env = "development"
	}
	if os.Getenv("SentryDSN") != "" {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:              os.Getenv("SentryDSN"),
			Release:          VersionTag,
			TracesSampleRate: 0.15,
			Debug:            true,
			AttachStacktrace: true,
			Environment:      env,
		})
		if err != nil {
			log.Fatalf("sentry.Init: %s", err)
		}
		// Flush buffered events before the program terminates.
		defer sentry.Flush(2 * time.Second)
		defer sentry.Recover()
	}
	db, err := gorm.Open(mysql.Open(fmt.Sprintf(
		"%v:%v@tcp(db:3306)/%v?parseTime=true&loc=Local",
		tools.Cfg.DatabaseUser,
		tools.Cfg.DatabasePassword,
		tools.Cfg.DatabaseName),
	), &gorm.Config{})
	if err != nil {
		sentry.CaptureException(err)
		sentry.Flush(time.Second * 5)
		log.Fatalf("%v", err)
	}

	err = db.AutoMigrate(
		&model.User{},
		&model.Course{},
		&model.Chat{},
		&model.RegisterLink{},
		&model.Silence{},
		&model.ShortLink{},
		&model.Stat{},
		&model.StreamUnit{},
		&model.LectureHall{},
		&model.Stream{},
		&model.ProcessingJob{},
		&model.Worker{},
		&model.CameraPreset{},
		&model.ServerNotification{},
	)
	if err != nil {
		sentry.CaptureException(err)
		sentry.Flush(time.Second * 5)
		log.Fatalf("%v", err)
	}

	dao.DB = db
	dao.Logger = func(ctx context.Context, sql string) {
		fmt.Printf("SQL: %s\n", sql)
	}

	// tools.SwitchPreset()

	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
		Metrics:     true,
	})
	if err != nil {
		sentry.CaptureException(err)
		sentry.Flush(time.Second * 5)
		log.Fatalf("%v", err)
	}
	dao.Cache = *cache
	initCron()

	go GinServer()
	LoopForever()
}

func initCron() {
	cronService := cron.New()
	//Fetch students every 12 hours
	_, _ = cronService.AddFunc("0 */12 * * *", tum.FetchCourses)
	//Collect livestream stats (viewers) every minute
	_, _ = cronService.AddFunc("0-59 * * * *", api.CollectStats)
	//Flush stale sentry exceptions and transactions every 5 minutes
	_, _ = cronService.AddFunc("0-59/5 * * * *", func() { sentry.Flush(time.Minute * 2) })
	//Look for due streams and notify workers about them
	_, _ = cronService.AddFunc("0-59 * * * *", api_grpc.NotifyWorkers)
	cronService.Start()
}

// LoopForever on signal processing
func LoopForever() {
	fmt.Printf("Entering infinite loop\n")

	signal.Notify(OsSignal, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)
	_ = <-OsSignal

	fmt.Printf("Exiting infinite loop received OsSignal\n")
}
