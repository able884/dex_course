package respx

import (
	"net/http"

	"github.com/zeromicro/go-zero/core/logc"
	"github.com/zeromicro/go-zero/rest/httpx"
	"richcode.cc/dex/pkg/xcode"
)

func JsonResp(w http.ResponseWriter, r *http.Request, code int, msg string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	lang := r.Header.Get("X-Language")
	message := xcode.GetMessage(lang, code)
	if message != "" {
		msg = message
	}
	result := &RespJsonData{
		Code: code,
		Msg:  msg,
		Data: data,
	}
	wrapNullData(result)
	httpx.OkJson(w, result)
	if code != xcode.Ok {
		logc.Errorf(r.Context(), "gateway resp err : %#v", result)
	}
}

type RespJsonData struct {
	Code int         `json:"code"`
	Msg  string      `json:"message"`
	Data interface{} `json:"data"`
}

func wrapNullData(rsp *RespJsonData) {
	if rsp == nil {
		return
	}
	if rsp.Data != nil {
		return
	}
	rsp.Data = struct{}{}
}
