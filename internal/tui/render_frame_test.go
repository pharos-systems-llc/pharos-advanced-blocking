package tui

import (
	"strings"
	"testing"

	"github.com/iamrichardd/pharos-advanced-blocking/internal/config"
)

// These tests exercise the ACTUAL View() render output (not just model state).
//
// The original bug: View() padded the content to fill the height AND appended a
// separate spacer of the same size, producing a frame roughly twice the terminal
// height. In the alt-screen renderer that pushed all real content (help text, group
// lists, the command list) off the top of the screen, so the user saw "no output /
// garbled output" even though the model state was correct. State-only unit tests
// could not catch this; these render-output tests can.
//
// assertFits is the key guard: the rendered frame must never exceed the terminal
// height, otherwise the top scrolls away.

func renderTestCfg() *config.Config {
	return &config.Config{
		NetworkGroupMap: map[string]string{
			"192.0.2.50":  "servers",
			"192.0.2.51":  "iot",
			"192.0.2.100": "security",
		},
		Groups: []config.Group{
			{Name: "servers"},
			{Name: "iot"},
			{Name: "security"},
		},
	}
}

// assertFits verifies the rendered frame does not exceed the terminal height.
func assertFits(t *testing.T, out string, height int) {
	t.Helper()
	n := len(strings.Split(out, "\n"))
	if n > height {
		t.Errorf("FRAME OVERFLOW: rendered %d lines but terminal height is %d (content scrolls off the top of the screen)", n, height)
	}
}

func TestView_HelpRendersAndFits(t *testing.T) {
	m := New(renderTestCfg())
	m.width, m.height = 80, 24
	m.executeCommand("help", []string{})
	out := m.View()
	if !strings.Contains(out, "Available Commands") {
		t.Errorf("help text missing from View() output")
	}
	assertFits(t, out, 24)
}

func TestView_ViewGroupsRendersAndFits(t *testing.T) {
	m := New(renderTestCfg())
	m.width, m.height = 80, 24
	m.executeCommand("view", []string{"groups"})
	out := m.View()
	if !strings.Contains(out, "servers") {
		t.Errorf("groups list missing from View() output")
	}
	assertFits(t, out, 24)
}

func TestView_CommandTypeaheadRendersAndFits(t *testing.T) {
	m := New(renderTestCfg())
	m.width, m.height = 80, 24
	m.unifiedInput.SetValue("/")
	m.commandMatches = filterCommands("/")
	m.inTypeaheadMode = true
	m.contentType = ContentTypeCommandList
	out := m.View()
	if !strings.Contains(out, "/help") {
		t.Errorf("command list missing from View() output")
	}
	assertFits(t, out, 24)
}

// TestView_FitsAcrossSizes guards the height math (including the footer-wrap case,
// which used to overflow by one line) across a range of terminal dimensions.
func TestView_FitsAcrossSizes(t *testing.T) {
	sizes := [][2]int{{80, 24}, {80, 30}, {100, 40}, {60, 20}, {120, 50}, {70, 24}, {90, 24}}
	for _, sz := range sizes {
		w, h := sz[0], sz[1]

		mHelp := New(renderTestCfg())
		mHelp.width, mHelp.height = w, h
		mHelp.executeCommand("help", []string{})
		outHelp := mHelp.View()
		if !strings.Contains(outHelp, "Available Commands") {
			t.Errorf("[%dx%d] help text missing", w, h)
		}
		assertFits(t, outHelp, h)

		// /view groups appends to history -> the footer gains the verbose history hint
		// which can wrap to a second line; the content-height math must account for it.
		mGroups := New(renderTestCfg())
		mGroups.width, mGroups.height = w, h
		mGroups.executeCommand("view", []string{"groups"})
		outGroups := mGroups.View()
		if !strings.Contains(outGroups, "servers") {
			t.Errorf("[%dx%d] groups list missing", w, h)
		}
		assertFits(t, outGroups, h)

		// Search/table path with history present.
		mTable := New(renderTestCfg())
		mTable.width, mTable.height = w, h
		mTable.contentType = ContentTypeTable
		mTable.appendHistory("192", "")
		assertFits(t, mTable.View(), h)
	}
}
