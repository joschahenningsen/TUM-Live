package api

import (
	"TUM-Live/dao"
	"TUM-Live/tools"
	"context"
	"errors"
	"fmt"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"net/http"
	"strings"
	"time"
)

func configGinStreamRestRouter(router *gin.Engine) {
	g := router.Group("/")
	g.Use(tools.InitStream)
	g.Use(tools.AdminOfCourse)
	g.GET("/api/stream/:streamID", getStream)
}

func getStream(c *gin.Context) {
	foundContext, exists := c.Get("TUMLiveContext")
	if !exists {
		sentry.CaptureException(errors.New("context should exist but doesn't"))
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	tumLiveContext := foundContext.(tools.TUMLiveContext)
	stream := *tumLiveContext.Stream
	course := *tumLiveContext.Course
	c.JSON(http.StatusOK,
		gin.H{"course": course.Name,
			"courseID":    course.ID,
			"streamID":    stream.ID,
			"name":        stream.Name,
			"description": stream.Description,
			"start":       stream.Start,
			"end":         stream.End,
			"ingest":      fmt.Sprintf("%sstream?secret=%s", tools.Cfg.IngestBase, stream.StreamKey),
			"live":        stream.LiveNow,
			"vod":         stream.Recording})
}

//deprecated
func configGinStreamAuthRouter(router gin.IRoutes) {
	router.POST("/stream-management/on_publish", StartStream)
	router.POST("/stream-management/on_publish_done", EndStream)
}

//StartStream
// This function is called when a user attempts to push a stream to the server.
// w response writer. Status code determines wether streaming is approved: 200 if yes, 402 otherwise.
// r request. Form if valid: POST /on_publish/app/kurs-key example: {/on_publish/eidi-3zt45z452h4754nj2q74}
// deprecated
func StartStream(c *gin.Context) {
	_ = c.Request.ParseForm()
	slug := c.Request.FormValue("name")
	key := strings.Split(c.Request.FormValue("tcurl"), "?secret=")[1] // this could be nicer.
	println(slug + ":" + key)
	res, err := dao.GetStreamByKey(context.Background(), key)
	if err != nil {
		c.AbortWithStatus(http.StatusForbidden)
		fmt.Printf("stream rejected. cause: %v\n", err)
		return
	}
	// reject streams that are more than 30 minutes in the future or more than 30 minutes past
	if !(time.Now().After(res.Start.Add(time.Minute*-30)) && time.Now().Before(res.End.Add(time.Minute*30))) {
		c.AbortWithStatus(http.StatusForbidden)
		log.WithFields(log.Fields{"streamId": res.ID}).Info("Stream rejected, time out of bounds")
		return
	}
	fmt.Printf("stream approved: id=%d\n", res.ID)
	err = dao.SetStreamLive(context.Background(), key, tools.Cfg.LrzServerHls+slug+"/playlist.m3u8")
	if err != nil {
		log.Printf("Couldn't create live stream: %v\n", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusOK)
}

//deprecated
func EndStream(c *gin.Context) {
	_ = c.Request.ParseForm()
	key := strings.Split(c.Request.FormValue("tcurl"), "?secret=")[1] // this could be nicer.
	_ = dao.SetStreamNotLive(context.Background(), key)
}
