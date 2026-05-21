package worker

import (
	"log"

	"github.com/hibiken/asynq"
	"github.com/slash-init/goflow/pkg/queue"
)

func NewWorkerServer(redisAddr string) *asynq.Server {
	return asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			// Named priority queues - higher number = higher priority
			Queues: map[string]int{
				"critical": 6,
				"default":  3,
				"low":      1,
			},
			Concurrency: 10, //10 tasks processed simultaneously
		},
	)
}

func RegisterHandlers(srv *asynq.Server) {
	mux := asynq.NewServeMux() //like Fiber's router but for tasks

	mux.HandleFunc(queue.TypeSendEmail, HandleSendEmail)
	mux.HandleFunc(queue.TypeGenerateReport, HandleGenerateReport)
	mux.HandleFunc("task:fail", HandleAlwaysFail)

	if err := srv.Run(mux); err != nil {
		log.Fatal("Worker server failed: ", err)
	}
}
