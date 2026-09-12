package core

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// windowRef identifies the terminal window so it can be hidden while the
// player is open and restored when playback ends.
type windowRef struct {
	backend string // "hyprctl" (Wayland) or "xdotool" (X11)
	id      string // window address (hyprctl) or window id (xdotool)
	ws      string // original workspace id (hyprctl) to restore into
}

// parseHyprctlWindow extracts the window address and workspace id from
// `hyprctl activewindow -j` output.
func parseHyprctlWindow(out []byte) (id, ws string, err error) {
	var data struct {
		Address string `json:"address"`
		Mapped  bool   `json:"mapped"`
		WS      struct {
			ID int `json:"id"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return "", "", err
	}
	if data.Address == "" || !data.Mapped {
		return "", "", fmt.Errorf("no mapped window")
	}
	return data.Address, fmt.Sprintf("%d", data.WS.ID), nil
}

// parseXdotoolID extracts the window id from `xdotool getactivewindow`.
func parseXdotoolID(out []byte) (string, error) {
	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", fmt.Errorf("empty window id")
	}
	return id, nil
}

// captureTerminalWindow records the currently active window (the terminal the
// CLI is running in) plus, on Hyprland, its workspace so playback can hide it
// and later restore it. Returns an empty ref when no supported tool exists.
func captureTerminalWindow() windowRef {
	if runtime.GOOS != "linux" {
		return windowRef{}
	}
	if _, err := exec.LookPath("hyprctl"); err == nil {
		out, err := exec.Command("hyprctl", "activewindow", "-j").Output()
		if err != nil {
			return windowRef{}
		}
		id, ws, err := parseHyprctlWindow(out)
		if err != nil {
			return windowRef{}
		}
		return windowRef{backend: "hyprctl", id: id, ws: ws}
	}
	if _, err := exec.LookPath("xdotool"); err == nil {
		out, err := exec.Command("xdotool", "getactivewindow").Output()
		if err != nil {
			return windowRef{}
		}
		id, err := parseXdotoolID(out)
		if err != nil {
			return windowRef{}
		}
		return windowRef{backend: "xdotool", id: id}
	}
	return windowRef{}
}

// hyprctlHideDispatch builds the dispatcher argument that moves the window to
// the hidden pfmini special workspace.
func hyprctlHideDispatch(id string) string {
	return "special:pfmini,address:" + id
}

// hyprctlShowDispatch builds the dispatcher argument that moves the window
// back to its original workspace.
func hyprctlShowDispatch(id, ws string) string {
	if ws == "" {
		ws = "1"
	}
	return ws + ",address:" + id
}

// hideTerminal tucks the terminal away while the player is open. Best-effort;
// failures are silent so playback is never blocked.
func hideTerminal(w windowRef) {
	if w.backend == "" {
		return
	}
	if w.backend == "hyprctl" {
		// Move to a hidden special workspace; it returns to ws on restore.
		_ = exec.Command("hyprctl", "dispatch", "movetoworkspacesilent", hyprctlHideDispatch(w.id)).Run()
		return
	}
	_ = exec.Command("xdotool", "windowminimize", w.id).Run()
}

// showTerminal brings the terminal back to its previous workspace/window
// state after the player exits. Best-effort; failures are silent.
func showTerminal(w windowRef) {
	if w.backend == "" {
		return
	}
	if w.backend == "hyprctl" {
		_ = exec.Command("hyprctl", "dispatch", "movetoworkspacesilent", hyprctlShowDispatch(w.id, w.ws)).Run()
		return
	}
	_ = exec.Command("xdotool", "windowmap", w.id).Run()
	_ = exec.Command("xdotool", "windowactivate", w.id).Run()
}
