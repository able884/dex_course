package slot

import (
	"richcode.cc/dex/consumer/internal/svc"
)

type SlotServiceGroup struct {
	*SlotService
	Ws           *SlotWsService
	NotCompleted *RecoverFailedBlockService
}

func NewSlotServiceGroup(sc *svc.ServiceContext, slotChan chan uint64, errChan chan uint64) *SlotServiceGroup {
	slotService := NewSlotService(sc, slotChan, errChan)
	return &SlotServiceGroup{
		SlotService:  slotService,
		Ws:           NewSlotWsService(slotService),
		NotCompleted: NewRecoverFailedBlockService(slotService),
	}
}

func (s *SlotServiceGroup) Start() {
	s.Ws.Start()
	s.NotCompleted.Start()
}
