// Package api_grpc Handles communication with workers
package api_grpc

import (
	"TUM-Live/dao"
	"container/heap"
	"context"
	"fmt"
	"github.com/getsentry/sentry-go"
	"github.com/joschahenningsen/TUM-Live-Worker-v2/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
	"net"
	"strings"
	"time"
)

type server struct {
	pb.UnimplementedHeartbeatServer
}

//SendHeartBeat serves heartbeat messages sent by workers
func (s server) SendHeartBeat(ctx context.Context, request *pb.HeartBeat) (*pb.Status, error) {
	if worker, err := dao.GetWorkerByID(ctx, request.GetWorkerID()); err != nil {
		log.Printf("Got heartbeat with invalid workerID %v", request.WorkerID)
		return nil, err
	} else {
		worker.Workload = int(request.Workload)
		worker.LastSeen = time.Now()
		worker.Status = strings.Join(request.Jobs, " - ")
		dao.SaveWorker(worker)
		return &pb.Status{Ok: true}, nil
	}
}

// init initializes a gRPC server on port 50052 which is routed to :443/api_grpc
func init() {
	log.Printf("Serving heartbeat")
	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Printf("Failed to init heartbeat server %v", err)
		return
	}
	grpcServer := grpc.NewServer()
	pb.RegisterHeartbeatServer(grpcServer, &server{})
	reflection.Register(grpcServer)
	go func() {
		if err = grpcServer.Serve(lis); err != nil {
			log.Printf("Can't serve heartbeat: %v", err)
		}
	}()
}

// NotifyWorkers collects all streams that are due to stream
// (starts in the next 10 minutes from a lecture hall)
// and invokes the corresponding calls at the workers with the least workload via gRPC
func NotifyWorkers() {
	streams := dao.GetDueStreamsFromLectureHalls()
	workers := dao.GetAliveWorkersOrderedByWorkload()
	if len(workers) == 0 {
		return
	}
	priorityQueue := make(PriorityQueue, len(workers))
	for i, worker := range workers {
		priorityQueue[i] = &Item{Worker: worker, Expiry: worker.Workload}
		priorityQueue[i].Index = i
	}
	heap.Init(&priorityQueue)
	var requests []pb.StreamRequest
	for i := range streams {
		courseForStream, err := dao.GetCourseById(context.Background(), streams[i].CourseID)
		if err != nil {
			log.Printf("Can't get course for stream, skipping: %v", err)
			sentry.CaptureException(err)
			continue
		}
		lectureHallForStream, err := dao.GetLectureHallByID(streams[i].LectureHallID)
		if err != nil {
			log.Printf("Can't get lecture hall for stream, skipping: %v", err)
			sentry.CaptureException(err)
			continue
		}
		sources := []string{lectureHallForStream.CombIP, lectureHallForStream.PresIP, lectureHallForStream.CameraIP}
		for sourceNum, source := range sources {
			if source == "" {
				continue
			}
			var sourceType string
			if sourceNum == 1 {
				sourceType = "PRES"
			} else if sourceNum == 2 {
				sourceType = "CAM"
			} else {
				sourceType = "COMB"
			}
			requests = append(requests, pb.StreamRequest{
				SourceType:    sourceType,
				SourceUrl:     source,
				CourseSlug:    courseForStream.Slug,
				Start:         timestamppb.New(streams[i].Start),
				End:           timestamppb.New(streams[i].End),
				PublishStream: courseForStream.LiveEnabled,
				PublishVoD:    courseForStream.VODEnabled,
			})
		}
	}

	for i := range requests {
		if priorityQueue.Len() == 0 {
			log.Printf("Not enough alive workers to serve stream!")
			continue
		}
		item := heap.Pop(&priorityQueue).(*Item)
		conn, err := grpc.Dial(fmt.Sprintf("%s:50051", item.Worker.Host), grpc.WithInsecure())
		if err != nil {
			log.Printf("Unable to dial server %v", err)
			continue
		}

		client := pb.NewStreamClient(conn)
		resp, err := client.RequestStream(context.Background(), &requests[i])
		if err != nil || !resp.Ok {
			log.Printf("could not assign stream!")
			continue
		}
		_ = conn.Close()
		item.Expiry += 2
		heap.Push(&priorityQueue, *item)
	}
}