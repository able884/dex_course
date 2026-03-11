package slot

import (
	"errors"
	"time"

	"richcode.cc/dex/model/solmodel"
)

type RecoverFailedBlockService struct {
	*SlotService
}

func NewRecoverFailedBlockService(slotService *SlotService) *RecoverFailedBlockService {
	return &RecoverFailedBlockService{
		SlotService: slotService,
	}
}

func (s *RecoverFailedBlockService) Start() {
	s.RecoverFailedBlock()
}

// RecoverFailedBlock 失败区块恢复
func (s *RecoverFailedBlockService) RecoverFailedBlock() {
	slot := s.sc.Config.Sol.StartBlock

	if slot == 0 {
		block, err := s.sc.BlockModel.FindFirstFailBlock(s.ctx)
		if err != nil {
			s.Errorf("RecoverFailedBlock:FindFirstFailBlock %v", err)
			slot = 0
		} else {
			slot = uint64(block.Slot)
		}
	}

	s.Infof("RecoverFailedBlock: start slot: %v, startBlock: %v", slot, s.sc.Config.Sol.StartBlock)

	var checkTicker = time.NewTicker(time.Millisecond * 5000)
	var sendTicker = time.NewTicker(time.Millisecond * 5000)
	defer checkTicker.Stop()
	defer sendTicker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			s.Info("slotFailed stop succeed")
			return
		case <-checkTicker.C:
			slots, err := s.sc.BlockModel.FindProcessingSlots(s.ctx, int64(slot-100), 50)
			s.Infof("FindProcessingSlots err: %v, size: %v", err, len(slots))
			switch {
			case errors.Is(err, solmodel.ErrNotFound) || len(slots) == 0:
				return
			case err == nil:
			default:
				s.Error("FindProcessingSlot err:", err)
			}
			for _, slot := range slots {
				select {
				case <-s.ctx.Done():
					return
				case <-sendTicker.C:
					s.Infof("RecoverFailedBlock: push slot: %v to err chain, start Block: %v", slot.Slot, s.sc.Config.Sol.StartBlock)

					s.errChan <- uint64(slot.Slot)
				}
			}
		}
	}

}
