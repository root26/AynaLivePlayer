package sourcelogin

import (
	"AynaLivePlayer/core/events"
	"AynaLivePlayer/global"
	"AynaLivePlayer/gui/component"
	config2 "AynaLivePlayer/gui/views/config"
	"AynaLivePlayer/pkg/config"
	"AynaLivePlayer/pkg/eventbus"
	"AynaLivePlayer/pkg/i18n"
	"AynaLivePlayer/pkg/logger"
	"AynaLivePlayer/resource"
	"bytes"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/AynaLivePlayer/miaosic"
	"github.com/skip2/go-qrcode"
)

const MODULE_PLGUIN_NETEASELOGIN = "plugin.neteaselogin"

type SourceLogin struct {
	SessionPath string `json:"session_path"`
	sessions    map[string]string
	log         logger.ILogger
	panel       fyne.CanvasObject
}

func (w *SourceLogin) OnLoad() {
	_ = config.LoadJson(w.SessionPath, &w.sessions)
}

func (w *SourceLogin) OnSave() {
	_ = config.SaveJson(w.SessionPath, &w.sessions)
}

func NewSourceLogin() *SourceLogin {
	return &SourceLogin{
		SessionPath: "config/source_session.json",
		log:         global.Logger.WithPrefix("plugin.sourcelogin"),
		sessions:    make(map[string]string),
	}
}

func (w *SourceLogin) Name() string {
	return "SourceLogin"
}

func (w *SourceLogin) Enable() error {
	config.LoadConfig(w)
	config2.AddConfigLayout(w)
	return nil
}

func (w *SourceLogin) Disable() error {
	w.log.Info("save session for all provider")
	for _, provider := range w.listLoginableProviders() {
		w.log.Infof("save session for %s", provider)
		session, err := w.saveSession(provider)
		if err != nil {
			w.log.Warnf("save session for %s failed: %v", provider, err)
			continue
		}
		w.sessions[provider] = session
	}
	return nil
}

func (w *SourceLogin) Title() string {
	return i18n.T("plugin.sourcelogin.title")
}

func (w *SourceLogin) Description() string {
	return i18n.T("plugin.sourcelogin.description")
}

// todo: fix using fyne async update ui
func (w *SourceLogin) CreatePanel() fyne.CanvasObject {
	if w.panel != nil {
		return w.panel
	}
	currentUser := widget.NewLabel(i18n.T("plugin.sourcelogin.current_user.notlogin"))
	currentStatus := container.NewHBox(
		widget.NewLabel(i18n.T("plugin.sourcelogin.current_user")),
		currentUser)

	providerChoice := widget.NewSelect([]string{}, func(s string) {
		w.log.Info("switching provider to ", s)
		if s != "" {
			isLogin, err := w.isLogin(s)
			if err != nil {
				_ = global.EventBus.Publish(events.ErrorUpdate,
					events.ErrorUpdateEvent{Error: err})
				return
			}
			if isLogin {
				currentUser.SetText(i18n.T("plugin.sourcelogin.current_user.loggedin"))
			} else {
				currentUser.SetText(i18n.T("plugin.sourcelogin.current_user.notlogin"))
			}
		}
	})

	sourcePanel := container.NewGridWithColumns(2,
		providerChoice, currentStatus)
	restoredSessions := make(map[string]bool)
	_ = global.EventBus.Subscribe("",
		events.MediaProviderUpdate,
		"plugin.sourcelogin.providers",
		func(event *eventbus.Event) {
			data := event.Data.(events.MediaProviderUpdateEvent)
			loginableProviders := make([]string, 0)
			for _, providerInfo := range data.ProviderInfos {
				if providerInfo.Loginable {
					loginableProviders = append(loginableProviders, providerInfo.Name)
				}
			}
			for _, provider := range loginableProviders {
				if restoredSessions[provider] {
					continue
				}
				session, ok := w.sessions[provider]
				if !ok || session == "" {
					continue
				}
				restoredSessions[provider] = true
				go func(providerName string, providerSession string) {
					if err := w.restoreSession(providerName, providerSession); err != nil {
						w.log.Warnf("failed to restore session for %s: %v", providerName, err)
					}
				}(provider, session)
			}
			fyne.DoAndWait(func() {
				providerChoice.Options = loginableProviders
				providerChoice.Refresh()
				if providerChoice.Selected == "" && len(loginableProviders) > 0 {
					providerChoice.SetSelected(loginableProviders[0])
				}
			})
		})

	logoutBtn := component.NewAsyncButton(
		i18n.T("plugin.sourcelogin.logout"),
		func() {
			if providerChoice.Selected == "" {
				return
			}
			err := w.logout(providerChoice.Selected)
			if err != nil {
				_ = global.EventBus.Publish(events.ErrorUpdate,
					events.ErrorUpdateEvent{Error: err})
				return
			}
			fyne.DoAndWait(func() {
				currentUser.SetText(i18n.T("plugin.sourcelogin.current_user.notlogin"))
			})
			w.sessions[providerChoice.Selected] = ""
		},
	)
	qrcodeImg := canvas.NewImageFromResource(resource.ImageEmptyQrCode)
	qrcodeImg.SetMinSize(fyne.NewSize(200, 200))
	qrcodeImg.FillMode = canvas.ImageFillContain
	var currentLoginSession *miaosic.QrLoginSession
	//var key string
	qrStatus := widget.NewLabel("AAAAAAAA")
	qrStatus.SetText("")
	newQrBtn := component.NewAsyncButton(
		i18n.T("plugin.sourcelogin.qr.new"),
		func() {
			var err error
			if providerChoice.Selected == "" {
				return
			}
			fyne.DoAndWait(func() {
				qrStatus.SetText("")
			})
			w.log.Info("getting a new qr code for login")
			resp, err := global.EventBus.Call(
				events.CmdMiaosicQrLogin,
				events.ReplyMiaosicQrLogin,
				events.CmdMiaosicQrLoginData{
					Provider: providerChoice.Selected,
				},
			)
			if err != nil {
				_ = global.EventBus.Publish(events.ErrorUpdate,
					events.ErrorUpdateEvent{Error: err})
				return
			}
			qrData := resp.Data.(events.ReplyMiaosicQrLoginData)
			if qrData.Error != nil {
				_ = global.EventBus.Publish(events.ErrorUpdate,
					events.ErrorUpdateEvent{Error: qrData.Error})
				return
			}
			currentLoginSession = &qrData.Session
			w.log.Debugf("trying encode url %s to qrcode", currentLoginSession.Url)
			data, err := qrcode.Encode(currentLoginSession.Url, qrcode.Medium, 256)
			if err != nil {
				_ = global.EventBus.Publish(events.ErrorUpdate,
					events.ErrorUpdateEvent{Error: err})
				return
			}
			//w.log.Debug("create img from raw data")
			fyne.DoAndWait(func() {
				pic := canvas.NewImageFromReader(bytes.NewReader(data), "qrcode")
				qrcodeImg.Resource = pic.Resource
				qrcodeImg.Refresh()
			})
		},
	)
	finishQrBtn := component.NewAsyncButton(
		i18n.T("plugin.sourcelogin.qr.finish"),
		func() {
			if currentLoginSession == nil {
				return
			}
			currentProvider := providerChoice.Selected
			if currentProvider == "" {
				return
			}
			w.log.Info("checking qr status")
			resp, err := global.EventBus.Call(
				events.CmdMiaosicQrLoginVerify,
				events.ReplyMiaosicQrLoginVerify,
				events.CmdMiaosicQrLoginVerifyData{
					Provider: currentProvider,
					Session:  *currentLoginSession,
				},
			)
			if err != nil {
				_ = global.EventBus.Publish(events.ErrorUpdate,
					events.ErrorUpdateEvent{Error: err})
				return
			}
			resultData := resp.Data.(events.ReplyMiaosicQrLoginVerifyData)
			if resultData.Error != nil {
				_ = global.EventBus.Publish(events.ErrorUpdate,
					events.ErrorUpdateEvent{Error: resultData.Error})
				return
			}
			fyne.DoAndWait(func() {
				qrStatus.SetText(resultData.Result.Message)
			})
			if resultData.Result.Success {
				currentLoginSession = nil
				fyne.DoAndWait(func() {
					qrcodeImg.Resource = resource.ImageEmptyQrCode
					qrcodeImg.Refresh()
					providerChoice.OnChanged(currentProvider)
				})
				session, sessionErr := w.saveSession(currentProvider)
				if sessionErr != nil {
					_ = global.EventBus.Publish(events.ErrorUpdate,
						events.ErrorUpdateEvent{Error: sessionErr})
					return
				}
				w.sessions[currentProvider] = session
			}
		},
	)
	controlBox := container.NewHBox(newQrBtn, finishQrBtn, logoutBtn)
	qrImagePanel := container.NewCenter(
		container.NewVBox(qrcodeImg, qrStatus),
	)
	w.panel = container.NewVBox(sourcePanel, controlBox, qrImagePanel)
	return w.panel
}

func (w *SourceLogin) listLoginableProviders() []string {
	resp, err := global.EventBus.Call(
		events.CmdMiaosicListProviders,
		events.ReplyMiaosicListProviders,
		events.CmdMiaosicListProvidersData{},
	)
	if err != nil {
		w.log.Warnf("list providers failed: %v", err)
		return []string{}
	}
	data := resp.Data.(events.ReplyMiaosicListProvidersData)
	providers := make([]string, 0)
	for _, provider := range data.Providers {
		if provider.Loginable {
			providers = append(providers, provider.Name)
		}
	}
	return providers
}

func (w *SourceLogin) isLogin(provider string) (bool, error) {
	resp, err := global.EventBus.Call(
		events.CmdMiaosicIsLoginByProvider,
		events.ReplyMiaosicIsLoginByProvider,
		events.CmdMiaosicIsLoginByProviderData{
			Provider: provider,
		},
	)
	if err != nil {
		return false, err
	}
	data := resp.Data.(events.ReplyMiaosicIsLoginByProviderData)
	return data.IsLogin, data.Error
}

func (w *SourceLogin) logout(provider string) error {
	resp, err := global.EventBus.Call(
		events.CmdMiaosicLogoutByProvider,
		events.ReplyMiaosicLogoutByProvider,
		events.CmdMiaosicLogoutByProviderData{
			Provider: provider,
		},
	)
	if err != nil {
		return err
	}
	data := resp.Data.(events.ReplyMiaosicLogoutByProviderData)
	return data.Error
}

func (w *SourceLogin) restoreSession(provider, session string) error {
	resp, err := global.EventBus.Call(
		events.CmdMiaosicRestoreSessionByProvider,
		events.ReplyMiaosicRestoreSessionByProvider,
		events.CmdMiaosicRestoreSessionByProviderData{
			Provider: provider,
			Session:  session,
		},
	)
	if err != nil {
		return err
	}
	data := resp.Data.(events.ReplyMiaosicRestoreSessionByProviderData)
	return data.Error
}

func (w *SourceLogin) saveSession(provider string) (string, error) {
	resp, err := global.EventBus.Call(
		events.CmdMiaosicSaveSessionByProvider,
		events.ReplyMiaosicSaveSessionByProvider,
		events.CmdMiaosicSaveSessionByProviderData{
			Provider: provider,
		},
	)
	if err != nil {
		return "", err
	}
	data := resp.Data.(events.ReplyMiaosicSaveSessionByProviderData)
	return data.Session, data.Error
}
