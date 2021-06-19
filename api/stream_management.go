package api

import (
	"TUM-Live/dao"
	"TUM-Live/model"
	"TUM-Live/tools"
	"context"
	"errors"
	"fmt"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"strings"
	"time"
)

func configGinStreamRestRouter(router *gin.Engine) {
	g := router.Group("/")
	g.Use(tools.InitStream)
	g.Use(tools.AdminOfCourse)
	g.GET("/api/stream/:streamID", getStream)
	g.POST("/api/stream/chatMode/:streamID", setStreamChatMode)
}

func setStreamChatMode(c *gin.Context) {
	var req chatMode
	if err := c.BindJSON(&req); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	err := dao.SetChatMode(c.Param("streamID"), req.Moderated)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
}

type chatMode struct {
	Moderated bool `json:"moderated"`
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
	c.JSON(http.StatusOK, gin.H{"course": course.Name, "name": stream.Name, "start": stream.Start, "end": stream.End, "key": stream.StreamKey, "live": stream.LiveNow, "vod": stream.Recording})
}

func configGinStreamAuthRouter(router gin.IRoutes) {
	router.POST("/stream-management/on_publish", StartStream)
	router.POST("/stream-management/on_publish_done", EndStream)
	router.POST("/stream-management/on_record_done", OnRecordingFinished)
}

/*StartStream
* This function is called when a user attempts to push a stream to the server.
* @w: response writer. Status code determines wether streaming is approved: 200 if yes, 402 otherwise.
* @r: request. Form if valid: POST /on_publish/app/kurs-key example: {/on_publish/eidi-3zt45z452h4754nj2q74}
 */
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
	fmt.Printf("stream approved: id=%d\n", res.ID)
	err = dao.SetStreamLive(context.Background(), key, tools.Cfg.LrzServerHls+slug+"/playlist.m3u8")
	if err != nil {
		log.Printf("Couldn't create live stream: %v\n", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusOK)
}

func EndStream(c *gin.Context) {
	_ = c.Request.ParseForm()
	key := strings.Split(c.Request.FormValue("tcurl"), "?secret=")[1] // this could be nicer.
	_ = dao.SetStreamNotLive(context.Background(), key)
}

func OnRecordingFinished(c *gin.Context) {
	_ = c.Request.ParseForm()
	key := strings.Split(c.Request.FormValue("tcurl"), "?secret=")[1] // this could be nicer.
	filepath := c.Request.FormValue("path")
	_ = dao.SetStreamNotLive(context.Background(), key)
	stream, err := dao.GetStreamByKey(context.Background(), key)
	if err != nil {
		log.Printf("invalid end stream request. Weird %v\n", err)
		return
	}
	var convertJob = model.ProcessingJob{
		FilePath:    filepath,
		StreamID:    stream.ID,
		AvailableAt: time.Now().Add(time.Hour * 2),
	}
	dao.InsertConvertJob(context.Background(), &convertJob)
}
