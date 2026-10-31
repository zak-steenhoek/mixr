package mixr

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/getlantern/systray"

	"github.com/zak-steenhoek/mixr/pkg/mixr/icon"
	"github.com/zak-steenhoek/mixr/pkg/mixr/util"
)

func (d *Mixr) initializeTray(onDone func()) {
	logger := d.logger.Named("tray")

	onReady := func() {
		logger.Debug("Tray instance ready")

		systray.SetTemplateIcon(icon.MixrLogo, icon.MixrLogo)
		systray.SetTitle("mixr")
		systray.SetTooltip("mixr — serial disconnected")

		connectionStatus := systray.AddMenuItem("Connection: disconnected", "Current USB serial connection state")
		connectionStatus.Disable()
		statusChanges := d.serial.SubscribeToConnectionStatus()
		systray.AddSeparator()

		editConfig := systray.AddMenuItem("Edit configuration", "Open config file with notepad")
		editConfig.SetIcon(icon.EditConfig)
		openLogs := systray.AddMenuItem("Open log folder", "Open mixr's diagnostic log folder")

		refreshSessions := systray.AddMenuItem("Re-scan audio sessions", "Manually refresh audio sessions if something's stuck")
		refreshSessions.SetIcon(icon.RefreshSessions)

		systray.AddSeparator()
		about := systray.AddMenuItem(fmt.Sprintf("About mixr — %s", d.displayVersion()), "mixr build identity")
		about.Disable()

		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit", "Stop mixr and quit")

		// wait on things to happen
		go func() {
			for {
				select {
				case status := <-statusChanges:
					label := string(status)
					if len(label) > 0 {
						label = strings.ToUpper(label[:1]) + label[1:]
					}
					connectionStatus.SetTitle("Connection: " + label)
					systray.SetTooltip("mixr — serial " + string(status))

				// quit
				case <-quit.ClickedCh:
					logger.Info("Quit menu item clicked, stopping")

					d.signalStop()

				// edit config
				case <-editConfig.ClickedCh:
					logger.Info("Edit config menu item clicked, opening config for editing")

					editor := "notepad.exe"
					if util.Linux() {
						editor = "gedit"
					}
					if err := util.OpenExternal(logger, editor, userConfigFilepath); err != nil {
						logger.Warnw("Failed to open config file for editing", "error", err)
					}

				case <-openLogs.ClickedCh:
					logger.Info("Open log folder menu item clicked")
					if err := util.EnsureDirExists(logDirectory); err != nil {
						logger.Warnw("Failed to create log directory", "error", err)
						continue
					}
					logPath, err := filepath.Abs(logDirectory)
					if err != nil {
						logger.Warnw("Failed to resolve log directory", "error", err)
						continue
					}
					opener := "explorer.exe"
					if util.Linux() {
						opener = "xdg-open"
					}
					if err := util.OpenExternal(logger, opener, logPath); err != nil {
						logger.Warnw("Failed to open log directory", "path", logPath, "error", err)
					}
				// refresh sessions
				case <-refreshSessions.ClickedCh:
					logger.Info("Refresh sessions menu item clicked, triggering session map refresh")

					// performance: the reason that forcing a refresh here is okay is that users can't spam the
					// right-click -> select-this-option sequence at a rate that's meaningful to performance
					d.sessions.refreshSessions(true)
				}
			}
		}()

		// actually start the main runtime
		onDone()
	}

	onExit := func() {
		logger.Debug("Tray exited")
	}

	// start the tray icon
	logger.Debug("Running in tray")
	systray.Run(onReady, onExit)
}

func (d *Mixr) stopTray() {
	d.logger.Debug("Quitting tray")
	systray.Quit()
}
