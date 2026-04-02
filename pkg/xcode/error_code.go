package xcode

import "strings"

type ErrCode uint32

const (
	Ok = 10000
)

func GetMessage(lang string, code int) string {
	lang = strings.ToLower(lang)
	if lang == "zh" {
		return statusCNMsg[code]
	} else if lang == "en" {
		return statusEnMsg[code]
	} else if lang == "uf" {
		return statusUfMsg[code]
	} else if lang == "ja" {
		return statusJaMsg[code]
	} else if lang == "ko" {
		return statusKoMsg[code]
	}
	return statusEnMsg[code]
}

var statusCNMsg = map[int]string{}
var statusEnMsg = map[int]string{}
var statusUfMsg = map[int]string{}
var statusKoMsg = map[int]string{}
var statusJaMsg = map[int]string{}
