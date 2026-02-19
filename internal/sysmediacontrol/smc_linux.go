package sysmediacontrol

import (
	"AynaLivePlayer/core/events"
	"AynaLivePlayer/core/model"
	"AynaLivePlayer/global"
	"AynaLivePlayer/pkg/config"
	"AynaLivePlayer/pkg/eventbus"
	"AynaLivePlayer/pkg/logger"
	"fmt"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
	"sync"
	"time"
)

const (
	mprisBusName  = "org.mpris.MediaPlayer2.AynaLivePlayer"
	mprisObjPath  = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	mprisRootIF   = "org.mpris.MediaPlayer2"
	mprisPlayerIF = "org.mpris.MediaPlayer2.Player"
	noTrackPath   = dbus.ObjectPath("/org/mpris/MediaPlayer2/TrackList/NoTrack")
)

var (
	linuxSMCLog logger.ILogger
	linuxSMC    *linuxMpris
)

type linuxMpris struct {
	conn  *dbus.Conn
	props *prop.Properties
	mu    sync.Mutex

	trackSeq   uint64
	trackPath  dbus.ObjectPath
	metadata   map[string]dbus.Variant
	positionUS int64
	durationUS int64
	volume     float64
	playback   string
}

type mprisRoot struct{}
type mprisPlayer struct{}

func (m *linuxMpris) nextTrackPath() dbus.ObjectPath {
	m.trackSeq++
	return dbus.ObjectPath(fmt.Sprintf("/org/mpris/MediaPlayer2/track/%d", m.trackSeq))
}

func (m *linuxMpris) emitProps(changed map[string]dbus.Variant) {
	_ = m.conn.Emit(
		mprisObjPath,
		"org.freedesktop.DBus.Properties.PropertiesChanged",
		mprisPlayerIF,
		changed,
		[]string{},
	)
}

func toMprisVolume(v float64) float64 {
	if v < 0 {
		v = 0
	}
	return v / 100.0
}

func fromMprisVolume(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v * 100.0
}

func (m *linuxMpris) setPlayback(status string) {
	m.mu.Lock()
	m.playback = status
	props := m.props
	m.mu.Unlock()
	if props != nil {
		props.SetMust(mprisPlayerIF, "PlaybackStatus", status)
	}
	m.emitProps(map[string]dbus.Variant{
		"PlaybackStatus": dbus.MakeVariant(status),
	})
}

func (m *linuxMpris) setPosition(seconds float64) {
	pos := int64(seconds * float64(time.Second/time.Microsecond))
	if pos < 0 {
		pos = 0
	}
	m.mu.Lock()
	m.positionUS = pos
	props := m.props
	m.mu.Unlock()
	// Keep Position up-to-date for Get(Position), but avoid emitting change signal.
	// KDE/GNOME estimate progress locally while playing; frequent signal pushes cause jitter.
	if props != nil {
		props.SetMust(mprisPlayerIF, "Position", pos)
	}
}

func (m *linuxMpris) setDuration(seconds float64) {
	duration := int64(seconds * float64(time.Second/time.Microsecond))
	if duration < 0 {
		duration = 0
	}
	m.mu.Lock()
	m.durationUS = duration
	m.metadata["mpris:length"] = dbus.MakeVariant(duration)
	md := m.metadata
	props := m.props
	m.mu.Unlock()
	if props != nil {
		props.SetMust(mprisPlayerIF, "Metadata", md)
	}
	m.emitProps(map[string]dbus.Variant{
		"Metadata": dbus.MakeVariant(md),
	})
}

func (m *linuxMpris) setVolume(v float64) {
	mv := toMprisVolume(v)
	m.mu.Lock()
	m.volume = mv
	props := m.props
	m.mu.Unlock()
	if props != nil {
		props.SetMust(mprisPlayerIF, "Volume", mv)
	}
	m.emitProps(map[string]dbus.Variant{
		"Volume": dbus.MakeVariant(mv),
	})
}

func (m *linuxMpris) setPlaying(data events.PlayerPlayingUpdateEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if data.Removed {
		m.trackPath = noTrackPath
		m.metadata = map[string]dbus.Variant{
			"mpris:trackid": dbus.MakeVariant(noTrackPath),
		}
		m.positionUS = 0
		m.playback = "Stopped"
		props := m.props
		if props != nil {
			props.SetMust(mprisPlayerIF, "PlaybackStatus", m.playback)
			props.SetMust(mprisPlayerIF, "Metadata", m.metadata)
			props.SetMust(mprisPlayerIF, "Position", m.positionUS)
		}
		m.emitProps(map[string]dbus.Variant{
			"PlaybackStatus": dbus.MakeVariant(m.playback),
			"Metadata":       dbus.MakeVariant(m.metadata),
		})
		return
	}

	m.trackPath = m.nextTrackPath()
	metadata := map[string]dbus.Variant{
		"mpris:trackid": dbus.MakeVariant(m.trackPath),
		"xesam:title":   dbus.MakeVariant(data.Media.Info.Title),
		"xesam:album":   dbus.MakeVariant(data.Media.Info.Album),
	}
	if data.Media.Info.Artist != "" {
		metadata["xesam:artist"] = dbus.MakeVariant([]string{data.Media.Info.Artist})
	}
	if data.Media.Info.Cover.Url != "" {
		metadata["mpris:artUrl"] = dbus.MakeVariant(data.Media.Info.Cover.Url)
	}
	if m.durationUS > 0 {
		metadata["mpris:length"] = dbus.MakeVariant(m.durationUS)
	}

	m.metadata = metadata
	m.positionUS = 0
	m.playback = "Playing"
	props := m.props
	if props != nil {
		props.SetMust(mprisPlayerIF, "PlaybackStatus", m.playback)
		props.SetMust(mprisPlayerIF, "Metadata", m.metadata)
		props.SetMust(mprisPlayerIF, "Position", m.positionUS)
	}
	m.emitProps(map[string]dbus.Variant{
		"PlaybackStatus": dbus.MakeVariant(m.playback),
		"Metadata":       dbus.MakeVariant(m.metadata),
	})
}

func (m *mprisRoot) Raise() *dbus.Error { return nil }
func (m *mprisRoot) Quit() *dbus.Error  { return nil }

func (m *mprisPlayer) Next() *dbus.Error {
	_ = global.EventBus.Publish(events.PlayerPlayNextCmd, events.PlayerPlayNextCmdEvent{})
	return nil
}

func (m *mprisPlayer) Previous() *dbus.Error {
	_ = global.EventBus.Publish(events.PlayerSeekCmd, events.PlayerSeekCmdEvent{
		Position: 0,
		Absolute: true,
	})
	return nil
}

func (m *mprisPlayer) Pause() *dbus.Error {
	_ = global.EventBus.Publish(events.PlayerSetPauseCmd, events.PlayerSetPauseCmdEvent{Pause: true})
	return nil
}

func (m *mprisPlayer) PlayPause() *dbus.Error {
	_ = global.EventBus.Publish(events.PlayerToggleCmd, events.PlayerToggleCmdEvent{})
	return nil
}

func (m *mprisPlayer) Stop() *dbus.Error {
	_ = global.EventBus.Publish(events.PlayerSetPauseCmd, events.PlayerSetPauseCmdEvent{Pause: true})
	return nil
}

func (m *mprisPlayer) Play() *dbus.Error {
	_ = global.EventBus.Publish(events.PlayerSetPauseCmd, events.PlayerSetPauseCmdEvent{Pause: false})
	return nil
}

func (m *mprisPlayer) Seek(offset int64) *dbus.Error {
	_ = global.EventBus.Publish(events.PlayerSeekCmd, events.PlayerSeekCmdEvent{
		Position: float64(offset) / float64(time.Second/time.Microsecond),
		Absolute: false,
	})
	return nil
}

func (m *mprisPlayer) SetPosition(trackID dbus.ObjectPath, position int64) *dbus.Error {
	if linuxSMC == nil {
		return nil
	}
	linuxSMC.mu.Lock()
	currentTrack := linuxSMC.trackPath
	linuxSMC.mu.Unlock()
	if currentTrack != noTrackPath && trackID != currentTrack {
		return nil
	}
	_ = global.EventBus.Publish(events.PlayerSeekCmd, events.PlayerSeekCmdEvent{
		Position: float64(position) / float64(time.Second/time.Microsecond),
		Absolute: true,
	})
	return nil
}

func (m *mprisPlayer) OpenUri(_ string) *dbus.Error { return nil }

func InitSystemMediaControl() {
	linuxSMCLog = global.Logger.WithPrefix("SMTC-Linux")
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		linuxSMCLog.Warnf("failed to connect session bus: %v", err)
		return
	}

	reply, err := conn.RequestName(mprisBusName, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		linuxSMCLog.Warnf("failed to own mpris bus name (%v, %v)", reply, err)
		_ = conn.Close()
		return
	}

	linuxSMC = &linuxMpris{
		conn:      conn,
		trackPath: noTrackPath,
		metadata: map[string]dbus.Variant{
			"mpris:trackid": dbus.MakeVariant(noTrackPath),
		},
		positionUS: 0,
		durationUS: 0,
		volume:     0.5,
		playback:   "Stopped",
	}

	propsSpec := map[string]map[string]*prop.Prop{
		mprisRootIF: {
			"CanQuit":             {Value: false, Writable: false, Emit: prop.EmitTrue},
			"CanRaise":            {Value: false, Writable: false, Emit: prop.EmitTrue},
			"HasTrackList":        {Value: false, Writable: false, Emit: prop.EmitTrue},
			"Identity":            {Value: config.ProgramName, Writable: false, Emit: prop.EmitTrue},
			"DesktopEntry":        {Value: "AynaLivePlayer", Writable: false, Emit: prop.EmitTrue},
			"SupportedUriSchemes": {Value: []string{"file", "http", "https"}, Writable: false, Emit: prop.EmitTrue},
			"SupportedMimeTypes":  {Value: []string{}, Writable: false, Emit: prop.EmitTrue},
			"Fullscreen":          {Value: false, Writable: false, Emit: prop.EmitTrue},
			"CanSetFullscreen":    {Value: false, Writable: false, Emit: prop.EmitTrue},
		},
		mprisPlayerIF: {
			"PlaybackStatus": {Value: linuxSMC.playback, Writable: false, Emit: prop.EmitFalse},
			"Metadata":       {Value: linuxSMC.metadata, Writable: false, Emit: prop.EmitFalse},
			"Volume": {
				Value:    linuxSMC.volume,
				Writable: true,
				Emit:     prop.EmitFalse,
				Callback: func(c *prop.Change) *dbus.Error {
					v, ok := c.Value.(float64)
					if !ok {
						return dbus.MakeFailedError(fmt.Errorf("invalid volume type %T", c.Value))
					}
					linuxSMC.setVolume(fromMprisVolume(v))
					_ = global.EventBus.Publish(events.PlayerVolumeChangeCmd, events.PlayerVolumeChangeCmdEvent{
						Volume: fromMprisVolume(v),
					})
					return nil
				},
			},
			"Position":      {Value: linuxSMC.positionUS, Writable: false, Emit: prop.EmitFalse},
			"CanGoNext":     {Value: true, Writable: false, Emit: prop.EmitTrue},
			"CanGoPrevious": {Value: true, Writable: false, Emit: prop.EmitTrue},
			"CanPlay":       {Value: true, Writable: false, Emit: prop.EmitTrue},
			"CanPause":      {Value: true, Writable: false, Emit: prop.EmitTrue},
			"CanSeek":       {Value: true, Writable: false, Emit: prop.EmitTrue},
			"CanControl":    {Value: true, Writable: false, Emit: prop.EmitTrue},
		},
	}

	linuxSMC.props = prop.New(conn, mprisObjPath, propsSpec)
	_ = conn.Export(&mprisRoot{}, mprisObjPath, mprisRootIF)
	_ = conn.Export(&mprisPlayer{}, mprisObjPath, mprisPlayerIF)

	node := &introspect.Node{
		Name: string(mprisObjPath),
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{
				Name: mprisRootIF,
				Methods: []introspect.Method{
					{Name: "Raise"},
					{Name: "Quit"},
				},
			},
			{
				Name: mprisPlayerIF,
				Methods: []introspect.Method{
					{Name: "Next"},
					{Name: "Previous"},
					{Name: "Pause"},
					{Name: "PlayPause"},
					{Name: "Stop"},
					{Name: "Play"},
					{
						Name: "Seek",
						Args: []introspect.Arg{{Name: "Offset", Type: "x", Direction: "in"}},
					},
					{
						Name: "SetPosition",
						Args: []introspect.Arg{
							{Name: "TrackId", Type: "o", Direction: "in"},
							{Name: "Position", Type: "x", Direction: "in"},
						},
					},
					{
						Name: "OpenUri",
						Args: []introspect.Arg{{Name: "Uri", Type: "s", Direction: "in"}},
					},
				},
				Signals: []introspect.Signal{
					{
						Name: "Seeked",
						Args: []introspect.Arg{{Name: "Position", Type: "x"}},
					},
				},
			},
		},
	}
	_ = conn.Export(introspect.NewIntrospectable(node), mprisObjPath, "org.freedesktop.DBus.Introspectable")

	_ = global.EventBus.Subscribe("", events.PlayerPlayingUpdate, "sysmediacontrol.linux.playing", func(event *eventbus.Event) {
		linuxSMC.setPlaying(event.Data.(events.PlayerPlayingUpdateEvent))
	})
	_ = global.EventBus.Subscribe("", events.PlayerPropertyPauseUpdate, "sysmediacontrol.linux.pause", func(event *eventbus.Event) {
		if event.Data.(events.PlayerPropertyPauseUpdateEvent).Paused {
			linuxSMC.setPlayback("Paused")
		} else {
			linuxSMC.setPlayback("Playing")
		}
	})
	_ = global.EventBus.Subscribe("", events.PlayerPropertyDurationUpdate, "sysmediacontrol.linux.duration", func(event *eventbus.Event) {
		linuxSMC.setDuration(event.Data.(events.PlayerPropertyDurationUpdateEvent).Duration)
	})
	_ = global.EventBus.Subscribe("", events.PlayerPropertyTimePosUpdate, "sysmediacontrol.linux.timepos", func(event *eventbus.Event) {
		linuxSMC.setPosition(event.Data.(events.PlayerPropertyTimePosUpdateEvent).TimePos)
	})
	_ = global.EventBus.Subscribe("", events.PlayerPropertyVolumeUpdate, "sysmediacontrol.linux.volume", func(event *eventbus.Event) {
		linuxSMC.setVolume(event.Data.(events.PlayerPropertyVolumeUpdateEvent).Volume)
	})
	_ = global.EventBus.Subscribe("", events.PlayerPropertyStateUpdate, "sysmediacontrol.linux.state", func(event *eventbus.Event) {
		state := event.Data.(events.PlayerPropertyStateUpdateEvent).State
		if state == model.PlayerStateIdle {
			linuxSMC.setPlayback("Stopped")
		}
	})

	linuxSMCLog.Info("linux MPRIS media control initialized")
}

func Destroy() {
	if linuxSMC == nil {
		return
	}
	_ = global.EventBus.Unsubscribe(events.PlayerPlayingUpdate, "sysmediacontrol.linux.playing")
	_ = global.EventBus.Unsubscribe(events.PlayerPropertyPauseUpdate, "sysmediacontrol.linux.pause")
	_ = global.EventBus.Unsubscribe(events.PlayerPropertyDurationUpdate, "sysmediacontrol.linux.duration")
	_ = global.EventBus.Unsubscribe(events.PlayerPropertyTimePosUpdate, "sysmediacontrol.linux.timepos")
	_ = global.EventBus.Unsubscribe(events.PlayerPropertyVolumeUpdate, "sysmediacontrol.linux.volume")
	_ = global.EventBus.Unsubscribe(events.PlayerPropertyStateUpdate, "sysmediacontrol.linux.state")

	_, _ = linuxSMC.conn.ReleaseName(mprisBusName)
	_ = linuxSMC.conn.Close()
	linuxSMC = nil
}
