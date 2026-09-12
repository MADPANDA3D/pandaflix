package core

import (
	"strings"
	"testing"
)

func TestParseHyprctlWindow(t *testing.T) {
	id, ws, err := parseHyprctlWindow([]byte(`{"address":"0x5589e4992c30","mapped":true,"workspace":{"id":7,"name":"7"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if id != "0x5589e4992c30" || ws != "7" {
		t.Fatalf("unexpected parse: id=%q ws=%q", id, ws)
	}

	if _, _, err := parseHyprctlWindow([]byte(`{"address":"","mapped":true}`)); err == nil {
		t.Fatal("empty address accepted")
	}
	if _, _, err := parseHyprctlWindow([]byte(`{"address":"0x1","mapped":false}`)); err == nil {
		t.Fatal("unmapped window accepted")
	}
	if _, _, err := parseHyprctlWindow([]byte(`not json`)); err == nil {
		t.Fatal("invalid json accepted")
	}
}

func TestParseXdotoolID(t *testing.T) {
	id, err := parseXdotoolID([]byte(" 62914567\n"))
	if err != nil || id != "62914567" {
		t.Fatalf("unexpected id: %q err=%v", id, err)
	}
	if _, err := parseXdotoolID([]byte("  \n")); err == nil {
		t.Fatal("empty id accepted")
	}
}

func TestWindowDispatchers(t *testing.T) {
	// Empty refs must be safe no-ops.
	hideTerminal(windowRef{})
	showTerminal(windowRef{})

	hide := hyprctlHideDispatch("0x1")
	if !strings.HasPrefix(hide, "special:pfmini,address:0x1") {
		t.Fatalf("hide dispatch malformed: %q", hide)
	}
	show := hyprctlShowDispatch("0x1", "7")
	if show != "7,address:0x1" {
		t.Fatalf("show dispatch malformed: %q", show)
	}
	if fallback := hyprctlShowDispatch("0x1", ""); fallback != "1,address:0x1" {
		t.Fatalf("show fallback malformed: %q", fallback)
	}
}
