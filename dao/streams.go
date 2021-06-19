package dao

import (
	"TUM-Live/model"
	"context"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

func GetDueStreamsFromLectureHalls() []model.Stream {
	var res []model.Stream
	DB.Model(&model.Stream{}).
		Where("lecture_hall_id IS NOT NULL AND start BETWEEN ? AND ? AND live_now = false AND recording = false", time.Now(), time.Now().Add(time.Minute*10)).
		Scan(&res)
	return res
}

func GetStreamByKey(ctx context.Context, key string) (stream model.Stream, err error) {
	var res model.Stream
	err = DB.First(&res, "stream_key = ?", key).Error
	if err != nil { // entry probably not existent -> not authenticated
		fmt.Printf("error getting stream by key: %v\n", err)
		return res, err
	}
	return res, nil
}

func SetChatMode(streamID string, moderated bool) error {
	Cache.Del(fmt.Sprintf("streambyid%v", streamID))
	err := DB.Model(&model.Stream{}).Where("id = ?", streamID).Update("moderated", moderated).Error
	return err
}

func DeleteUnit(id uint) {
	defer Cache.Clear()
	DB.Delete(&model.StreamUnit{}, id)
}

func GetUnitByID(id string) (model.StreamUnit, error) {
	var unit model.StreamUnit
	err := DB.First(&unit, "id = ?", id).Error
	return unit, err
}

func GetStreamByTumOnlineID(ctx context.Context, id uint) (stream model.Stream, err error) {
	var res model.Stream
	err = DB.Preload("Chats").First(&res, "tum_online_event_id = ?", id).Error
	if err != nil {
		return res, err
	}
	return res, nil
}

func GetStreamByID(ctx context.Context, id string) (stream model.Stream, err error) {
	if cached, found := Cache.Get(fmt.Sprintf("streambyid%v", id)); found {
		return cached.(model.Stream), nil
	}
	var res model.Stream
	err = DB.Preload("Silences").Preload("Chats").Preload("Units", func(db *gorm.DB) *gorm.DB {
		return db.Order("unit_start asc")
	}).First(&res, "id = ?", id).Error
	if err != nil {
		fmt.Printf("error getting stream by id: %v\n", err)
		return res, err
	}
	Cache.SetWithTTL(fmt.Sprintf("streambyid%v", id), res, 1, time.Second*10)
	return res, nil
}

func DeleteStreamsWithTumID(ids []uint) {
	// transaction for performance
	_ = DB.Transaction(func(tx *gorm.DB) error {
		for i := range ids {
			tx.Where("tum_online_event_id = ?", ids[i]).Delete(&model.Stream{})
		}
		return nil
	})
}

func CreateStream(ctx context.Context, stream model.Stream) (err error) {
	dbErr := DB.Create(stream).Error
	return dbErr
}

func AddVodView(id string) {
	_ = DB.Transaction(func(tx *gorm.DB) error {
		var stream model.Stream
		if err := tx.Where("id = ? AND live_now = 0", id).First(&stream).Error; err != nil {
			return err
		}
		if err := tx.Model(&stream).Update("VodViews", stream.VodViews+1).Error; err != nil {
			return err
		}
		return nil
	})
}

func UpdateStream(stream model.Stream) error {
	defer Cache.Clear()
	err := DB.Model(&stream).Updates(map[string]interface{}{
		"name":        stream.Name,
		"description": stream.Description,
		"start":       stream.Start,
		"end":         stream.End}).Error
	return err
}

func SetStreamLive(ctx context.Context, streamKey string, playlistUrl string) (err error) {
	Cache.Clear()
	dbErr := DB.Model(&model.Stream{}).
		Where("stream_key = ?", streamKey).
		Update("live_now", true).
		Update("playlist_url", playlistUrl).
		Error
	return dbErr
}

func GetCurrentLive(ctx context.Context) (currentLive []model.Stream, err error) {
	if streams, found := Cache.Get("AllCurrentlyLiveStreams"); found {
		return streams.([]model.Stream), nil
	}
	var streams []model.Stream
	if err := DB.Find(&streams, "live_now = ?", true).Error; err != nil {
		return nil, err
	}
	Cache.SetWithTTL("AllCurrentlyLiveStreams", streams, 1, time.Minute)
	return streams, err
}

func SetStreamNotLive(ctx context.Context, streamKey string) (err error) {
	Cache.Clear() // costs a bit but hey
	dbErr := DB.Model(&model.Stream{}).
		Where("stream_key = ?", streamKey).
		Update("live_now", false).
		Error
	return dbErr
}

func InsertConvertJob(ctx context.Context, job *model.ProcessingJob) {
	if Logger != nil {
		Logger(ctx, "inserting processing job.")
	}
	DB.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(job)
}

func DeleteStream(streamID string) {
	DB.Where("id = ?", streamID).Delete(&model.Stream{})
	Cache.Clear()
}

func UpdateSilences(silences []model.Silence, streamID string) error {
	DB.Delete(&model.Silence{}, "stream_id = ?", streamID)
	return DB.Save(&silences).Error
}
