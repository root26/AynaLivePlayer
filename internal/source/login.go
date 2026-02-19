package source

import (
	"AynaLivePlayer/core/events"
	"AynaLivePlayer/global"
	"AynaLivePlayer/pkg/eventbus"
	"github.com/AynaLivePlayer/miaosic"
)

func handleSourceLogin() {
	err := global.EventBus.Subscribe("",
		events.CmdMiaosicQrLogin, "internal.media_provider.qrlogin_handler", func(event *eventbus.Event) {
			data := event.Data.(events.CmdMiaosicQrLoginData)
			log.Infof("trying login %s", data.Provider)
			var session miaosic.QrLoginSession
			sess, err := miaosic.QrLoginByProvider(data.Provider)
			if err == nil && sess != nil {
				session = *sess
			}
			_ = global.EventBus.Reply(
				event, events.ReplyMiaosicQrLogin,
				events.ReplyMiaosicQrLoginData{
					Session: session,
					Error:   err,
				})
		})
	if err != nil {
		log.ErrorW("Subscribe miaosic qrlogin failed", "error", err)
	}
	err = global.EventBus.Subscribe("",
		events.CmdMiaosicQrLoginVerify, "internal.media_provider.qrloginverify_handler", func(event *eventbus.Event) {
			data := event.Data.(events.CmdMiaosicQrLoginVerifyData)
			log.Infof("trying login verify %s", data.Provider)
			var result miaosic.QrLoginResult
			res, err := miaosic.QrLoginVerifyByProvider(data.Provider, &data.Session)
			if err == nil && res != nil {
				result = *res
			}
			_ = global.EventBus.Reply(
				event, events.ReplyMiaosicQrLoginVerify,
				events.ReplyMiaosicQrLoginVerifyData{
					Result: result,
					Error:  err,
				})
		})
	if err != nil {
		log.ErrorW("Subscribe miaosic qrloginverify failed", "error", err)
	}

	err = global.EventBus.Subscribe("",
		events.CmdMiaosicLogoutByProvider, "internal.media_provider.logout_by_provider", func(event *eventbus.Event) {
			data := event.Data.(events.CmdMiaosicLogoutByProviderData)
			_ = global.EventBus.Reply(
				event, events.ReplyMiaosicLogoutByProvider,
				events.ReplyMiaosicLogoutByProviderData{
					Error: miaosic.LogoutByProvider(data.Provider),
				})
		})
	if err != nil {
		log.ErrorW("Subscribe miaosic logout failed", "error", err)
	}

	err = global.EventBus.Subscribe("",
		events.CmdMiaosicIsLoginByProvider, "internal.media_provider.is_login_by_provider", func(event *eventbus.Event) {
			data := event.Data.(events.CmdMiaosicIsLoginByProviderData)
			isLogin, loginErr := miaosic.IsLoginByProvider(data.Provider)
			_ = global.EventBus.Reply(
				event, events.ReplyMiaosicIsLoginByProvider,
				events.ReplyMiaosicIsLoginByProviderData{
					IsLogin: isLogin,
					Error:   loginErr,
				})
		})
	if err != nil {
		log.ErrorW("Subscribe miaosic is login failed", "error", err)
	}

	err = global.EventBus.Subscribe("",
		events.CmdMiaosicRestoreSessionByProvider, "internal.media_provider.restore_session_by_provider", func(event *eventbus.Event) {
			data := event.Data.(events.CmdMiaosicRestoreSessionByProviderData)
			_ = global.EventBus.Reply(
				event, events.ReplyMiaosicRestoreSessionByProvider,
				events.ReplyMiaosicRestoreSessionByProviderData{
					Error: miaosic.RestoreSessionByProvider(data.Provider, data.Session),
				})
		})
	if err != nil {
		log.ErrorW("Subscribe miaosic restore session failed", "error", err)
	}

	err = global.EventBus.Subscribe("",
		events.CmdMiaosicSaveSessionByProvider, "internal.media_provider.save_session_by_provider", func(event *eventbus.Event) {
			data := event.Data.(events.CmdMiaosicSaveSessionByProviderData)
			session, sessionErr := miaosic.SaveSessionByProvider(data.Provider)
			_ = global.EventBus.Reply(
				event, events.ReplyMiaosicSaveSessionByProvider,
				events.ReplyMiaosicSaveSessionByProviderData{
					Session: session,
					Error:   sessionErr,
				})
		})
	if err != nil {
		log.ErrorW("Subscribe miaosic save session failed", "error", err)
	}
}
