package source

import (
	"AynaLivePlayer/core/events"
	"AynaLivePlayer/global"
	"AynaLivePlayer/pkg/eventbus"
	"github.com/AynaLivePlayer/miaosic"
)

func listProviderInfos() []events.MiaosicProviderInfo {
	providers := make([]events.MiaosicProviderInfo, 0)
	for _, providerName := range miaosic.ListAvailableProviders() {
		p, ok := miaosic.GetProvider(providerName)
		if !ok {
			continue
		}
		_, loginable := p.(miaosic.Loginable)
		providers = append(providers, events.MiaosicProviderInfo{
			Name:      providerName,
			Loginable: loginable,
		})
	}
	return providers
}

func handleProvider() {
	err := global.EventBus.Subscribe("",
		events.CmdMiaosicListProviders, "internal.media_provider.list_providers", func(event *eventbus.Event) {
			providers := listProviderInfos()
			_ = global.EventBus.Reply(
				event, events.ReplyMiaosicListProviders,
				events.ReplyMiaosicListProvidersData{
					Providers: providers,
				})
		})
	if err != nil {
		log.ErrorW("Subscribe list providers event failed", "error", err)
	}

	err = global.EventBus.Subscribe("",
		events.CmdMiaosicMatchMediaByProvider, "internal.media_provider.match_media_by_provider", func(event *eventbus.Event) {
			data := event.Data.(events.CmdMiaosicMatchMediaByProviderData)
			meta, found := miaosic.MatchMediaByProvider(data.Provider, data.Keyword)
			_ = global.EventBus.Reply(
				event, events.ReplyMiaosicMatchMediaByProvider,
				events.ReplyMiaosicMatchMediaByProviderData{
					Meta:  meta,
					Found: found,
				})
		})
	if err != nil {
		log.ErrorW("Subscribe match media by provider event failed", "error", err)
	}
}
