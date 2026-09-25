package menu

import (
	"sdmm/internal/app/selfupdate"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/app/window"
	"sdmm/internal/env"
	"sdmm/internal/imguiext/icon"
	"sdmm/internal/imguiext/style"
	w "sdmm/internal/imguiext/widget"

	"github.com/SpaiR/imgui-go"
)

func (m *Menu) showUpdateMenu() {
	label := "Update available"
	switch m.updateStatus {
	case upStatusChecking:
		label = "Checking updates..."
	case upStatusUpdating:
		label = "Downloading update..."
	case upStatusUpdated:
		label = "Restart to update"
	case upStatusCurrent:
		label = "Up to date"
	case upStatusError:
		label = "Update needs attention"
	}
	if m.updateNotice != "" && m.updateStatus == upStatusAvailable {
		label = "Beta is behind Stable"
	}
	w.Button(icon.SystemUpdate+" "+label+"##update_status", m.ShowUpdatePopup).
		Style(style.ButtonFrame{}).TextColor(style.Teal).Build()
	if m.updateOpen {
		imgui.OpenPopup("update_menu")
		m.updateOpen = false
	}
	workshop.PushStyle()
	if imgui.BeginPopup("update_menu") {
		width := 380 * window.PointSize()
		imgui.PushTextWrapPosV(width)
		imgui.TextDisabled("Installed: " + env.Version + " (" + selfupdate.CurrentChannel(env.Version).Label() + ")")
		if m.updateStatus == upStatusChecking || m.updateStatus == upStatusUpdating {
			imgui.Text("Release channel: " + m.updateChannel.Label())
		} else {
			imgui.Text("Release channel")
			imgui.SetNextItemWidth(width)
			if imgui.BeginCombo("##release_channel", m.updateChannel.Label()) {
				for _, channel := range []selfupdate.Channel{selfupdate.Stable, selfupdate.Beta} {
					if imgui.SelectableV(channel.Label(), channel == m.updateChannel, 0, imgui.Vec2{}) && channel != m.updateChannel {
						m.app.DoSelectUpdateChannel(channel)
					}
				}
				imgui.EndCombo()
			}
		}
		target := m.updateChannel
		if m.updateVersion != "" && m.updateStatus != upStatusCurrent {
			target = selfupdate.CurrentChannel(m.updateVersion)
		}
		switching := target.Valid() && target != selfupdate.CurrentChannel(env.Version)
		if m.updateChannel == selfupdate.Beta {
			imgui.TextWrapped("Beta includes features still being tested.")
		}
		if m.updateNotice != "" {
			imgui.PushStyleColor(imgui.StyleColorText, style.Amber)
			imgui.TextWrapped(m.updateNotice)
			imgui.PopStyleColor()
		}
		imgui.Separator()
		if m.updateVersion != "" {
			imgui.TextColored(style.Amber, "Voidworks "+m.updateVersion)
		}
		if m.updateDescription != "" {
			imgui.BeginChildV("release_notes", imgui.Vec2{X: width, Y: 125 * window.PointSize()}, false, 0)
			imgui.TextWrapped(m.updateDescription)
			imgui.EndChild()
			imgui.Separator()
		}
		if m.updateError != "" {
			imgui.TextWrapped(m.updateError)
			imgui.Separator()
		}
		switch m.updateStatus {
		case upStatusChecking:
			imgui.Text("Checking GitHub for updates...")
		case upStatusCurrent:
			if m.updateNotice == "" {
				imgui.Text("You're using the latest available version.")
			}
			w.Button("Done", m.doHideUpdateButton).Build()
		case upStatusAvailable:
			label := "Download update"
			if switching {
				label = "Download " + target.Label()
			}
			if workshop.Button(label, true) {
				m.app.DoSelfUpdate()
			}
			// A switch the updater offered (not one picked in the menu) can be skipped.
			if !switching || target != m.updateChannel {
				w.Button("Skip this version", m.doIgnoreUpdate).Build()
			}
		case upStatusUpdating:
			imgui.Text("Downloading and verifying the update...")
			imgui.Text("You can keep working.")
		case upStatusUpdated:
			imgui.Text("The update is ready. Save prompts appear before restarting.")
			label := "Update & restart"
			if switching {
				label = "Switch to " + target.Label() + " & restart"
			}
			if workshop.Button(label, true) {
				imgui.CloseCurrentPopup()
				m.app.DoRestart()
			}
			w.Button("Later", func() { imgui.CloseCurrentPopup() }).Build()
		case upStatusError:
			w.Button("Try again", m.app.DoCheckForUpdates).Build()
			imgui.SameLine()
			w.Button("Download manually", m.app.DoOpenUpdateDownload).Build()
		}
		imgui.PopTextWrapPos()
		imgui.EndPopup()
	}
	workshop.PopStyle()
}

func (m *Menu) ShowUpdatePopup()                            { m.updateOpen = true }
func (m *Menu) SetUpdateChannel(channel selfupdate.Channel) { m.updateChannel = channel }
func (m *Menu) SetChecking() {
	m.updateStatus = upStatusChecking
	m.updateError, m.updateDescription, m.updateVersion, m.updateNotice = "", "", "", ""
}
func (m *Menu) SetUpToDate(version string) {
	m.updateStatus = upStatusCurrent
	m.updateVersion = version
	m.updateError, m.updateDescription = "", ""
}
func (m *Menu) SetRestartError(message string) {
	m.updateStatus = upStatusUpdated
	m.updateError = message
	m.ShowUpdatePopup()
}
func (m *Menu) doHideUpdateButton() { m.updateStatus = upStatusNone; imgui.CloseCurrentPopup() }
func (m *Menu) doIgnoreUpdate()     { m.doHideUpdateButton(); m.app.DoIgnoreUpdate() }
