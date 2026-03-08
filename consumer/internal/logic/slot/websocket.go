package slot

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/zeromicro/go-zero/core/proc"
	"github.com/zeromicro/go-zero/core/threading"

	"github.com/gorilla/websocket"
)

type SlotWsService struct {
	*SlotService
}

func NewSlotWsService(slotService *SlotService) *SlotWsService {
	return &SlotWsService{
		SlotService: slotService,
	}
}

func (s *SlotWsService) Start() {
	proc.AddShutdownListener(func() {
		s.Infof("SlotWsService:ShutdownListener")
		s.cancel(errors.New("close slot"))
	})
	s.SlotWs()
}

func (s *SlotService) SlotWs() {
	s.MustConnect()
	s.Infof("SlotWs:MustConnect success")

	threading.GoSafe(func() {
		// 开始消费增量
		for {
			select {
			case <-s.ctx.Done():
				s.Info("slotWs stop succeed")
				return
			default:
			}
			s.ReadSlotMessage()
		}
	})
}

func (s *SlotService) ReadSlotMessage() {
	defer func() {
		cause := recover()
		if cause != nil {
			s.Error("ReadSlotMessage panic:", cause)
			s.MustConnect()
		}
	}()
	_, message, err := s.Conn.ReadMessage()
	if err != nil {
		s.Error("SlotWs ReadMessage", err)
		if strings.Contains(err.Error(), "close") {
			s.MustConnect()
		}
		if strings.Contains(err.Error(), "broken pipe") {
			s.MustConnect()
		}
		return
	}
	var resp SlotResp
	err = json.Unmarshal(message, &resp)
	if err != nil {
		s.Error("SlotWs son.Unmarshal", err)
		return
	}
	if resp.Params.Result.Slot == 0 {
		return
	}

	s.maxSlot = resp.Params.Result.Slot
	// fmt.Println("latest slot: ", s.maxSlot)

	// 将最新的slot发送到realTimeChan
	s.realTimeChan <- s.maxSlot
}

func (s *SlotService) MustConnect() {
	dialer := websocket.DefaultDialer
	for {
		s.Infof("MustConnect:slot ws url: %v", s.sc.Config.Sol.WSUrl)
		dialer.HandshakeTimeout = time.Second * 5
		c, _, err := dialer.Dial(s.sc.Config.Sol.WSUrl, nil)
		if err != nil {
			s.Errorf("MustConnect:slot ws Dial err: %v", err)
		} else {
			s.Conn = c
			for i := 0; i < 10; i++ {
				err = c.WriteMessage(websocket.TextMessage, []byte("{\"id\":1,\"jsonrpc\":\"2.0\",\"method\": \"slotSubscribe\"}\n"))
				if err != nil {
					s.Error("slot ws slotSubscribe err: %v", err)
				} else {
					return
				}
				time.Sleep(1 * time.Second)
			}
		}
		time.Sleep(1 * time.Second)
	}
}

type SlotResp struct {
	Jsonrpc string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  struct {
		Result struct {
			Slot   uint64 `json:"slot"`
			Parent uint64 `json:"parent"`
			Root   uint64 `json:"root"`
		} `json:"result"`
		Subscription int `json:"subscription"`
	} `json:"params"`
}
