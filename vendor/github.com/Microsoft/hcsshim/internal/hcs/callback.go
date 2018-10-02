package hcs

import (
	"fmt"
	"sync"
	"syscall"

	"github.com/Microsoft/hcsshim/internal/interop"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/sirupsen/logrus"
)

var (
	nextCallback    uintptr
	callbackMap     = map[uintptr]*notifcationWatcherContext{}
	callbackMapLock = sync.RWMutex{}

	notificationWatcherCallback = syscall.NewCallback(notificationWatcher)

	// Notifications for HCS_SYSTEM handles
	hcsNotificationSystemExited                      hcsNotification = 0x00000001
	hcsNotificationSystemCreateCompleted             hcsNotification = 0x00000002
	hcsNotificationSystemStartCompleted              hcsNotification = 0x00000003
	hcsNotificationSystemPauseCompleted              hcsNotification = 0x00000004
	hcsNotificationSystemResumeCompleted             hcsNotification = 0x00000005
	hcsNotificationSystemCrashReport                 hcsNotification = 0x00000006
	hcsNotificationSystemSiloJobCreated              hcsNotification = 0x00000007
	hcsNotificationSystemSaveCompleted               hcsNotification = 0x00000008
	hcsNotificationSystemRdpEnhancedModeStateChanged hcsNotification = 0x00000009
	hcsNotificationSystemShutdownFailed              hcsNotification = 0x0000000A
	hcsNotificationSystemGetPropertiesCompleted      hcsNotification = 0x0000000B
	hcsNotificationSystemModifyCompleted             hcsNotification = 0x0000000C
	hcsNotificationSystemCrashInitiated              hcsNotification = 0x0000000D
	hcsNotificationSystemGuestConnectionClosed       hcsNotification = 0x0000000E

	// Notifications for HCS_PROCESS handles
	hcsNotificationProcessExited hcsNotification = 0x00010000

	// Common notifications
	hcsNotificationInvalid           hcsNotification = 0x00000000
	hcsNotificationServiceDisconnect hcsNotification = 0x01000000
)

type hcsNotification uint32

func (hn hcsNotification) String() string {
	switch hn {
	case hcsNotificationSystemExited:
		return "SystemExited"
	case hcsNotificationSystemCreateCompleted:
		return "SystemCreateCompleted"
	case hcsNotificationSystemStartCompleted:
		return "SystemStartCompleted"
	case hcsNotificationSystemPauseCompleted:
		return "SystemPauseCompleted"
	case hcsNotificationSystemResumeCompleted:
		return "SystemResumeCompleted"
	case hcsNotificationSystemCrashReport:
		return "SystemCrashReport"
	case hcsNotificationSystemSiloJobCreated:
		return "SystemSiloJobCreated"
	case hcsNotificationSystemSaveCompleted:
		return "SystemSaveCompleted"
	case hcsNotificationSystemRdpEnhancedModeStateChanged:
		return "SystemRdpEnhancedModeStateChanged"
	case hcsNotificationSystemShutdownFailed:
		return "SystemShutdownFailed"
	case hcsNotificationSystemGetPropertiesCompleted:
		return "SystemGetPropertiesCompleted"
	case hcsNotificationSystemModifyCompleted:
		return "SystemModifyCompleted"
	case hcsNotificationSystemCrashInitiated:
		return "SystemCrashInitiated"
	case hcsNotificationSystemGuestConnectionClosed:
		return "SystemGuestConnectionClosed"
	case hcsNotificationProcessExited:
		return "ProcessExited"
	case hcsNotificationInvalid:
		return "Invalid"
	case hcsNotificationServiceDisconnect:
		return "ServiceDisconnect"
	default:
		return fmt.Sprintf("Unknown: %d", hn)
	}
}

type notificationChannel chan error

type notifcationWatcherContext struct {
	channels notificationChannels
	handle   hcsCallback

	systemID  string
	processID int
}

type notificationChannels map[hcsNotification]notificationChannel

func newSystemChannels() notificationChannels {
	channels := make(notificationChannels)

	channels[hcsNotificationSystemExited] = make(notificationChannel, 1)
	channels[hcsNotificationSystemCreateCompleted] = make(notificationChannel, 1)
	channels[hcsNotificationSystemStartCompleted] = make(notificationChannel, 1)
	channels[hcsNotificationSystemPauseCompleted] = make(notificationChannel, 1)
	channels[hcsNotificationSystemResumeCompleted] = make(notificationChannel, 1)

	return channels
}

func newProcessChannels() notificationChannels {
	channels := make(notificationChannels)

	channels[hcsNotificationProcessExited] = make(notificationChannel, 1)

	return channels
}

func closeChannels(channels notificationChannels) {
	for _, c := range channels {
		close(c)
	}
}

func notificationWatcher(notificationType hcsNotification, callbackNumber uintptr, notificationStatus uintptr, notificationData *uint16) uintptr {
	var result error
	if int32(notificationStatus) < 0 {
		result = interop.Win32FromHresult(notificationStatus)
	}

	callbackMapLock.RLock()
	context := callbackMap[callbackNumber]
	callbackMapLock.RUnlock()

	if context == nil {
		return 0
	}

	log := logrus.WithFields(logrus.Fields{
		"notification-type": notificationType,
		"system-id":         context.systemID,
	})
	if context.processID != 0 {
		log.Data[logfields.ProcessID] = context.processID
	}
	log.Debug("")

	// The HCS notification system can grow overtime. We explicitly opt-in to
	// the notifications we would like to handle, all others we simply return.
	// This means that as it grows we don't have issues associated with new
	// notification types the code didn't know about.
	switch notificationType {
	case hcsNotificationSystemExited, hcsNotificationSystemCreateCompleted, hcsNotificationSystemStartCompleted, hcsNotificationSystemPauseCompleted, hcsNotificationSystemResumeCompleted:
	case hcsNotificationProcessExited:
	default:
		return 0
	}

	if channel, ok := context.channels[notificationType]; ok {
		channel <- result
	}

	return 0
}
