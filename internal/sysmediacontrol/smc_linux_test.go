//go:build linux

package sysmediacontrol

import (
	"AynaLivePlayer/core/events"
	"AynaLivePlayer/global"
	"AynaLivePlayer/pkg/eventbus"
	"errors"
	"sync"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/require"
)

type publishRecord struct {
	id   string
	data interface{}
}

type mockBus struct {
	mu        sync.Mutex
	published []publishRecord
}

func (m *mockBus) Start() error                                       { return nil }
func (m *mockBus) Wait() error                                        { return nil }
func (m *mockBus) Stop() error                                        { return nil }
func (m *mockBus) Subscribe(string, string, string, eventbus.HandlerFunc) error {
	return nil
}
func (m *mockBus) SubscribeAny(string, string, eventbus.HandlerFunc) error { return nil }
func (m *mockBus) SubscribeOnce(string, string, string, eventbus.HandlerFunc) error {
	return nil
}
func (m *mockBus) Unsubscribe(string, string) error { return nil }
func (m *mockBus) Publish(eventID string, data interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = append(m.published, publishRecord{id: eventID, data: data})
	return nil
}
func (m *mockBus) PublishToChannel(string, string, interface{}) error { return nil }
func (m *mockBus) PublishEvent(*eventbus.Event) error                 { return nil }
func (m *mockBus) Call(string, string, interface{}) (*eventbus.Event, error) {
	return nil, errors.New("not implemented")
}
func (m *mockBus) Reply(*eventbus.Event, string, interface{}) error { return nil }

func (m *mockBus) snapshot() []publishRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]publishRecord, len(m.published))
	copy(out, m.published)
	return out
}

func setupSMCTest(t *testing.T) *mockBus {
	t.Helper()
	oldBus := global.EventBus
	oldSMC := linuxSMC
	mb := &mockBus{}
	global.EventBus = mb
	linuxSMC = nil
	t.Cleanup(func() {
		global.EventBus = oldBus
		linuxSMC = oldSMC
	})
	return mb
}

func TestMprisPlayerPublishesControlEvents(t *testing.T) {
	mb := setupSMCTest(t)
	p := &mprisPlayer{}

	require.Nil(t, p.Next())
	require.Nil(t, p.Previous())
	require.Nil(t, p.Pause())
	require.Nil(t, p.Play())
	require.Nil(t, p.PlayPause())
	require.Nil(t, p.Stop())
	require.Nil(t, p.Seek(3_000_000))

	pubs := mb.snapshot()
	require.Len(t, pubs, 7)

	require.Equal(t, events.PlayerPlayNextCmd, pubs[0].id)
	_, ok := pubs[0].data.(events.PlayerPlayNextCmdEvent)
	require.True(t, ok)

	require.Equal(t, events.PlayerSeekCmd, pubs[1].id)
	prevSeek, ok := pubs[1].data.(events.PlayerSeekCmdEvent)
	require.True(t, ok)
	require.True(t, prevSeek.Absolute)
	require.Equal(t, 0.0, prevSeek.Position)

	require.Equal(t, events.PlayerSetPauseCmd, pubs[2].id)
	pauseEvt, ok := pubs[2].data.(events.PlayerSetPauseCmdEvent)
	require.True(t, ok)
	require.True(t, pauseEvt.Pause)

	require.Equal(t, events.PlayerSetPauseCmd, pubs[3].id)
	playEvt, ok := pubs[3].data.(events.PlayerSetPauseCmdEvent)
	require.True(t, ok)
	require.False(t, playEvt.Pause)

	require.Equal(t, events.PlayerToggleCmd, pubs[4].id)
	_, ok = pubs[4].data.(events.PlayerToggleCmdEvent)
	require.True(t, ok)

	require.Equal(t, events.PlayerSetPauseCmd, pubs[5].id)
	stopEvt, ok := pubs[5].data.(events.PlayerSetPauseCmdEvent)
	require.True(t, ok)
	require.True(t, stopEvt.Pause)

	require.Equal(t, events.PlayerSeekCmd, pubs[6].id)
	seekEvt, ok := pubs[6].data.(events.PlayerSeekCmdEvent)
	require.True(t, ok)
	require.False(t, seekEvt.Absolute)
	require.InDelta(t, 3.0, seekEvt.Position, 1e-6)
}

func TestMprisPlayerSetPositionTrackGuard(t *testing.T) {
	mb := setupSMCTest(t)
	p := &mprisPlayer{}

	linuxSMC = &linuxMpris{
		trackPath: dbus.ObjectPath("/org/mpris/MediaPlayer2/track/1"),
	}

	require.Nil(t, p.SetPosition(dbus.ObjectPath("/org/mpris/MediaPlayer2/track/2"), 5_000_000))
	require.Len(t, mb.snapshot(), 0)

	require.Nil(t, p.SetPosition(dbus.ObjectPath("/org/mpris/MediaPlayer2/track/1"), 5_000_000))
	pubs := mb.snapshot()
	require.Len(t, pubs, 1)
	require.Equal(t, events.PlayerSeekCmd, pubs[0].id)
	seekEvt, ok := pubs[0].data.(events.PlayerSeekCmdEvent)
	require.True(t, ok)
	require.True(t, seekEvt.Absolute)
	require.InDelta(t, 5.0, seekEvt.Position, 1e-6)

	linuxSMC.trackPath = noTrackPath
	require.Nil(t, p.SetPosition(dbus.ObjectPath("/org/mpris/MediaPlayer2/track/whatever"), 2_000_000))
	pubs = mb.snapshot()
	require.Len(t, pubs, 2)
	lastEvt, ok := pubs[1].data.(events.PlayerSeekCmdEvent)
	require.True(t, ok)
	require.True(t, lastEvt.Absolute)
	require.InDelta(t, 2.0, lastEvt.Position, 1e-6)
}

