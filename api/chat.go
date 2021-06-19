package api

import (
	"TUM-Live/dao"
	"TUM-Live/model"
	"TUM-Live/tools"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

var m *melody.Melody
var stats = map[string]int{}
var statsLock = sync.RWMutex{}

func configGinChatRouter(router *gin.RouterGroup) {
	wsGroup := router.Group("/:streamID")
	wsGroup.Use(tools.InitStream)
	wsGroup.GET("/ws", ChatStream)
	if m == nil {
		log.Printf("creating melody")
		m = melody.New()
	}
	m.HandleConnect(func(s *melody.Session) {
		ctx, _ := s.Get("ctx") // get gin context
		foundContext, exists := ctx.(*gin.Context).Get("TUMLiveContext")
		if !exists {
			sentry.CaptureException(errors.New("context should exist but doesn't"))
			return
		}
		tumLiveContext := foundContext.(tools.TUMLiveContext)
		if !tumLiveContext.Stream.Recording {
			statsLock.Lock()
			if statMsg, err := json.Marshal(gin.H{"viewers": stats[ctx.(*gin.Context).Param("streamID")]}); err == nil {
				_ = s.Write(statMsg)
			} else {
				sentry.CaptureException(err)
			}
			statsLock.Unlock()
		}
	})

	m.HandleMessage(func(s *melody.Session, msg []byte) {
		ctx, _ := s.Get("ctx") // get gin context
		foundContext, exists := ctx.(*gin.Context).Get("TUMLiveContext")
		if !exists {
			sentry.CaptureException(errors.New("context should exist but doesn't"))
			ctx.(*gin.Context).AbortWithStatus(http.StatusInternalServerError)
			return
		}
		tumLiveContext := foundContext.(tools.TUMLiveContext)
		var chat chatReq
		if err := json.Unmarshal(msg, &chat); err != nil {
			return
		}
		if chat.Msg == "" || len(chat.Msg) > 200 {
			return
		}
		if dao.IsUserCooledDown(fmt.Sprintf("%v", tumLiveContext.User.ID)) {
			return
		}
		if !tumLiveContext.Course.ChatEnabled {
			return
		}
		uname := tumLiveContext.User.Name
		if chat.Anonymous {
			uname = "Anonymous"
		}
		dao.AddMessage(model.Chat{
			UserID:   strconv.Itoa(int(tumLiveContext.User.ID)),
			UserName: uname,
			Message:  chat.Msg,
			StreamID: tumLiveContext.Stream.ID,
			Admin:    tumLiveContext.User.ID == tumLiveContext.Course.UserID,
			SendTime: time.Now().In(tools.Loc),
		})
		if broadcast, err := json.Marshal(ChatRep{
			Msg:   chat.Msg,
			Name:  uname,
			Admin: tumLiveContext.User.ID == tumLiveContext.Course.UserID,
		}); err == nil {
			_ = m.BroadcastFilter(broadcast, func(q *melody.Session) bool { // filter broadcasting to same lecture.
				return q.Request.URL.Path == s.Request.URL.Path
			})
		}
	})
}

type

type chatReq struct {
	Msg       string `json:"msg"`
	Anonymous bool   `json:"anonymous"`
}
type ChatRep struct {
	Msg   string `json:"msg"`
	Name  string `json:"name"`
	Admin bool   `json:"admin"`
}

func CollectStats() {
	log.Printf("Collecting stats\n")
	defer sentry.Flush(time.Second * 2)
	for sID, numWatchers := range stats {
		log.Printf("Collecting stats for stream %v, viewers:%v\n", sID, numWatchers)
		stat := model.Stat{
			Time:    time.Now(),
			Viewers: numWatchers,
		}
		if s, err := dao.GetStreamByID(context.Background(), sID); err == nil {
			if s.Recording { // broadcast stats for livestreams and upcoming videos only
				log.Printf("stream not live, skipping stats\n")
				delete(stats, strconv.Itoa(int(s.ID)))
				continue
			}
			if s.LiveNow { // store stats for livestreams only
				s.Stats = append(s.Stats, stat)
				if err = dao.SaveStream(&s); err != nil {
					sentry.CaptureException(err)
					log.Printf("Error saving stats: %v\n", err)
				}
			}
			if mStat, err := json.Marshal(gin.H{"viewers": numWatchers}); err == nil {
				_ = m.BroadcastFilter(mStat, func(q *melody.Session) bool { // filter broadcasting to same lecture.
					return q.Request.URL.Path == fmt.Sprintf("/api/chat/%v/ws", sID)
				})
			} else {
				sentry.CaptureException(err)
			}
		}
	}
}

func notifyViewersLiveStart(streamId uint)  {
	req, _ := json.Marshal(gin.H{"live": true})
	_ = m.BroadcastFilter(req, func(s *melody.Session) bool {
		return s.Request.URL.Path == fmt.Sprintf("/api/chat/%v/ws", streamId)
	})
}

func notifyViewersLiveEnd(streamId string)  {
	req, _ := json.Marshal(gin.H{"live": false})
	_ = m.BroadcastFilter(req, func(s *melody.Session) bool {
		return s.Request.URL.Path == fmt.Sprintf("/api/chat/%v/ws", streamId)
	})
}

func ChatStream(c *gin.Context) {
	// max participants in chat to prevent attacks
	if m.Len() > 10000 {
		return
	}
	addUser(c.Param("streamID"))
	joinTime := time.Now()
	defer removeUser(c.Param("streamID"), joinTime)
	ctxMap := make(map[string]interface{}, 1)
	ctxMap["ctx"] = c

	_ = m.HandleRequestWithKeys(c.Writer, c.Request, ctxMap)
}

func addUser(id string) {
	statsLock.Lock()
	if _, ok := stats[id]; !ok {
		stats[id] = 0
	}
	stats[id] += 1
	statsLock.Unlock()
}

func removeUser(id string, jointime time.Time) {
	// watched at least 5 minutes of the lecture? Count as view.
	if jointime.Before(time.Now().Add(time.Minute * -5)) {
		dao.AddVodView(id)
	}
	statsLock.Lock()
	stats[id] -= 1
	if stats[id] <= 0 {
		delete(stats, id)
	}
	statsLock.Unlock()
}
