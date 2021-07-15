package tools

import (
	"TUM-Live/dao"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/getsentry/sentry-go"
	"log"
	"net/http"
	"time"
)


func NotifyWorkers() {
	log.Println("getting streams for workers")
	streams := dao.GetDueStreamsFromLectureHalls()
	workers := dao.GetAliveWorkersOrderedByWorkload()
	if len(workers) == 0 {
		return
	}
	for i, stream := range streams {
		assignedWorker := workers[i%len(workers)]
		log.Printf("stream %v assigned to %v", stream.Model.ID, assignedWorker.Host)
		lectureHall, err := dao.GetLectureHallByID(stream.LectureHallID)
		course, _ := dao.GetCourseById(context.Background(), stream.CourseID)
		if err != nil {
			sentry.CaptureException(err)
			continue
		}
		sources := make(map[string]string)
		if lectureHall.CamIP != "" {
			sources["CAM"] = lectureHall.CamIP
		}
		if lectureHall.PresIP != "" {
			sources["PRES"] = lectureHall.PresIP
		}
		if lectureHall.CombIP != "" {
			sources["COMB"] = lectureHall.CombIP
		}
		if req, err := json.Marshal(streamLectureHallRequest{
			Sources:      sources,
			StreamEnd:    stream.End,
			StreamName:   fmt.Sprintf("%s%v", course.Slug, stream.Start.Format("2006_01_02_15_04")), // again, wtf go
			ID:           fmt.Sprintf("%v", stream.Model.ID),
			Upload:       course.VODEnabled,
			Semester:     course.Year,
			TeachingTerm: course.TeachingTerm,
			Slug:         course.Slug,
		}); err == nil {
			_, err := http.Post(fmt.Sprintf("https://%s/%s/streamLectureHall", assignedWorker.Host, assignedWorker.WorkerID), "application/json", bytes.NewBuffer(req))
			if err != nil {
				// todo: pick new worker for stream
				sentry.CaptureException(err)
				continue
			}
		} else {
			sentry.CaptureException(err)
		}
	}
}

type streamLectureHallRequest struct {
	Sources      map[string]string `json:"sources"` //CAM->123.4.5.6/extron5
	StreamEnd    time.Time         `json:"streamEnd"`
	StreamName   string            `json:"streamName"`
	ID           string            `json:"id"`
	Upload       bool              `json:"upload"`
	Semester     int               `json:"semester"`
	TeachingTerm string            `json:"teachingTerm"`
	Slug         string            `json:"slug"`
}
