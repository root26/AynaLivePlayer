package vlc

import (
	"AynaLivePlayer/core/events"
	"AynaLivePlayer/core/model"
	"AynaLivePlayer/global"
	"AynaLivePlayer/pkg/config"
	"AynaLivePlayer/pkg/eventbus"
	"AynaLivePlayer/pkg/logger"
	"errors"
	"fmt"
	"github.com/AynaLivePlayer/miaosic"
	"github.com/adrg/libvlc-go/v3"
	"math"
	"runtime"
	"strings"
	"sync"
)

var running bool = false
var log logger.ILogger = nil
var player *vlc.Player
var eventManager *vlc.EventManager
var lock sync.Mutex

// 状态变量
var prevPercentPos float64 = 0
var prevTimePos float64 = 0
var duration float64 = 0
var currentState = model.PlayerStateIdle
var currentMedia model.Media
var currentWindowHandle uintptr

var audioDevices []model.AudioDevice
var currentAudioDevice string

func setWindowHandle(handle uintptr) error {
	if player == nil {
		return errors.New("player is not initialized")
	}
	if handle == 0 {
		return errors.New("invalid window handle 0")
	}

	os := runtime.GOOS
	switch os {
	case "windows":
		// Windows 平台使用 DirectX
		if err := player.SetHWND(handle); err != nil {
			return err
		}
	case "darwin":
		// macOS 平台使用 NSView
		if err := player.SetNSObject(handle); err != nil {
			return err
		}
	case "linux":
		// Linux 平台使用 XWindow
		if err := player.SetXWindow(uint32(handle)); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported platform: %s", os)
	}

	currentWindowHandle = handle
	return nil
}

func SetupPlayer() {
	running = true
	config.LoadConfig(cfg)
	log = global.Logger.WithPrefix("VLC Player")

	opts := []string{"--quiet"}
	if !cfg.DisplayMusicCover {
		opts = append(opts, "--no-video")
	}

	// 初始化libvlc
	if err := vlc.Init(opts...); err != nil {
		log.Error("initialize libvlc failed: ", err)
		return
	}

	// 创建播放器
	var err error
	player, err = vlc.NewPlayer()
	if err != nil {
		log.Error("create player failed: ", err)
		return
	}

	// 获取事件管理器
	eventManager, err = player.EventManager()
	if err != nil {
		log.Error("get event manager failed: ", err)
		return
	}

	// 注册事件
	registerEvents()
	registerCmdHandler()
	updateAudioDeviceList()
	restoreConfig()
	log.Info("VLC player initialized")
}

func StopPlayer() {
	log.Info("stopping VLC player")
	if currentAudioDevice != "" {
		cfg.AudioDevice = currentAudioDevice
		log.Infof("save audio device config: %s", cfg.AudioDevice)
	}
	running = false
	currentState = model.PlayerStateIdle
	if player != nil {
		err := player.Stop()
		if err != nil {
			log.Error("stop player failed: ", err)
		}
		err = player.Release()
		if err != nil {
			log.Error("release player failed: ", err)
		}
	}
	err := vlc.Release()
	if err != nil {
		log.Error("release player failed: ", err)
	}
	log.Info("VLC player stopped")
}

func registerEvents() {
	// 播放结束事件
	_, err := eventManager.Attach(vlc.MediaPlayerEndReached, func(e vlc.Event, userData interface{}) {
		currentState = model.PlayerStateIdle
		_ = global.EventBus.Publish(events.PlayerPropertyStateUpdate, events.PlayerPropertyStateUpdateEvent{State: currentState})
		_ = global.EventBus.Publish(events.PlayerPlayingUpdate, events.PlayerPlayingUpdateEvent{
			Media:   model.Media{},
			Removed: true,
		})
	}, nil)
	if err != nil {
		log.Error("register MediaPlayerEndReached event failed: ", err)
	}

	// 播放位置改变事件
	_, err = eventManager.Attach(vlc.MediaPlayerPositionChanged, func(e vlc.Event, userData interface{}) {
		pos32, _ := player.MediaPosition()
		pos := float64(pos32)
		if duration > 0 {
			timePos := pos * duration
			percentPos := pos * 100
			// 忽略小变化
			if math.Abs(timePos-prevTimePos) < 0.5 && math.Abs(percentPos-prevPercentPos) < 0.5 {
				return
			}
			prevTimePos = timePos
			prevPercentPos = percentPos
			_ = global.EventBus.Publish(events.PlayerPropertyTimePosUpdate, events.PlayerPropertyTimePosUpdateEvent{
				TimePos: timePos,
			})
			_ = global.EventBus.Publish(events.PlayerPropertyPercentPosUpdate, events.PlayerPropertyPercentPosUpdateEvent{
				PercentPos: percentPos,
			})
		}
	}, nil)
	if err != nil {
		log.Error("register MediaPlayerPositionChanged event failed: ", err)
	}

	// 时间改变事件（获取时长）
	_, err = eventManager.Attach(vlc.MediaPlayerTimeChanged, func(e vlc.Event, userData interface{}) {
		dur, _ := player.MediaLength()
		duration = float64(dur) / 1000.0 // 转换为秒
		_ = global.EventBus.Publish(events.PlayerPropertyDurationUpdate, events.PlayerPropertyDurationUpdateEvent{
			Duration: duration,
		})
	}, nil)
	if err != nil {
		log.Error("register MediaPlayerTimeChanged event failed: ", err)
	}

	// 暂停状态改变
	_, err = eventManager.Attach(vlc.MediaPlayerPaused, func(e vlc.Event, userData interface{}) {
		log.Debug("VLC player paused")
		_ = global.EventBus.Publish(events.PlayerPropertyPauseUpdate, events.PlayerPropertyPauseUpdateEvent{
			Paused: true,
		})
	}, nil)
	if err != nil {
		log.Error("register MediaPlayerPaused event failed: ", err)
	}

	_, err = eventManager.Attach(vlc.MediaPlayerPlaying, func(e vlc.Event, userData interface{}) {
		log.Debug("VLC player playing")
		currentState = currentState.NextState(model.PlayerStatePlaying)
		_ = global.EventBus.Publish(events.PlayerPropertyStateUpdate, events.PlayerPropertyStateUpdateEvent{
			State: currentState,
		})
		_ = global.EventBus.Publish(events.PlayerPropertyPauseUpdate, events.PlayerPropertyPauseUpdateEvent{
			Paused: false,
		})
	}, nil)
	if err != nil {
		log.Error("register MediaPlayerPlaying event failed: ", err)
	}

	_, err = eventManager.Attach(vlc.MediaPlayerOpening, func(e vlc.Event, userData interface{}) {
		currentState = currentState.NextState(model.PlayerStateLoading)
		_ = global.EventBus.Publish(events.PlayerPropertyStateUpdate, events.PlayerPropertyStateUpdateEvent{
			State: currentState,
		})
	}, nil)
	if err != nil {
		log.Error("register MediaPlayerOpening event failed: ", err)
	}

	_, err = eventManager.Attach(vlc.MediaPlayerStopped, func(e vlc.Event, userData interface{}) {
		currentState = model.PlayerStateIdle
		_ = global.EventBus.Publish(events.PlayerPropertyStateUpdate, events.PlayerPropertyStateUpdateEvent{
			State: currentState,
		})
	}, nil)
	if err != nil {
		log.Error("register MediaPlayerStopped event failed: ", err)
	}

	_, err = eventManager.Attach(vlc.MediaPlayerEncounteredError, func(e vlc.Event, userData interface{}) {
		currentState = model.PlayerStateIdle
		_ = global.EventBus.Publish(events.PlayerPropertyStateUpdate, events.PlayerPropertyStateUpdateEvent{
			State: currentState,
		})
		_ = global.EventBus.Publish(events.PlayerPlayErrorUpdate, events.PlayerPlayErrorUpdateEvent{
			Error: errors.New("vlc encountered playback error"),
		})
	}, nil)
	if err != nil {
		log.Error("register MediaPlayerEncounteredError event failed: ", err)
	}

	_, err = eventManager.Attach(vlc.MediaPlayerAudioVolume, func(e vlc.Event, userData interface{}) {
		volume, _ := player.Volume()
		log.Debug("VLC player audio volume: ", volume)
		_ = global.EventBus.Publish(events.PlayerPropertyVolumeUpdate, events.PlayerPropertyVolumeUpdateEvent{
			Volume: float64(volume),
		})
	}, nil)
}

func registerCmdHandler() {
	global.EventBus.Subscribe("", events.PlayerPlayCmd, "player.play", func(evnt *eventbus.Event) {
		mediaInfo := evnt.Data.(events.PlayerPlayCmdEvent).Media.Info
		mediaData := evnt.Data.(events.PlayerPlayCmdEvent).Media
		currentState = currentState.NextState(model.PlayerStateLoading)
		_ = global.EventBus.Publish(events.PlayerPropertyStateUpdate, events.PlayerPropertyStateUpdateEvent{
			State: currentState,
		})

		log.Infof("[VLC Player] Play media %s", mediaInfo.Title)

		respInfo, err := global.EventBus.Call(events.CmdMiaosicGetMediaInfo, events.ReplyMiaosicGetMediaInfo,
			events.CmdMiaosicGetMediaInfoData{Meta: mediaData.Info.Meta})
		if err == nil {
			infoReply := respInfo.Data.(events.ReplyMiaosicGetMediaInfoData)
			if infoReply.Error == nil {
				mediaData.Info = infoReply.Info
			}
		}

		_ = global.EventBus.Publish(events.PlayerPlayingUpdate, events.PlayerPlayingUpdateEvent{
			Media:   mediaData,
			Removed: false,
		})

		respURL, err := global.EventBus.Call(events.CmdMiaosicGetMediaUrl, events.ReplyMiaosicGetMediaUrl,
			events.CmdMiaosicGetMediaUrlData{Meta: mediaData.Info.Meta, Quality: miaosic.QualityAny})
		if err != nil {
			log.Warn("[VLC PlayControl] get media url failed ", mediaInfo.Meta.ID(), err)
			_ = global.EventBus.Publish(
				events.PlayerPlayErrorUpdate,
				events.PlayerPlayErrorUpdateEvent{
					Error: err,
				})
			return
		}
		mediaUrls := respURL.Data.(events.ReplyMiaosicGetMediaUrlData)
		if mediaUrls.Error != nil || len(mediaUrls.Urls) == 0 {
			replyErr := mediaUrls.Error
			if replyErr == nil {
				replyErr = errors.New("empty media url list")
			}
			log.Warn("[VLC PlayControl] get media url failed ", mediaInfo.Meta.ID(), replyErr)
			_ = global.EventBus.Publish(
				events.PlayerPlayErrorUpdate,
				events.PlayerPlayErrorUpdateEvent{
					Error: replyErr,
				})
			return
		}

		lock.Lock()
		defer lock.Unlock()

		// 创建媒体对象
		var media *vlc.Media
		mediaURL := mediaUrls.Urls[0]
		log.Debugf("[VLC PlayControl] get player media %s", mediaURL.Url)
		if strings.HasPrefix(mediaURL.Url, "http") {
			media, err = vlc.NewMediaFromURL(mediaURL.Url)
		} else {
			media, err = vlc.NewMediaFromPath(mediaURL.Url)
		}
		if err != nil {
			log.Error("create media failed: ", err)
			_ = global.EventBus.Publish(events.PlayerPlayErrorUpdate, events.PlayerPlayErrorUpdateEvent{Error: err})
			return
		}
		defer func() {
			if err := media.Release(); err != nil {
				log.Warn("release media failed: ", err)
			}
		}()

		// 设置HTTP头
		if val, ok := mediaURL.Header["User-Agent"]; ok {
			err = media.AddOptions(":http-user-agent=" + val)
			if err != nil {
				log.Warn("add http-user-agent options failed: ", err)
			}
		}
		if val, ok := mediaURL.Header["Referer"]; ok {
			err = media.AddOptions(":http-referrer=" + val)
			if err != nil {
				log.Warn("add http-referrer options failed: ", err)
			}
		}

		currentMedia = mediaData

		// 播放
		if err := player.SetMedia(media); err != nil {
			log.Error("set media failed: ", err)
			_ = global.EventBus.Publish(events.PlayerPlayErrorUpdate, events.PlayerPlayErrorUpdateEvent{Error: err})
			return
		}

		if currentWindowHandle != 0 {
			if err := setWindowHandle(currentWindowHandle); err != nil {
				log.Error("apply window handle failed: ", err)
			}
		}

		if err := player.Play(); err != nil {
			log.Error("play failed: ", err)
			_ = global.EventBus.Publish(events.PlayerPlayErrorUpdate, events.PlayerPlayErrorUpdateEvent{Error: err})
			return
		}

		// 重置位置信息
		prevPercentPos = 0
		prevTimePos = 0
		_ = global.EventBus.Publish(events.PlayerPropertyTimePosUpdate, events.PlayerPropertyTimePosUpdateEvent{
			TimePos: 0,
		})
		_ = global.EventBus.Publish(events.PlayerPropertyPercentPosUpdate, events.PlayerPropertyPercentPosUpdateEvent{
			PercentPos: 0,
		})
	})

	global.EventBus.Subscribe("", events.PlayerToggleCmd, "player.toggle", func(evnt *eventbus.Event) {
		lock.Lock()
		defer lock.Unlock()
		err := player.TogglePause()
		if err != nil {
			log.Errorf("[VLC Player] Toggle pause failed: %v", err)
			return
		}
	})

	global.EventBus.Subscribe("", events.PlayerSetPauseCmd, "player.set_paused", func(evnt *eventbus.Event) {
		lock.Lock()
		defer lock.Unlock()
		data := evnt.Data.(events.PlayerSetPauseCmdEvent)
		err := player.SetPause(data.Pause)
		if err != nil {
			log.Errorf("[VLC Player] SetPause failed: %v", err)
			return
		}
	})

	global.EventBus.Subscribe("", events.PlayerSeekCmd, "player.seek", func(evnt *eventbus.Event) {
		lock.Lock()
		defer lock.Unlock()
		data := evnt.Data.(events.PlayerSeekCmdEvent)
		var err error
		if data.Absolute {
			err = player.SetMediaTime(int(data.Position * 1000)) // 转换为毫秒
		} else {
			err = player.SetMediaPosition(float32(data.Position / 100))
		}
		if err != nil {
			log.Warn("seek failed", err)
		}
	})

	global.EventBus.Subscribe("", events.PlayerVolumeChangeCmd, "player.volume", func(evnt *eventbus.Event) {
		lock.Lock()
		defer lock.Unlock()
		data := evnt.Data.(events.PlayerVolumeChangeCmdEvent)
		err := player.SetVolume(int(data.Volume))
		if err != nil {
			log.Errorf("[VLC Player] SetVolume failed: %v", err)
		}
	})

	global.EventBus.Subscribe("", events.PlayerVideoPlayerSetWindowHandleCmd, "player.set_window_handle", func(evnt *eventbus.Event) {
		handle := evnt.Data.(events.PlayerVideoPlayerSetWindowHandleCmdEvent).Handle
		if err := setWindowHandle(handle); err != nil {
			log.Warn("set window handle failed", err)
		}
	})

	global.EventBus.Subscribe("", events.PlayerSetAudioDeviceCmd, "player.set_audio_device", func(evnt *eventbus.Event) {
		device := evnt.Data.(events.PlayerSetAudioDeviceCmdEvent).Device
		if err := setAudioDevice(device); err != nil {
			log.Warn("set audio device failed", err)
			_ = global.EventBus.Publish(
				events.ErrorUpdate,
				events.ErrorUpdateEvent{
					Error: err,
				})
		}
	})
}

// setAudioDevice 设置音频输出设备
func setAudioDevice(deviceID string) error {
	lock.Lock()
	defer lock.Unlock()

	if deviceID == "" {
		return nil
	}

	log.Infof("set audio device to: %s", deviceID)

	// 验证设备是否在列表中
	found := false
	for _, dev := range audioDevices {
		if dev.Name == deviceID {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("audio device not found: %s", deviceID)
	}

	// 设置音频设备
	if err := player.SetAudioOutputDevice(deviceID, ""); err != nil {
		log.Error("set audio device failed: ", err)
		return err
	}

	currentAudioDevice = deviceID

	// 更新配置
	cfg.AudioDevice = deviceID

	// 发送更新事件
	_ = global.EventBus.Publish(events.PlayerAudioDeviceUpdate, events.PlayerAudioDeviceUpdateEvent{
		Current: currentAudioDevice,
		Devices: audioDevices,
	})

	return nil
}

// updateAudioDeviceList 获取并更新音频设备列表
func updateAudioDeviceList() {
	lock.Lock()
	defer lock.Unlock()

	// 获取所有音频设备
	devices, err := player.AudioOutputDevices()
	if err != nil {
		log.Error("get audio device list failed: ", err)
		return
	}

	// 获取当前音频设备
	currentDevice, err := player.AudioOutputDevice()
	if err != nil {
		log.Warn("get current audio device failed: ", err)
		currentDevice = ""
	}
	log.Debugf("current audio device list: %s", devices)
	log.Debugf("current audio device: %s", currentDevice)

	// 转换设备格式
	audioDevices = make([]model.AudioDevice, 0, len(devices))
	for _, device := range devices {
		audioDevices = append(audioDevices, model.AudioDevice{
			Name:        device.Name,
			Description: device.Description,
		})
	}

	currentAudioDevice = currentDevice

	log.Infof("update audio device list: %d devices, current: %s",
		len(audioDevices), currentAudioDevice)

	// 发送事件通知
	_ = global.EventBus.Publish(events.PlayerAudioDeviceUpdate, events.PlayerAudioDeviceUpdateEvent{
		Current: currentAudioDevice,
		Devices: audioDevices,
	})
}
