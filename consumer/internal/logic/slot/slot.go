package slot

import (
	"context"
	"errors"

	"richcode.cc/dex/consumer/internal/svc"

	"github.com/gorilla/websocket"
	"github.com/zeromicro/go-zero/core/logx"
)

var ErrServiceStop = errors.New("service stop")

type SlotService struct {
	Conn *websocket.Conn
	sc   *svc.ServiceContext
	logx.Logger

	ctx     context.Context
	cancel  func(err error)
	maxSlot uint64

	realTimeChan chan uint64
	errChan      chan uint64
}

func NewSlotService(sc *svc.ServiceContext, slotChan chan uint64, errChan chan uint64) *SlotService {
	ctx, cancel := context.WithCancelCause(context.Background())
	return &SlotService{
		Logger:       logx.WithContext(context.Background()).WithFields(logx.Field("service", "slot")),
		sc:           sc,
		ctx:          ctx,
		cancel:       cancel,
		realTimeChan: slotChan,
		errChan:      errChan,
	}
}

func (s *SlotService) Start() {
}

func (s *SlotService) Stop() {
	s.Info("stop slot")
	s.cancel(ErrServiceStop)
	if s.Conn != nil {
		_ = s.Conn.Close()
	}
}
