package TUM_Live

import (
	"TUM-Live/dao"
	"TUM-Live/model"
	"context"
	"github.com/dgraph-io/ristretto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"io/ioutil"
	"log"
	"testing"
)

func TestMain(m *testing.M) {
	InitDB()
	m.Run()
}

func InitDB() {
	input, err := ioutil.ReadFile("test.db")
	if err != nil {
		log.Fatal(err)
	}
	err = ioutil.WriteFile("/tmp/test.db", input, 0644)
	if err != nil {
		log.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
	if err != nil {
		log.Panic(err)
	}
	if err = db.AutoMigrate(
		&model.User{},
		&model.Student{},
		&model.Course{},
		&model.Chat{},
		&model.RegisterLink{},
		&model.Stat{},
		&model.StreamUnit{},
		&model.Stream{},
		&model.ProcessingJob{},
		&model.Worker{},
	); err != nil {
		log.Panicf("Database can't be migrated: %v", err)
	}
	dao.DB = db
	if cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 10,      // number of keys to track frequency of (1000 for testing).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	}); err != nil {
		log.Panic(err)
	} else {
		dao.Cache = *cache
	}
}

func TestUser(t *testing.T) {
	user, err := dao.GetUserByEmail(context.Background(), "admin@local.host")
	if err != nil {
		t.Fatalf("DB didn't return existing user by email: %v", err)
	}
	if match, err := user.ComparePasswordAndHash("testPassword"); err != nil || !match {
		t.Fatalf("couldn't validate password for admin: %v", err)
	}
}
