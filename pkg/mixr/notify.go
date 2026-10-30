package mixr

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/gen2brain/beeep"
	"go.uber.org/zap"

	"github.com/zak-steenhoek/mixr/pkg/mixr/icon"
)

// Notifier provides generic notification sending
type Notifier interface {
	Notify(title string, message string)
}

// ToastNotifier provides toast notifications for Windows
type ToastNotifier struct {
	logger *zap.SugaredLogger
}

// NewToastNotifier creates a new ToastNotifier
func NewToastNotifier(logger *zap.SugaredLogger) (*ToastNotifier, error) {
	logger = logger.Named("notifier")
	tn := &ToastNotifier{logger: logger}

	logger.Debug("Created toast notifier instance")

	return tn, nil
}

// Notify sends a toast notification (or falls back to other types of notification for older Windows versions)
func (tn *ToastNotifier) Notify(title string, message string) {

	// Toast libraries need a filesystem path, so materialize the same embedded
	// icon used by the tray and executable. Replace stale copies after upgrades.
	appIconPath := filepath.Join(os.TempDir(), "mixr.ico")
	existing, _ := ioutil.ReadFile(appIconPath)
	if !bytes.Equal(existing, icon.MixrLogo) {
		tn.logger.Debugw("Creating mixr toast icon", "path", appIconPath)
		if err := ioutil.WriteFile(appIconPath, icon.MixrLogo, 0600); err != nil {
			tn.logger.Errorw("Failed to write toast notification icon", "error", err)
		}
	}

	tn.logger.Infow("Sending toast notification", "title", title, "message", message)

	// send the actual notification
	if err := beeep.Notify(title, message, appIconPath); err != nil {
		tn.logger.Errorw("Failed to send toast notification", "error", err)
	}
}
