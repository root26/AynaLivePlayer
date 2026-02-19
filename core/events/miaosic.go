package events

import "github.com/AynaLivePlayer/miaosic"

const CmdMiaosicListProviders = "cmd.miaosic.listProviders"

type CmdMiaosicListProvidersData struct{}

const ReplyMiaosicListProviders = "reply.miaosic.listProviders"

type MiaosicProviderInfo struct {
	Name      string `json:"name"`
	Loginable bool   `json:"loginable"`
}

type ReplyMiaosicListProvidersData struct {
	Providers []MiaosicProviderInfo `json:"providers"`
}

const CmdMiaosicMatchMediaByProvider = "cmd.miaosic.matchMediaByProvider"

type CmdMiaosicMatchMediaByProviderData struct {
	Provider string `json:"provider"`
	Keyword  string `json:"keyword"`
}

const ReplyMiaosicMatchMediaByProvider = "reply.miaosic.matchMediaByProvider"

type ReplyMiaosicMatchMediaByProviderData struct {
	Meta  miaosic.MetaData `json:"meta"`
	Found bool             `json:"found"`
}

const CmdMiaosicGetMediaInfo = "cmd.miaosic.getMediaInfo"

type CmdMiaosicGetMediaInfoData struct {
	Meta miaosic.MetaData `json:"meta"`
}

const ReplyMiaosicGetMediaInfo = "reply.miaosic.getMediaInfo"

type ReplyMiaosicGetMediaInfoData struct {
	Info  miaosic.MediaInfo `json:"info"`
	Error error             `json:"error"`
}

const CmdMiaosicGetMediaUrl = "cmd.miaosic.getMediaUrl"

type CmdMiaosicGetMediaUrlData struct {
	Meta    miaosic.MetaData `json:"meta"`
	Quality miaosic.Quality  `json:"quality"`
}

const ReplyMiaosicGetMediaUrl = "reply.miaosic.getMediaUrl"

type ReplyMiaosicGetMediaUrlData struct {
	Urls  []miaosic.MediaUrl `json:"urls"`
	Error error              `json:"error"`
}

const CmdMiaosicQrLogin = "cmd.miaosic.qrLogin"

type CmdMiaosicQrLoginData struct {
	Provider string `json:"provider"`
}

const ReplyMiaosicQrLogin = "reply.miaosic.qrLogin"

type ReplyMiaosicQrLoginData struct {
	Session miaosic.QrLoginSession `json:"session"`
	Error   error                  `json:"error"`
}

const CmdMiaosicQrLoginVerify = "cmd.miaosic.qrLoginVerify"

type CmdMiaosicQrLoginVerifyData struct {
	Provider string                 `json:"provider"`
	Session  miaosic.QrLoginSession `json:"session"`
}

const ReplyMiaosicQrLoginVerify = "reply.miaosic.qrLoginVerify"

type ReplyMiaosicQrLoginVerifyData struct {
	Result miaosic.QrLoginResult `json:"result"`
	Error  error                 `json:"error"`
}

const CmdMiaosicLogoutByProvider = "cmd.miaosic.logoutByProvider"

type CmdMiaosicLogoutByProviderData struct {
	Provider string `json:"provider"`
}

const ReplyMiaosicLogoutByProvider = "reply.miaosic.logoutByProvider"

type ReplyMiaosicLogoutByProviderData struct {
	Error error `json:"error"`
}

const CmdMiaosicIsLoginByProvider = "cmd.miaosic.isLoginByProvider"

type CmdMiaosicIsLoginByProviderData struct {
	Provider string `json:"provider"`
}

const ReplyMiaosicIsLoginByProvider = "reply.miaosic.isLoginByProvider"

type ReplyMiaosicIsLoginByProviderData struct {
	IsLogin bool  `json:"is_login"`
	Error   error `json:"error"`
}

const CmdMiaosicRestoreSessionByProvider = "cmd.miaosic.restoreSessionByProvider"

type CmdMiaosicRestoreSessionByProviderData struct {
	Provider string `json:"provider"`
	Session  string `json:"session"`
}

const ReplyMiaosicRestoreSessionByProvider = "reply.miaosic.restoreSessionByProvider"

type ReplyMiaosicRestoreSessionByProviderData struct {
	Error error `json:"error"`
}

const CmdMiaosicSaveSessionByProvider = "cmd.miaosic.saveSessionByProvider"

type CmdMiaosicSaveSessionByProviderData struct {
	Provider string `json:"provider"`
}

const ReplyMiaosicSaveSessionByProvider = "reply.miaosic.saveSessionByProvider"

type ReplyMiaosicSaveSessionByProviderData struct {
	Session string `json:"session"`
	Error   error  `json:"error"`
}
