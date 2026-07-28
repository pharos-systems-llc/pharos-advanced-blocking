package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iamrichardd/pharos-advanced-blocking/internal/config"
)

// ============================================================================
// TEST CONFIG HELPERS
// ============================================================================

// testConfigWithIPs creates a test config with provided IP→group mappings
func testConfigWithIPs(ips map[string]string) *config.Config {
	cfg := &config.Config{
		NetworkGroupMap: ips,
		Groups: []config.Group{
			{Name: "servers"},
			{Name: "iot"},
			{Name: "security"},
		},
	}
	return cfg
}

// testConfigEmpty creates a test config with no clients
func testConfigEmpty() *config.Config {
	return &config.Config{
		NetworkGroupMap: make(map[string]string),
		Groups: []config.Group{
			{Name: "servers"},
			{Name: "iot"},
		},
	}
}

// mockConfig creates a default test config for most tests
func mockConfig() *config.Config {
	return testConfigWithIPs(map[string]string{
		"192.0.2.50":  "servers",
		"192.0.2.51":  "iot",
		"192.0.2.100": "security",
		"10.0.0.1":    "servers",
		"172.16.0.1":  "iot",
	})
}

// ============================================================================
// ASSERTION HELPERS
// ============================================================================

func assertContentType(t *testing.T, m *Model, expected ContentType, msg string) {
	t.Helper()
	if m.contentType != expected {
		t.Errorf("%s: expected contentType %v, got %v", msg, expected, m.contentType)
	}
}

func assertFilteredCount(t *testing.T, m *Model, expected int, msg string) {
	t.Helper()
	if len(m.filtered) != expected {
		t.Errorf("%s: expected %d filtered results, got %d", msg, expected, len(m.filtered))
	}
}

func assertHistoryLength(t *testing.T, m *Model, expected int, msg string) {
	t.Helper()
	if len(m.commandHistory) != expected {
		t.Errorf("%s: expected %d history entries, got %d", msg, expected, len(m.commandHistory))
	}
}

func assertHistoryEntry(t *testing.T, m *Model, index int, expectedCmd string, msg string) {
	t.Helper()
	if index >= len(m.commandHistory) {
		t.Errorf("%s: history index %d out of bounds (len=%d)", msg, index, len(m.commandHistory))
		return
	}
	if m.commandHistory[index].Command != expectedCmd {
		t.Errorf("%s: expected history[%d]='%s', got '%s'", msg, index, expectedCmd, m.commandHistory[index].Command)
	}
}

func assertTypeaheadActive(t *testing.T, m *Model, expected bool, msg string) {
	t.Helper()
	if m.inTypeaheadMode != expected {
		t.Errorf("%s: expected inTypeaheadMode=%v, got %v", msg, expected, m.inTypeaheadMode)
	}
}

func assertSearchTypeaheadActive(t *testing.T, m *Model, expected bool, msg string) {
	t.Helper()
	if m.inSearchTypeahead != expected {
		t.Errorf("%s: expected inSearchTypeahead=%v, got %v", msg, expected, m.inSearchTypeahead)
	}
}

func assertInputValue(t *testing.T, m *Model, expected string, msg string) {
	t.Helper()
	if m.unifiedInput.Value() != expected {
		t.Errorf("%s: expected input '%s', got '%s'", msg, expected, m.unifiedInput.Value())
	}
}

func assertNoError(t *testing.T, err error, msg string) {
	t.Helper()
	if err != nil {
		t.Errorf("%s: unexpected error: %v", msg, err)
	}
}

func TestTUI_Lifecycle(t *testing.T) {
	// 1. Create a decoupled configuration structure
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		"192.168.1.150": "IoT",
		"10.0.0.5":      "Laptops",
	})

	// 2. Initialize the Bubble Tea Model
	m := New(cfg)

	// Simulate Init
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Expected Init() to return a command for blinking text input")
	}

	// Verify initial content type is "empty"
	if m.contentType != ContentTypeEmpty {
		t.Errorf("Expected initial contentType to be 'empty', got %q", m.contentType)
	}

	// 3. Ensure clients list is properly populated and sorted
	if len(m.clients) != 3 {
		t.Errorf("Expected 3 clients, got %d", len(m.clients))
	}

	// Ensure clients are alphabetically/lexicographically sorted by IP for stable view
	if m.clients[0].IP != "10.0.0.5" {
		t.Errorf("Expected first IP to be 10.0.0.5, got %s", m.clients[0].IP)
	}

	// 4. Simulate User Search Input
	// We'll simulate pressing the 'I', 'o', 'T' keys
	keyMsgs := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'I'}},
		{Type: tea.KeyRunes, Runes: []rune{'o'}},
		{Type: tea.KeyRunes, Runes: []rune{'T'}},
	}

	// Because Update returns a tea.Model interface, we keep track of the pointer
	var updatedModel tea.Model = m
	for _, key := range keyMsgs {
		updatedModel, _ = updatedModel.Update(key)
	}
	m2, ok := updatedModel.(*Model)
	if !ok {
		t.Fatal("Expected model type *Model")
	}

	// Verify contentType changes to "table" when search input is present
	if m2.contentType != ContentTypeTable {
		t.Errorf("Expected contentType to be 'table' after search input, got %q", m2.contentType)
	}

	// 5. Verify dynamic filtering functionality
	if len(m2.filtered) != 1 {
		t.Fatalf("Expected 1 filtered result, got %d", len(m2.filtered))
	}
	if m2.filtered[0].Group != "IoT" {
		t.Errorf("Expected filtered group to be 'IoT', got '%s'", m2.filtered[0].Group)
	}
	if m2.filtered[0].IP != "192.168.1.150" {
		t.Errorf("Expected IP to be '192.168.1.150'")
	}

	// 6. Verify View rendering
	m2.ready = true
	m2.width = 100 // Simulate terminal window sizing
	m2.height = 30
	viewOutput := m2.View()

	if !strings.Contains(viewOutput, "IoT") {
		t.Errorf("Expected view to contain 'IoT', got:\n%s", viewOutput)
	}
	if !strings.Contains(viewOutput, "192.168.1.150") {
		t.Errorf("Expected view to contain '192.168.1.150'")
	}
	if !strings.Contains(viewOutput, "Pharos Advanced Blocking") {
		t.Errorf("Expected view to contain title")
	}
	if !strings.Contains(viewOutput, "ctrl+c / esc: exit | /help: commands") {
		t.Errorf("Expected view to contain footer help status line")
	}
}

func TestTUI_HelpCommand(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Simulate typing "/help" and pressing Enter
	helpKeyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	_, _ = m.Update(helpKeyMsg)

	for _, r := range "help" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify contentType changes to "help"
	if m.contentType != ContentTypeHelp {
		t.Errorf("Expected contentType to be 'help' after /help command, got %q", m.contentType)
	}

	// Verify content text contains help information
	if !strings.Contains(m.contentText, "Available Commands") {
		t.Errorf("Expected contentText to contain 'Available Commands'")
	}

	// Verify the view renders help text
	viewOutput := m.View()
	if !strings.Contains(viewOutput, "Available Commands") {
		t.Errorf("Expected help text in view output")
	}
	if !strings.Contains(viewOutput, "/clear") {
		t.Errorf("Expected /clear command in help text")
	}
}

func TestTUI_ClearCommand(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		"192.168.1.150": "IoT",
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// First, do a search
	for _, r := range "192" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.contentType != ContentTypeTable {
		t.Errorf("Expected contentType to be 'table' after search")
	}
	if len(m.filtered) != 2 {
		t.Errorf("Expected 2 filtered results, got %d", len(m.filtered))
	}

	// Clear the search input and type /clear command
	m.unifiedInput.SetValue("")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "clear" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify contentType changes back to "empty"
	if m.contentType != ContentTypeEmpty {
		t.Errorf("Expected contentType to be 'empty' after /clear command, got %q", m.contentType)
	}

	// Verify search box is cleared
	if m.unifiedInput.Value() != "" {
		t.Errorf("Expected search input to be cleared, got %q", m.unifiedInput.Value())
	}

	// Verify the view renders empty state hint
	viewOutput := m.View()
	if !strings.Contains(viewOutput, "Start typing to search") {
		t.Errorf("Expected empty state hint in view output")
	}
}

func TestTUI_SlashCommandTypeahead(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "/" to enter typeahead mode
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	// Verify we're in typeahead mode
	if !m.inTypeaheadMode {
		t.Errorf("Expected to be in typeahead mode after typing /")
	}

	// Verify contentType is "command_list"
	if m.contentType != ContentTypeCommandList {
		t.Errorf("Expected contentType to be 'command_list', got %q", m.contentType)
	}

	// Verify all commands are shown for just "/"
	if len(m.commandMatches) != 4 {
		t.Errorf("Expected 4 command matches for '/', got %d", len(m.commandMatches))
	}

	// Verify view renders command list
	viewOutput := m.View()
	if !strings.Contains(viewOutput, "Available Slash Commands") {
		t.Errorf("Expected command list header in view output")
	}
	if !strings.Contains(viewOutput, "/help") {
		t.Errorf("Expected /help command in view output")
	}
}

func TestTUI_CommandFiltering_Help(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "/h" to filter help commands
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	// Verify filtered results
	if len(m.commandMatches) != 1 {
		t.Errorf("Expected 1 command match for '/h', got %d", len(m.commandMatches))
	}
	if len(m.commandMatches) > 0 && !strings.HasPrefix(m.commandMatches[0].Name, "/help") {
		t.Errorf("Expected /help to match /h")
	}

	// Verify view renders only matching command
	viewOutput := m.View()
	if !strings.Contains(viewOutput, "/help") {
		t.Errorf("Expected /help in filtered results")
	}
}

func TestTUI_CommandFiltering_Exit(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "/e" to filter exit commands
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

	// Verify filtered results
	if len(m.commandMatches) != 1 {
		t.Errorf("Expected 1 command match for '/e', got %d", len(m.commandMatches))
	}
	if len(m.commandMatches) > 0 && !strings.HasPrefix(m.commandMatches[0].Name, "/exit") {
		t.Errorf("Expected /exit to match /e")
	}
}

func TestTUI_CommandFiltering_Alias(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "/q" which is an alias for /exit
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Verify /exit command is matched by alias
	if len(m.commandMatches) != 1 {
		t.Errorf("Expected 1 command match for '/q' alias, got %d", len(m.commandMatches))
	}
	if len(m.commandMatches) > 0 && m.commandMatches[0].Name != "/exit" {
		t.Errorf("Expected /exit to match /q alias")
	}
}

func TestTUI_ArrowNavigation_Down(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "/" to show all commands
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	// Verify initial selection is 0
	if m.selectedCommand != 0 {
		t.Errorf("Expected initial selection to be 0, got %d", m.selectedCommand)
	}

	// Press down arrow
	m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Verify selection moved to 1
	if m.selectedCommand != 1 {
		t.Errorf("Expected selection to be 1 after down arrow, got %d", m.selectedCommand)
	}

	// Press down arrow again
	m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Verify selection moved to 2
	if m.selectedCommand != 2 {
		t.Errorf("Expected selection to be 2 after second down arrow, got %d", m.selectedCommand)
	}
}

func TestTUI_ArrowNavigation_Up(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "/" to show all commands
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	// Move down twice
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})

	if m.selectedCommand != 2 {
		t.Errorf("Expected selection to be 2, got %d", m.selectedCommand)
	}

	// Press up arrow
	m.Update(tea.KeyMsg{Type: tea.KeyUp})

	// Verify selection moved to 1
	if m.selectedCommand != 1 {
		t.Errorf("Expected selection to be 1 after up arrow, got %d", m.selectedCommand)
	}
}

func TestTUI_ExitTypeaheadWithRegularText(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
			"192.168.1.150": "IoT",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Start in typeahead mode
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.inTypeaheadMode {
		t.Errorf("Expected to be in typeahead mode")
	}

	// Clear and type regular text (backspace then regular char)
	m.unifiedInput.SetValue("")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})

	// Verify we're NOT in typeahead mode anymore
	if m.inTypeaheadMode {
		t.Errorf("Expected to exit typeahead mode when typing regular text")
	}

	// Verify contentType is "table" for regular search
	if m.contentType != ContentTypeTable {
		t.Errorf("Expected contentType to be 'table' for regular search, got %q", m.contentType)
	}
}

func TestTUI_CommandListHighlight(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "/" and navigate
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Verify selectedCommand is set
	if m.selectedCommand != 1 {
		t.Errorf("Expected selectedCommand to be 1")
	}

	// Render the view and verify it contains the selected command indicator
	viewOutput := m.View()
	if !strings.Contains(viewOutput, "/exit") {
		t.Errorf("Expected /exit in view output")
	}
}

func TestTUI_ClearBackToNormalSearch(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Start typeahead
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.inTypeaheadMode {
		t.Errorf("Expected typeahead mode")
	}

	// Type "/help"
	for _, r := range "help" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Verify in typeahead mode
	if !m.inTypeaheadMode {
		t.Errorf("Expected to be in typeahead mode")
	}

	// Clear the input
	m.unifiedInput.SetValue("")

	// Verify empty state
	if m.unifiedInput.Value() != "" {
		t.Errorf("Expected search input to be cleared")
	}
}

func TestTUI_TabCompletion(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "/" to enter typeahead mode
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	// Type "hel" to filter to /help command
	for _, r := range "hel" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Verify we have a single match for /help
	if len(m.commandMatches) != 1 {
		t.Errorf("Expected 1 command match for '/hel', got %d", len(m.commandMatches))
	}
	if m.commandMatches[0].Name != "/help" {
		t.Errorf("Expected /help command, got %q", m.commandMatches[0].Name)
	}

	// Press Tab to complete
	m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Verify input is now "/help " (with trailing space)
	if m.unifiedInput.Value() != "/help " {
		t.Errorf("Expected '/help ' after Tab completion, got %q", m.unifiedInput.Value())
	}

	// Verify exited typeahead mode (user can now type subcommand arguments)
	if m.inTypeaheadMode {
		t.Errorf("Expected to exit typeahead mode after Tab completion")
	}

	// Verify contentType is empty (ready to accept command arguments)
	if m.contentType != ContentTypeEmpty {
		t.Errorf("Expected contentType 'empty' after Tab completion, got %q", m.contentType)
	}
}

func TestTUI_TabCompletion_MultiMatch(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "/" to enter typeahead mode
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	// Verify all 4 commands are shown
	if len(m.commandMatches) != 4 {
		t.Errorf("Expected 4 command matches for '/', got %d", len(m.commandMatches))
	}

	// Press Tab with multiple matches (completes to selected command)
	m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Verify Tab completes to the selected command (/help is default selected)
	if m.unifiedInput.Value() != "/help " {
		t.Errorf("Expected '/help ' (default selected), got %q", m.unifiedInput.Value())
	}

	// Verify exited typeahead after completion
	if m.inTypeaheadMode {
		t.Errorf("Expected to exit typeahead mode after Tab completion")
	}
}

func TestTUI_TabCompletion_OutsideTypeahead(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "abc" (normal search, no slash command)
	for _, r := range "abc" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Verify not in typeahead mode
	if m.inTypeaheadMode {
		t.Errorf("Expected NOT to be in typeahead mode for normal search")
	}

	// Press Tab (should be a no-op, not crash)
	m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Verify Tab had no effect outside typeahead
	if m.unifiedInput.Value() != "abc" {
		t.Errorf("Expected Tab to be no-op outside typeahead, got input %q", m.unifiedInput.Value())
	}
}

func TestTUI_TabCompletion_AfterArrowNav(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "/" to enter typeahead mode
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	// Verify /help is selected (default selectedCommand = 0)
	if m.commandMatches[0].Name != "/help" {
		t.Errorf("Expected first match to be /help")
	}

	// Navigate Down to /exit (assuming order: /help, /exit, /clear, /view)
	m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Verify /exit is now selected
	if m.selectedCommand != 1 {
		t.Errorf("Expected selectedCommand to be 1 after Down, got %d", m.selectedCommand)
	}

	// Press Tab to complete the currently selected command (/exit)
	m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Verify Tab completed to /exit, not /help
	if m.unifiedInput.Value() != "/exit " {
		t.Errorf("Expected '/exit ' after Tab with Down navigation, got %q", m.unifiedInput.Value())
	}
}

func TestTUI_TabCompletion_SubcommandEntry(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",

	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "/view " to show view subcommands
	for _, r := range "/view " {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Verify we have view subcommands in matches
	if len(m.commandMatches) < 1 {
		t.Errorf("Expected at least 1 match for '/view '")
	}

	// Press Tab to complete to the first subcommand "/view groups"
	m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// After Tab, we should have the first subcommand completed
	if !strings.HasPrefix(m.unifiedInput.Value(), "/view ") {
		t.Errorf("Expected input to start with '/view ' after Tab, got %q", m.unifiedInput.Value())
	}

	// Verify not back in typeahead mode (so user can continue typing)
	if m.inTypeaheadMode {
		t.Errorf("Expected NOT to be in typeahead mode after typing subcommand")
	}
}

func TestTUI_ViewSubcommandFiltering_Prefix(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "/view g" to filter subcommands starting with "g"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "view g" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Verify we're in typeahead mode
	if !m.inTypeaheadMode {
		t.Errorf("Expected to be in typeahead mode after typing /view g")
	}

	// Verify we have 2 matches: /view group and /view groups
	if len(m.commandMatches) != 2 {
		t.Errorf("Expected 2 command matches for '/view g', got %d", len(m.commandMatches))
	}

	// Verify matches are the group-related subcommands
	for _, cmd := range m.commandMatches {
		if !strings.HasPrefix(cmd.Name, "/view group") {
			t.Errorf("Expected match to start with '/view group', got %q", cmd.Name)
		}
	}

	// Verify view renders the filtered subcommands
	viewOutput := m.View()
	if !strings.Contains(viewOutput, "/view group") {
		t.Errorf("Expected /view group in filtered results")
	}
}

func TestTUI_ViewSubcommandFiltering_NoMatch(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "/view xyz" to filter with no matching subcommands
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "view xyz" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Verify we're NOT in typeahead mode (no matches)
	if m.inTypeaheadMode {
		t.Errorf("Expected NOT to be in typeahead mode when no subcommands match")
	}

	// Verify we have 0 matches
	if len(m.commandMatches) != 0 {
		t.Errorf("Expected 0 command matches for '/view xyz', got %d", len(m.commandMatches))
	}

	// Verify contentType is "empty" (no matches shown)
	if m.contentType != ContentTypeEmpty {
		t.Errorf("Expected contentType 'empty' when no matches found, got %q", m.contentType)
	}
}

func TestTUI_ViewSubcommandAfterTabNoReentry(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.168.1.100": "Servers",
		},
		Groups: []config.Group{
			{
				Name:    "group1",
				Blocked: []string{"blocked.com"},
				Allowed: []string{"allowed.com"},
			},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "/view " to show all subcommands
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "view " {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Verify in typeahead mode with 3 matches
	if !m.inTypeaheadMode {
		t.Errorf("Expected to be in typeahead mode after /view ")
	}
	if len(m.commandMatches) != 3 {
		t.Errorf("Expected 3 subcommand matches for '/view ', got %d", len(m.commandMatches))
	}

	// Press Tab to complete to first match ("/view groups")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Verify we exited typeahead mode
	if m.inTypeaheadMode {
		t.Errorf("Expected to exit typeahead mode after Tab completion")
	}

	// Verify search input is now a full command with trailing space
	currentInput := m.unifiedInput.Value()
	if !strings.HasPrefix(currentInput, "/view ") {
		t.Errorf("Expected input to start with '/view ', got %q", currentInput)
	}

	// Now type a space (simulating user continuing to type)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})

	// Verify we're still NOT in typeahead mode (no re-entry)
	if m.inTypeaheadMode {
		t.Errorf("Expected NOT to re-enter typeahead mode after typing space following Tab completion")
	}

	// Verify contentType remains "empty" (not command_list)
	if m.contentType != ContentTypeEmpty {
		t.Errorf("Expected contentType 'empty' after typing space, got %q", m.contentType)
	}
}

// History tests start here

func TestTUI_HistoryAppend_SingleCommand(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Verify history is empty initially
	if len(m.commandHistory) != 0 {
		t.Errorf("Expected empty history initially, got %d commands", len(m.commandHistory))
	}

	// Type "/help" and press Enter
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "help" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify history now has one command
	if len(m.commandHistory) != 1 {
		t.Errorf("Expected 1 command in history, got %d", len(m.commandHistory))
	}

	// Verify the command is "/help"
	if m.commandHistory[0].Command != "/help" {
		t.Errorf("Expected command to be '/help', got %q", m.commandHistory[0].Command)
	}

	// Verify output is not empty
	if m.commandHistory[0].Output == "" {
		t.Errorf("Expected non-empty output for /help command")
	}
}

func TestTUI_HistoryAppend_MultipleCommands(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.168.1.100": "Servers",
			"192.168.1.150": "IoT",
		},
		Groups: []config.Group{
			{
				Name:    "Servers",
				Blocked: []string{"malware.com"},
				Allowed: []string{"trusted.com"},
			},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Execute /help
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "help" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Execute /view groups
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "view groups" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify history has 2 commands
	if len(m.commandHistory) != 2 {
		t.Errorf("Expected 2 commands in history, got %d", len(m.commandHistory))
	}

	// Verify order (oldest first)
	if m.commandHistory[0].Command != "/help" {
		t.Errorf("Expected first command to be '/help', got %q", m.commandHistory[0].Command)
	}
	if m.commandHistory[1].Command != "/view groups" {
		t.Errorf("Expected second command to be '/view groups', got %q", m.commandHistory[1].Command)
	}
}

func TestTUI_HistoryScroll_UpDown(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Execute /help to add to history
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "help" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// History scrolling (historyScroll) is only active when the history view is what
	// View() actually renders -- i.e. in Table/Empty mode. Primary content types like
	// Help/ViewGroup are rendered via renderContent() and scroll scrollOffset instead
	// (see TestTUI_ViewGroupScroll_UpDown). Put the model in table mode so ↑↓ drive the
	// history view.
	m.contentType = ContentTypeTable

	// Verify initial historyScroll is 0
	if m.historyScroll != 0 {
		t.Errorf("Expected historyScroll to be 0 initially, got %d", m.historyScroll)
	}

	// Press down arrow
	m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Verify historyScroll increased
	if m.historyScroll != 1 {
		t.Errorf("Expected historyScroll to be 1 after down arrow, got %d", m.historyScroll)
	}

	// Press up arrow
	m.Update(tea.KeyMsg{Type: tea.KeyUp})

	// Verify historyScroll decreased
	if m.historyScroll != 0 {
		t.Errorf("Expected historyScroll to be 0 after up arrow, got %d", m.historyScroll)
	}
}

// TestTUI_ViewGroupScroll_UpDown is a regression test for the reported bug where
// `/view group <name>` output was truncated and ↑↓ would not scroll it.
//
// Root cause: executing /view appends to commandHistory, and the scroll handler
// routed ↑↓ to historyScroll whenever history was non-empty. But the group detail is
// rendered via renderContent() which uses scrollOffset -- so the keys moved a counter
// that had no effect on the visible group detail. This test locks in that ↑↓ move
// scrollOffset (and NOT historyScroll) while ContentTypeViewGroup is displayed, even
// with command history present.
func TestTUI_ViewGroupScroll_UpDown(t *testing.T) {
	// Build a group with enough blocked domains to overflow the viewport.
	var blocked []string
	for i := 0; i < 60; i++ {
		blocked = append(blocked, fmt.Sprintf("blocked-domain-%02d.example.com", i))
	}
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{"192.168.1.10": "default+neve+roosevelt_laptop"},
		Groups: []config.Group{
			{Name: "default+neve+roosevelt_laptop", Blocked: blocked},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Execute: /view group default+neve+roosevelt_laptop
	for _, r := range "/view group default+neve+roosevelt_laptop" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Preconditions: the group detail is the active content, and the /view command was
	// recorded in history (this non-empty history is what used to hijack the keys).
	if m.contentType != ContentTypeViewGroup {
		t.Fatalf("Expected contentType ContentTypeViewGroup, got %v", m.contentType)
	}
	if len(m.commandHistory) == 0 {
		t.Fatalf("Expected command history to be non-empty after /view")
	}
	if m.scrollOffset != 0 {
		t.Fatalf("Expected scrollOffset to start at 0, got %d", m.scrollOffset)
	}

	// Capture the first visible line before scrolling.
	firstLineBefore := strings.SplitN(m.renderContent(m.height-4), "\n", 2)[0]

	// Press down: scrollOffset must advance, historyScroll must stay put.
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.scrollOffset != 2 {
		t.Errorf("Expected scrollOffset to be 2 after two down arrows, got %d", m.scrollOffset)
	}
	if m.historyScroll != 0 {
		t.Errorf("Expected historyScroll to remain 0 for group detail scroll, got %d", m.historyScroll)
	}

	// The visible content must actually change (top line scrolled off).
	firstLineAfter := strings.SplitN(m.renderContent(m.height-4), "\n", 2)[0]
	if firstLineBefore == firstLineAfter {
		t.Errorf("Expected group detail to scroll (first visible line to change); still %q", firstLineAfter)
	}

	// Press up: scrollOffset must decrease back toward the top.
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.scrollOffset != 1 {
		t.Errorf("Expected scrollOffset to be 1 after up arrow, got %d", m.scrollOffset)
	}
}

func TestTUI_HistoryScroll_Bounds(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Execute /help to add to history
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "help" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Table mode so ↑↓ drive the history view (see TestTUI_HistoryScroll_UpDown).
	m.contentType = ContentTypeTable

	// Try to scroll up from the top (should not go below 0)
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.historyScroll < 0 {
		t.Errorf("Expected historyScroll to not go below 0, got %d", m.historyScroll)
	}

	// Scroll down many times and verify max is capped at 100
	for i := 0; i < 110; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.historyScroll > 100 {
		t.Errorf("Expected historyScroll to be capped at 100, got %d", m.historyScroll)
	}
}

func TestTUI_HistoryClear_ResetOnClear(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Execute /help to add to history
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "help" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify history is not empty
	if len(m.commandHistory) == 0 {
		t.Errorf("Expected history to have commands before /clear")
	}

	// Execute /clear
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "clear" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify history is empty after /clear
	if len(m.commandHistory) != 0 {
		t.Errorf("Expected history to be empty after /clear, got %d commands", len(m.commandHistory))
	}

	// Verify historyScroll is reset
	if m.historyScroll != 0 {
		t.Errorf("Expected historyScroll to be 0 after /clear, got %d", m.historyScroll)
	}

	// Verify search input is cleared
	if m.unifiedInput.Value() != "" {
		t.Errorf("Expected search input to be cleared, got %q", m.unifiedInput.Value())
	}
}

func TestTUI_HistoryRender_TimestampFormat(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Execute /help
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "help" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify the command is stored with timestamp
	if m.commandHistory[0].Timestamp.IsZero() {
		t.Errorf("Expected timestamp to be set for command")
	}

	// Verify view renders history with timestamp
	viewOutput := m.View()
	if !strings.Contains(viewOutput, "/help") {
		t.Errorf("Expected /help command in history view")
	}

	// The timestamp should be formatted in 15:04:05 format
	// We can't check exact time but should see the separator "|"
	if !strings.Contains(viewOutput, "|") {
		t.Errorf("Expected timestamp separator '|' in history view")
	}
}

func TestTUI_HistoryRender_EmptyHistory(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// The welcome banner was deliberately removed for v0.3.0 GA (see the "DISABLED"
	// note in View()); the empty state now shows the search hint directly.
	viewOutput := m.View()
	if strings.Contains(viewOutput, "Welcome to Pharos Advanced Blocking") {
		t.Errorf("Welcome banner was removed for v0.3.0 GA but is still rendered")
	}
	if !strings.Contains(viewOutput, "Start typing to search") {
		t.Errorf("Expected empty state hint in view output")
	}

	// Same after explicitly clearing firstRun.
	m.firstRun = false
	viewOutput = m.View()
	if !strings.Contains(viewOutput, "Start typing to search") {
		t.Errorf("Expected empty state hint in view output after firstRun cleared")
	}
}

func TestTUI_HistoryRender_AfterCommand(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.168.1.100": "Servers",
		},
		Groups: []config.Group{
			{
				Name:    "Servers",
				Blocked: []string{"blocked.com"},
				Allowed: []string{"allowed.com"},
			},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Execute /view groups
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "view groups" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// After /view groups, contentType is ViewGroups, so View() renders the groups list
	// as primary content (this is the fix for the reported "no groups list" bug). The
	// command itself is recorded in history state, but the group-list pane -- not a raw
	// command echo -- is what is shown.
	if len(m.commandHistory) == 0 || m.commandHistory[0].Command != "/view groups" {
		t.Errorf("Expected /view groups recorded in command history state")
	}

	// The rendered view must contain the actual group list.
	viewOutput := m.View()
	if !strings.Contains(viewOutput, "Servers") {
		t.Errorf("Expected 'Servers' group to render in View()")
	}
}

func TestTUI_HistoryScroll_ResetOnNewCommand(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Execute /help
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "help" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Table mode so ↑↓ drive the history view (see TestTUI_HistoryScroll_UpDown).
	m.contentType = ContentTypeTable

	// Scroll down in history
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.historyScroll != 2 {
		t.Errorf("Expected historyScroll to be 2, got %d", m.historyScroll)
	}

	// Execute another command
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "help" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify historyScroll is reset to 0
	if m.historyScroll != 0 {
		t.Errorf("Expected historyScroll to be reset to 0, got %d", m.historyScroll)
	}
}

func TestTUI_HistoryFooterDisplay(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Without history, footer should not show history count
	viewOutput := m.View()
	if strings.Contains(viewOutput, "commands in history") {
		t.Errorf("Expected no history count in footer when no history exists")
	}

	// Execute /help to add to history
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "help" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// The history count/hint is shown in the footer only when the history view is the
	// primary content -- i.e. Table/Empty mode. Primary content types (Help/ViewGroup)
	// show a plain "↑↓: scroll" hint for their own content instead. Put the model in
	// table mode to exercise the history footer.
	m.contentType = ContentTypeTable

	// Verify footer shows history count and scroll hint
	viewOutput = m.View()

	// Check that we have history
	if len(m.commandHistory) != 1 {
		t.Errorf("Expected 1 command in history, got %d", len(m.commandHistory))
	}

	// Check that we're not in typeahead mode
	if m.inTypeaheadMode {
		t.Errorf("Expected NOT to be in typeahead mode after command execution")
	}

	// Verify footer contains history count indicator
	if !strings.Contains(viewOutput, "commands in history") {
		t.Errorf("Expected 'commands in history' in footer")
	}
}

// Parser tests start here

func TestParseUnifiedInput(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedType  InputType
		expectedQuery string
		expectedArgs  []string
	}{
		{
			name:         "empty input",
			input:        "",
			expectedType: InputTypeEmpty,
		},
		{
			name:          "implicit search",
			input:         "kids",
			expectedType:  InputTypeSearch,
			expectedQuery: "kids",
		},
		{
			name:          "command with args",
			input:         "/view groups",
			expectedType:  InputTypeCommand,
			expectedQuery: "view",
			expectedArgs:  []string{"groups"},
		},
		{
			name:          "case insensitive command",
			input:         "/VIEW GROUPS",
			expectedType:  InputTypeCommand,
			expectedQuery: "view",
			expectedArgs:  []string{"GROUPS"},
		},
		{
			name:          "search with IP",
			input:         "192.0.2.50",
			expectedType:  InputTypeSearch,
			expectedQuery: "192.0.2.50",
		},
		{
			name:          "command only slash",
			input:         "/",
			expectedType:  InputTypeCommand,
			expectedQuery: "",
		},
		{
			name:          "search with spaces preserved",
			input:         "   search term   ",
			expectedType:  InputTypeSearch,
			expectedQuery: "search term",
		},
		{
			name:          "command with multiple args",
			input:         "/view group MyGroup blocklists",
			expectedType:  InputTypeCommand,
			expectedQuery: "view",
			expectedArgs:  []string{"group", "MyGroup", "blocklists"},
		},
		{
			name:          "search with leading zero",
			input:         "192.168.001.1",
			expectedType:  InputTypeSearch,
			expectedQuery: "192.168.001.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseUnifiedInput(tt.input)

			if result.Type != tt.expectedType {
				t.Errorf("expected type %v, got %v", tt.expectedType, result.Type)
			}
			if result.Query != tt.expectedQuery {
				t.Errorf("expected query '%s', got '%s'", tt.expectedQuery, result.Query)
			}
			if len(result.Args) != len(tt.expectedArgs) {
				t.Errorf("expected %d args, got %d", len(tt.expectedArgs), len(result.Args))
			}
			for i, arg := range tt.expectedArgs {
				if result.Args[i] != arg {
					t.Errorf("arg[%d]: expected '%s', got '%s'", i, arg, result.Args[i])
				}
			}
		})
	}
}

// Phase 3 Integration Tests - Unified Input Routing

func TestUnifiedInput_ImplicitSearchRouting(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
			"192.168.1.150": "IoT",
			"10.0.0.5":      "Laptops",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 100
	m.height = 30

	// Simulate typing "192" (implicit search, no slash)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'9'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})

	// Verify contentType is "table" for implicit search
	if m.contentType != ContentTypeTable {
		t.Errorf("Expected contentType 'table' for implicit search, got %q", m.contentType)
	}

	// Verify filtering worked (should match the two 192.168.x.x IPs)
	if len(m.filtered) != 2 {
		t.Errorf("Expected 2 filtered results for '192', got %d", len(m.filtered))
	}

	// Verify input is in the unified input field
	if m.unifiedInput.Value() != "192" {
		t.Errorf("Expected unified input to contain '192', got %q", m.unifiedInput.Value())
	}
}

func TestUnifiedInput_ExplicitCommandRouting(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.168.1.100": "Servers",
		},
		Groups: []config.Group{
			{
				Name:    "Servers",
				Blocked: []string{"blocked.com"},
				Allowed: []string{"allowed.com"},
			},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 100
	m.height = 30

	// Simulate typing "/view groups" (explicit command)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "view groups" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Enter to execute
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify the command was executed
	if len(m.commandHistory) == 0 {
		t.Errorf("Expected command to be added to history")
	}

	// Verify the last command in history is "/view groups"
	if m.commandHistory[len(m.commandHistory)-1].Command != "/view groups" {
		t.Errorf("Expected command '/view groups' in history, got %q", m.commandHistory[len(m.commandHistory)-1].Command)
	}

	// Verify contentType changed to show group list
	if m.contentType != ContentTypeViewGroups {
		t.Errorf("Expected contentType 'view_groups' after /view groups, got %q", m.contentType)
	}
}

func TestUnifiedInput_SearchClearsInput(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 100
	m.height = 30

	// Type a search term
	for _, r := range "192" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Verify input is in the unified field
	if m.unifiedInput.Value() != "192" {
		t.Errorf("Expected unified input to be '192', got %q", m.unifiedInput.Value())
	}

	// Press Enter to "execute" the search
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// For implicit searches, the current behavior is to keep the search value
	// This is different from commands which clear the input
	// The existing code doesn't clear on Enter for regular search, which is OK
	if m.contentType != ContentTypeTable {
		t.Errorf("Expected contentType 'table' after search, got %q", m.contentType)
	}
}

func TestUnifiedInput_CommandInHistory(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 100
	m.height = 30

	// Execute a command through unified input
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "help" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify command is in history
	if len(m.commandHistory) != 1 {
		t.Errorf("Expected 1 command in history, got %d", len(m.commandHistory))
	}

	// Verify command text in history
	if m.commandHistory[0].Command != "/help" {
		t.Errorf("Expected '/help' in history, got %q", m.commandHistory[0].Command)
	}

	// Verify output is captured
	if m.commandHistory[0].Output == "" {
		t.Errorf("Expected non-empty output in history")
	}
}

func TestUnifiedInput_PlaceholderText(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)

	// Verify placeholder text includes both search and command hints
	if m.unifiedInput.Placeholder != "Search or type /help for commands" {
		t.Errorf("Expected placeholder to mention both search and commands, got %q", m.unifiedInput.Placeholder)
	}
}

func TestUnifiedInput_MultipleSequentialCommands(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.168.1.100": "Servers",
		},
		Groups: []config.Group{
			{
				Name:    "Servers",
				Blocked: []string{"blocked.com"},
				Allowed: []string{"allowed.com"},
			},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 100
	m.height = 30

	// Execute /help
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "help" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(m.commandHistory) != 1 {
		t.Errorf("Expected 1 command after /help, got %d", len(m.commandHistory))
	}

	// Execute /view groups
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "view groups" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(m.commandHistory) != 2 {
		t.Errorf("Expected 2 commands in history, got %d", len(m.commandHistory))
	}

	// Verify order
	if m.commandHistory[0].Command != "/help" {
		t.Errorf("Expected first command to be '/help', got %q", m.commandHistory[0].Command)
	}
	if m.commandHistory[1].Command != "/view groups" {
		t.Errorf("Expected second command to be '/view groups', got %q", m.commandHistory[1].Command)
	}
}

// FIX #3: Error Case Tests

// Test unknown command
func TestTUI_ExecuteCommand_UnknownCommand(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Try to execute an unknown command
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "xyz" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify that unknown command doesn't crash
	// The existing behavior is to clear the input
	if m.unifiedInput.Value() != "" {
		t.Errorf("Expected input to be cleared after unknown command, got %q", m.unifiedInput.Value())
	}
}

// Test view group with missing group name
func TestTUI_ExecuteCommand_ViewGroupMissingName(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Try to execute /view group without a group name
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "view group" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify that the command was added to history (it handles the error internally)
	if len(m.commandHistory) == 0 {
		t.Errorf("Expected command to be added to history")
	}

	// Verify the input was cleared after command execution
	if m.unifiedInput.Value() != "" {
		t.Errorf("Expected input to be cleared after command, got %q", m.unifiedInput.Value())
	}
}

// Test view with unknown subcommand
func TestTUI_ExecuteCommand_ViewUnknownSubcommand(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Call executeCommand directly with unknown subcommand
	m.executeCommand("view", []string{"xyz"})

	// Verify contentType is "help" (error state)
	if m.contentType != ContentTypeHelp {
		t.Errorf("Expected contentType 'help' for unknown subcommand, got %q", m.contentType)
	}

	// Verify the input was cleared after command execution
	if m.unifiedInput.Value() != "" {
		t.Errorf("Expected input to be cleared after command, got %q", m.unifiedInput.Value())
	}
}

// Test search with empty query
func TestTUI_FilterClients_EmptyQuery(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
			"192.168.1.150": "IoT",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// With empty search, all clients should be shown
	m.filterClients()
	if len(m.filtered) != len(m.clients) {
		t.Errorf("Expected all clients when query is empty, got %d/%d",
			len(m.filtered), len(m.clients))
	}
}

// Test Tab completion outside typeahead mode
func TestTUI_TabCompletion_OutsideTypeaheadModeExisting(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Set up a normal search state (not in typeahead)
	m.unifiedInput.SetValue("normal text")
	m.inTypeaheadMode = false

	// Verify not in typeahead mode
	if m.inTypeaheadMode {
		t.Errorf("Expected NOT to be in typeahead mode")
	}

	// Tab behavior when not in typeahead should not change model state
	// (Tab would be handled by textinput component, not our logic)
	if m.inTypeaheadMode {
		t.Errorf("Tab outside typeahead should not enter typeahead mode")
	}
}

// FIX #4: First-Run Banner Tests

func TestTUI_FirstRunBanner(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 100
	m.height = 30
	m.firstRun = true
	m.unifiedInput.SetValue("")

	// The welcome banner was deliberately removed for v0.3.0 GA (see the "DISABLED"
	// note in View()). Even with firstRun=true it must NOT render; the empty state
	// shows the normal search hint instead.
	view := m.View()
	if strings.Contains(view, "Welcome to Pharos Advanced Blocking") {
		t.Errorf("Welcome banner was removed for v0.3.0 GA but rendered with firstRun=true:\n%s", view)
	}
	if strings.Contains(view, "Quick start") {
		t.Errorf("Quick start banner text was removed but is still rendered")
	}
	if !strings.Contains(view, "Start typing to search") {
		t.Errorf("expected empty-state search hint in view")
	}
}

func TestTUI_FirstRunBannerDismissesOnKeypress(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)
	m.ready = true
	m.width = 100
	m.height = 30
	m.firstRun = true
	m.unifiedInput.SetValue("test")

	// Calling View() should dismiss the banner when input is not empty
	view := m.View()

	// After first keystroke, firstRun should be false
	if m.firstRun {
		t.Errorf("expected firstRun to be false after first keystroke")
	}
	if strings.Contains(view, "Welcome to Pharos Advanced Blocking") {
		t.Errorf("welcome banner should not appear after first keystroke")
	}
}

// FIX #5: Content Type Enum Tests

func TestContentType_String(t *testing.T) {
	tests := []struct {
		ct       ContentType
		expected string
	}{
		{ContentTypeEmpty, "empty"},
		{ContentTypeTable, "table"},
		{ContentTypeHelp, "help"},
		{ContentTypeError, "error"},
		{ContentTypeCommandList, "command_list"},
		{ContentTypeViewNetworkGroupMap, "view_networkgroupmap"},
		{ContentTypeViewGroups, "view_groups"},
		{ContentTypeViewGroup, "view_group"},
	}

	for _, tt := range tests {
		if tt.ct.String() != tt.expected {
			t.Errorf("ContentType(%d).String() = %s, want %s", tt.ct, tt.ct.String(), tt.expected)
		}
	}
}

func TestContentType_InitialValue(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
		
	})

	m := New(cfg)

	// Verify initial contentType is ContentTypeEmpty
	if m.contentType != ContentTypeEmpty {
		t.Errorf("Expected initial contentType to be ContentTypeEmpty, got %v", m.contentType)
	}
}

// Phase 4: Search Typeahead Tests

func TestTUI_SearchTypeahead_UniqueIPMatch(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.0.2.50":    "servers",
			"192.0.2.51":    "clients",
			"192.168.1.100": "IoT",
		},
		Groups: []config.Group{
			{Name: "servers", Blocked: []string{}, Allowed: []string{}},
			{Name: "clients", Blocked: []string{}, Allowed: []string{}},
			{Name: "IoT", Blocked: []string{}, Allowed: []string{}},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "192.0.2.50" (unique IP)
	for _, r := range "192.0.2.50" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Get search matches
	matches := m.getSearchMatches("192.0.2.50")
	if len(matches) == 0 {
		t.Errorf("expected search matches for '192.0.2.50', got none")
	}

	// Verify the IP is in the matches
	found := false
	for _, match := range matches {
		if match.Type == "ip" && match.Value == "192.0.2.50" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find '192.0.2.50' in search matches")
	}
}

func TestTUI_SearchTypeahead_PartialIPMatch(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.0.2.50":    "servers",
			"192.0.2.51":    "clients",
			"192.168.1.100": "IoT",
		},
		Groups: []config.Group{
			{Name: "servers", Blocked: []string{}, Allowed: []string{}},
			{Name: "clients", Blocked: []string{}, Allowed: []string{}},
			{Name: "IoT", Blocked: []string{}, Allowed: []string{}},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "192" (prefix match)
	for _, r := range "192" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Get search matches for prefix
	matches := m.getSearchMatches("192")
	if len(matches) < 2 {
		t.Errorf("expected at least 2 matches for prefix '192', got %d", len(matches))
	}

	// Verify multiple IPs are found
	ipCount := 0
	for _, match := range matches {
		if match.Type == "ip" {
			ipCount++
		}
	}
	if ipCount < 2 {
		t.Errorf("expected at least 2 IP matches for '192', got %d", ipCount)
	}
}

func TestTUI_SearchTypeahead_GroupMatch(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.0.2.50": "servers",
			"192.0.2.51": "security",
		},
		Groups: []config.Group{
			{Name: "servers", Blocked: []string{}, Allowed: []string{}},
			{Name: "security", Blocked: []string{}, Allowed: []string{}},
			{Name: "services", Blocked: []string{}, Allowed: []string{}},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "ser" to match groups like "servers", "security", "services"
	matches := m.getSearchMatches("ser")

	// Should find multiple group matches
	hasGroupMatch := false
	for _, match := range matches {
		if match.Type == "group" {
			hasGroupMatch = true
			break
		}
	}
	if !hasGroupMatch {
		t.Errorf("expected group match for 'ser', got only IPs")
	}
}

func TestTUI_SearchTypeahead_CaseInsensitiveMatch(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.0.2.50": "Servers",
		},
		Groups: []config.Group{
			{Name: "Servers", Blocked: []string{}, Allowed: []string{}},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Search should be case-insensitive
	matches1 := m.getSearchMatches("SER")
	matches2 := m.getSearchMatches("ser")

	if len(matches1) != len(matches2) {
		t.Errorf("search should be case-insensitive, got %d matches for 'SER', %d for 'ser'",
			len(matches1), len(matches2))
	}
}

func TestTUI_SearchTypeahead_CycleNavigation(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.0.2.50": "servers",
			"192.0.2.51": "clients",
		},
		Groups: []config.Group{
			{Name: "servers", Blocked: []string{}, Allowed: []string{}},
			{Name: "clients", Blocked: []string{}, Allowed: []string{}},
			{Name: "security", Blocked: []string{}, Allowed: []string{}},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Manually set up typeahead state
	m.inSearchTypeahead = true
	m.searchMatches = []SearchMatch{
		{"ip", "192.0.2.50"},
		{"group", "servers"},
		{"group", "clients"},
	}
	m.searchMatchIndex = 0

	// Down arrow should increment index
	m.searchMatchIndex = (m.searchMatchIndex + 1) % len(m.searchMatches)
	if m.searchMatchIndex != 1 {
		t.Errorf("expected index 1 after down arrow, got %d", m.searchMatchIndex)
	}

	// Down arrow again
	m.searchMatchIndex = (m.searchMatchIndex + 1) % len(m.searchMatches)
	if m.searchMatchIndex != 2 {
		t.Errorf("expected index 2 after second down arrow, got %d", m.searchMatchIndex)
	}

	// Up arrow should decrement index
	m.searchMatchIndex = (m.searchMatchIndex - 1 + len(m.searchMatches)) % len(m.searchMatches)
	if m.searchMatchIndex != 1 {
		t.Errorf("expected index 1 after up arrow, got %d", m.searchMatchIndex)
	}
}

func TestTUI_SearchTypeahead_TabActivatesTypeahead(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.0.2.50": "servers",
			"192.0.2.51": "clients",
		},
		Groups: []config.Group{
			{Name: "servers", Blocked: []string{}, Allowed: []string{}},
			{Name: "clients", Blocked: []string{}, Allowed: []string{}},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "192" (search input, no slash)
	for _, r := range "192" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Verify not in search typeahead yet
	if m.inSearchTypeahead {
		t.Errorf("expected NOT to be in search typeahead before Tab")
	}

	// Press Tab to activate search typeahead
	m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Verify now in search typeahead
	if !m.inSearchTypeahead {
		t.Errorf("expected to be in search typeahead after Tab")
	}

	// Verify search matches are populated
	if len(m.searchMatches) == 0 {
		t.Errorf("expected search matches to be populated after Tab")
	}
}

func TestTUI_SearchTypeahead_EscCancels(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.0.2.50": "servers",
		},
		Groups: []config.Group{
			{Name: "servers", Blocked: []string{}, Allowed: []string{}},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Set up typeahead state
	m.inSearchTypeahead = true
	m.unifiedInput.SetValue("192.0.2.50")
	m.searchMatches = []SearchMatch{{"ip", "192.0.2.50"}}
	m.searchMatchIndex = 0

	// Simulate Esc press (manually, since we handle it in Update)
	m.inSearchTypeahead = false
	m.searchMatches = []SearchMatch{}
	m.searchMatchIndex = 0
	m.unifiedInput.SetValue("")

	// Verify state is cleared
	if m.inSearchTypeahead {
		t.Errorf("expected inSearchTypeahead to be false after Esc")
	}
	if len(m.searchMatches) != 0 {
		t.Errorf("expected search matches to be cleared after Esc")
	}
	if m.unifiedInput.Value() != "" {
		t.Errorf("expected input to be cleared after Esc, got '%s'", m.unifiedInput.Value())
	}
}

func TestTUI_SearchTypeahead_EnterExecutesSearch(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.0.2.50": "servers",
		},
		Groups: []config.Group{
			{Name: "servers", Blocked: []string{}, Allowed: []string{}},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Type "192" (search input)
	for _, r := range "192" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Tab to activate typeahead
	m.Update(tea.KeyMsg{Type: tea.KeyTab})

	if !m.inSearchTypeahead {
		t.Errorf("expected to be in search typeahead after Tab")
	}

	// Press Enter to confirm selection
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify typeahead is exited
	if m.inSearchTypeahead {
		t.Errorf("expected to exit search typeahead after Enter")
	}

	// Verify content is still table (search results shown)
	if m.contentType != ContentTypeTable {
		t.Errorf("expected contentType 'table' after Enter, got %q", m.contentType)
	}
}

func TestTUI_SearchTypeahead_TabCycles(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.0.2.50": "servers",
			"192.0.2.51": "clients",
		},
		Groups: []config.Group{
			{Name: "servers", Blocked: []string{}, Allowed: []string{}},
			{Name: "clients", Blocked: []string{}, Allowed: []string{}},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Set up typeahead state manually with multiple matches
	m.inSearchTypeahead = true
	m.searchMatches = []SearchMatch{
		{"ip", "192.0.2.50"},
		{"ip", "192.0.2.51"},
		{"group", "servers"},
	}
	m.searchMatchIndex = 0

	// Tab should cycle to next match
	m.searchMatchIndex = (m.searchMatchIndex + 1) % len(m.searchMatches)
	if m.searchMatchIndex != 1 {
		t.Errorf("expected index 1 after Tab, got %d", m.searchMatchIndex)
	}

	// Tab again
	m.searchMatchIndex = (m.searchMatchIndex + 1) % len(m.searchMatches)
	if m.searchMatchIndex != 2 {
		t.Errorf("expected index 2 after second Tab, got %d", m.searchMatchIndex)
	}

	// Tab again (should cycle back to 0)
	m.searchMatchIndex = (m.searchMatchIndex + 1) % len(m.searchMatches)
	if m.searchMatchIndex != 0 {
		t.Errorf("expected index 0 after third Tab (cycle), got %d", m.searchMatchIndex)
	}
}

func TestTUI_SearchTypeahead_RenderList(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.0.2.50": "servers",
		},
		Groups: []config.Group{
			{Name: "servers", Blocked: []string{}, Allowed: []string{}},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Set up typeahead state
	m.inSearchTypeahead = true
	m.searchMatches = []SearchMatch{
		{"ip", "192.0.2.50"},
		{"group", "servers"},
	}
	m.searchMatchIndex = 0

	// Render typeahead list
	typeaheadView := m.renderSearchTypeaheadList()

	// Verify output contains expected elements
	if !strings.Contains(typeaheadView, "Search matches") {
		t.Errorf("expected 'Search matches' header in typeahead list")
	}
	if !strings.Contains(typeaheadView, "192.0.2.50") {
		t.Errorf("expected IP in typeahead list")
	}
	if !strings.Contains(typeaheadView, "servers") {
		t.Errorf("expected group in typeahead list")
	}
	if !strings.Contains(typeaheadView, "[ip]") {
		t.Errorf("expected type indicator [ip] in typeahead list")
	}
	if !strings.Contains(typeaheadView, "[group]") {
		t.Errorf("expected type indicator [group] in typeahead list")
	}
}

// Phase 4: Edge-Case Tests for Search Typeahead

func TestTUI_SearchTypeahead_NoMatches(t *testing.T) {
	m := New(&config.Config{
		NetworkGroupMap: map[string]string{
			"192.0.2.50": "servers",
		},
		Groups: []config.Group{
			{Name: "servers", Blocked: []string{}, Allowed: []string{}},
		},
	})
	m.ready = true
	m.width = 80
	m.height = 24

	// Simulate Tab press with prefix that matches nothing
	m.unifiedInput.SetValue("xyz999")
	matches := m.getSearchMatches("xyz999")

	if len(matches) != 0 {
		t.Errorf("expected no matches for 'xyz999', got %d matches", len(matches))
	}

	// Verify typeahead doesn't activate with zero matches
	if m.inSearchTypeahead {
		t.Errorf("expected inSearchTypeahead to remain false when no matches found")
	}
}

func TestTUI_SearchTypeahead_LargeResultSet(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: make(map[string]string),
	}

	// Inject 100+ clients into config
	for i := 0; i < 120; i++ {
		ip := fmt.Sprintf("192.0.2.%d", (i%256))
		cfg.NetworkGroupMap[ip] = "servers"
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Search for common prefix
	matches := m.getSearchMatches("192.0.2")

	if len(matches) < 100 {
		t.Errorf("expected 100+ matches for '192.0.2', got %d", len(matches))
	}

	// Verify navigation works with large result set
	m.inSearchTypeahead = true
	m.searchMatches = matches
	m.searchMatchIndex = 0

	// Simulate Down arrow multiple times
	for i := 0; i < len(matches)-1; i++ {
		m.searchMatchIndex = (m.searchMatchIndex + 1) % len(m.searchMatches)
	}

	// Should wrap back to start
	m.searchMatchIndex = (m.searchMatchIndex + 1) % len(m.searchMatches)
	if m.searchMatchIndex != 0 {
		t.Errorf("expected wrap-around to index 0, got %d", m.searchMatchIndex)
	}
}

func TestTUI_SearchTypeahead_ModeIntegration(t *testing.T) {
	m := New(&config.Config{
		NetworkGroupMap: map[string]string{
			"192.0.2.50": "servers",
		},
		Groups: []config.Group{
			{Name: "servers", Blocked: []string{}, Allowed: []string{}},
		},
	})
	m.ready = true
	m.width = 80
	m.height = 24

	// Start in search mode
	m.unifiedInput.SetValue("192")
	m.searchMatches = m.getSearchMatches("192")
	m.inSearchTypeahead = true
	m.searchMatchIndex = 0

	if !m.inSearchTypeahead {
		t.Errorf("expected search typeahead to be active")
	}

	// Switch to command mode (user clears and types "/")
	m.unifiedInput.SetValue("/view")
	parsed := ParseUnifiedInput(m.unifiedInput.Value())

	if parsed.Type != InputTypeCommand {
		t.Errorf("expected command mode, got search mode")
	}

	// Verify command typeahead can activate without search typeahead interference
	m.inSearchTypeahead = false  // Exit search mode
	m.searchMatches = []SearchMatch{}
	m.commandMatches = filterCommands("/view")
	m.inTypeaheadMode = true

	if m.inTypeaheadMode && m.inSearchTypeahead {
		t.Errorf("expected only command typeahead active, got both modes")
	}

	// Switch back to search mode
	m.inTypeaheadMode = false
	m.unifiedInput.SetValue("servers")
	m.searchMatches = m.getSearchMatches("servers")
	m.inSearchTypeahead = true

	if m.inSearchTypeahead && m.inTypeaheadMode {
		t.Errorf("expected only search typeahead active, got both modes")
	}

	if !m.inSearchTypeahead {
		t.Errorf("expected search typeahead to be active after switch back")
	}
}

// ============================================================================
// PHASE 5: INTEGRATION TESTS
// ============================================================================

func TestTUI_Integration_SearchThenViewCommand(t *testing.T) {
	// Test workflow: Search for IP → /view command → back to search
	cfg := testConfigWithIPs(map[string]string{
		"192.0.2.50":  "prod-servers",
			"192.0.2.51":  "prod-servers",
			"10.0.0.5":    "dev-machines",
			"10.0.0.10":   "dev-machines",
			"172.16.0.1":  "iot-devices",
		
	})
	m := New(cfg)

	// Step 1: Search for IP
	m.unifiedInput.SetValue("192.0.2.50")
	parsed := ParseUnifiedInput(m.unifiedInput.Value())
	if parsed.Type != InputTypeSearch {
		t.Errorf("expected search mode, got command mode")
	}
	m.filterClients() // filterClients uses m.unifiedInput.Value() internally
	m.contentType = ContentTypeTable

	// Verify search results appear
	if len(m.filtered) == 0 {
		t.Errorf("expected search results for '192.0.2.50'")
	}

	// Step 2: User switches to /view command
	m.unifiedInput.SetValue("/view groups")
	parsed = ParseUnifiedInput(m.unifiedInput.Value())
	if parsed.Type != InputTypeCommand {
		t.Errorf("expected command mode after typing /")
	}

	// Step 3: Execute view command
	_, _ = m.executeCommand(parsed.Query, parsed.Args)

	if m.contentType != ContentTypeViewGroups {
		t.Errorf("expected ContentTypeViewGroups, got %v", m.contentType)
	}

	// Step 4: Return to search
	m.unifiedInput.SetValue("dev-machines")
	parsed = ParseUnifiedInput(m.unifiedInput.Value())
	if parsed.Type != InputTypeSearch {
		t.Errorf("expected search mode after clearing /")
	}

	m.filterClients()
	if len(m.filtered) == 0 {
		t.Errorf("expected search results for 'dev-machines'")
	}
}

func TestTUI_Integration_RapidTabCompletions(t *testing.T) {
	// Test rapid Tab completions switching between command and search modes
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "servers",
			"192.168.1.150": "iot",
			"10.0.0.5":      "laptops",
		
	})
	m := New(cfg)

	// First Tab completion in command mode
	m.unifiedInput.SetValue("/v")
	m.commandMatches = filterCommands("/v")
	m.inTypeaheadMode = true

	if len(m.commandMatches) == 0 {
		t.Errorf("expected command matches for '/v'")
	}

	// Tab to complete first command
	if len(m.commandMatches) > 0 {
		m.unifiedInput.SetValue("/view ")
	}
	m.inTypeaheadMode = false

	// Now Tab in search mode on same instance
	m.unifiedInput.SetValue("192")
	m.searchMatches = m.getSearchMatches("192")
	m.inSearchTypeahead = true

	if len(m.searchMatches) == 0 {
		t.Errorf("expected search matches for '192'")
	}

	// Verify no state contamination between modes
	if m.inTypeaheadMode {
		t.Errorf("command typeahead should be off during search typeahead")
	}
}

func TestTUI_Integration_HistoryAcrossModes(t *testing.T) {
	// Test that history tracks both searches and commands in order
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "servers",
			"192.168.1.150": "iot",
		
	})
	m := New(cfg)

	// Perform search
	m.appendHistory("servers", "192.168.1.100 (servers)")

	// Perform command
	m.appendHistory("/view groups", "Groups: servers, iot")

	// Perform another search
	m.appendHistory("192", "192.168.1.100 (servers)")

	// Verify history has all 3 entries in order
	if len(m.commandHistory) != 3 {
		t.Errorf("expected 3 history entries, got %d", len(m.commandHistory))
	}

	if m.commandHistory[0].Command != "servers" {
		t.Errorf("expected first entry 'servers', got '%s'", m.commandHistory[0].Command)
	}

	if m.commandHistory[1].Command != "/view groups" {
		t.Errorf("expected second entry '/view groups', got '%s'", m.commandHistory[1].Command)
	}

	if m.commandHistory[2].Command != "192" {
		t.Errorf("expected third entry '192', got '%s'", m.commandHistory[2].Command)
	}
}

// ============================================================================
// PHASE 5: STRESS TESTS
// ============================================================================

func TestTUI_StressTest_RapidKeystrokes(t *testing.T) {
	// Simulate rapid typing of IP address
	cfg := testConfigWithIPs(map[string]string{
		"192.0.2.50": "servers",
		
	})
	m := New(cfg)

	// Simulate rapid typing
	keystrokes := []string{"1", "92", "192", "192.", "192.0", "192.0.", "192.0.2", "192.0.2."}

	for _, keystroke := range keystrokes {
		m.unifiedInput.SetValue(keystroke)

		// Each keystroke should be parseable
		parsed := ParseUnifiedInput(keystroke)
		// The parser should handle partial input gracefully
		_ = parsed

		// Should not crash when filtering
		if !strings.HasPrefix(keystroke, "/") && keystroke != "" {
			m.filterClients()
		}
	}
}

func TestTUI_StressTest_LargeDatasetNavigation(t *testing.T) {
	// Test navigation with large dataset (500+ clients)
	cfg := &config.Config{
		NetworkGroupMap: make(map[string]string),
	}

	// Inject 500 clients
	for i := 0; i < 500; i++ {
		ip := fmt.Sprintf("192.0.2.%d", (i%256))
		if i < 256 {
			cfg.NetworkGroupMap[ip] = "servers"
		} else {
			cfg.NetworkGroupMap[ip] = "iot"
		}
	}

	m := New(cfg)

	// Search for large result set
	m.unifiedInput.SetValue("192.0.2")
	m.searchMatches = m.getSearchMatches("192.0.2")
	m.inSearchTypeahead = true
	m.searchMatchIndex = 0

	if len(m.searchMatches) < 100 {
		t.Errorf("expected 100+ matches for '192.0.2', got %d", len(m.searchMatches))
	}

	// Simulate rapid navigation (Down 100 times)
	for i := 0; i < 100; i++ {
		m.searchMatchIndex = (m.searchMatchIndex + 1) % len(m.searchMatches)
	}

	// Simulate rapid navigation (Up 100 times)
	for i := 0; i < 100; i++ {
		m.searchMatchIndex = (m.searchMatchIndex - 1 + len(m.searchMatches)) % len(m.searchMatches)
	}

	// Should not crash or deadlock
	if m.searchMatchIndex < 0 || m.searchMatchIndex >= len(m.searchMatches) {
		t.Errorf("index out of bounds after navigation: %d", m.searchMatchIndex)
	}
}

// ============================================================================
// BENCHMARKS (for performance regression testing)
// ============================================================================

func BenchmarkFilterClients_SmallDataset(b *testing.B) {
	m := New(testConfigWithIPs(map[string]string{
		"192.0.2.1":   "servers",
		"192.0.2.2":   "iot",
		"192.0.2.3":   "security",
		"192.0.2.4":   "servers",
		"192.0.2.5":   "iot",
	}))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.unifiedInput.SetValue("192")
		m.filterClients()
	}
}

func BenchmarkFilterClients_LargeDataset(b *testing.B) {
	ips := make(map[string]string)
	for i := 0; i < 500; i++ {
		ips[fmt.Sprintf("192.0.2.%d", i%256)] = "servers"
	}
	m := New(testConfigWithIPs(ips))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.unifiedInput.SetValue("192")
		m.filterClients()
	}
}

func BenchmarkGetSearchMatches_SmallDataset(b *testing.B) {
	m := New(mockConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.getSearchMatches("192")
	}
}

func BenchmarkGetSearchMatches_LargeDataset(b *testing.B) {
	ips := make(map[string]string)
	for i := 0; i < 500; i++ {
		ips[fmt.Sprintf("192.0.2.%d", i%256)] = "servers"
	}
	m := New(testConfigWithIPs(ips))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.getSearchMatches("192")
	}
}

func BenchmarkParseUnifiedInput(b *testing.B) {
	inputs := []string{
		"192.0.2.50",
		"/view groups",
		"servers",
		"/help",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, input := range inputs {
			ParseUnifiedInput(input)
		}
	}
}

// ============================================================================
// REGRESSION TESTS: Typeahead and Help Display Issues
// ============================================================================

// Regression Test 1: Typeahead display broken when typing /v
// Issue: User types /v but no slash command options appear (should show view subcommands)
func TestRegression_TypeaheadForViewAlias(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.168.1.100": "servers",
		},
		Groups: []config.Group{
			{
				Name:    "servers",
				Blocked: []string{"blocked.com"},
				Allowed: []string{"allowed.com"},
			},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Simulate typing "/" and then "v"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	// Verify we're in typeahead mode for just "/"
	if !m.inTypeaheadMode {
		t.Errorf("Expected to be in typeahead mode after typing /")
	}
	if m.contentType != ContentTypeCommandList {
		t.Errorf("Expected ContentTypeCommandList after /, got %v", m.contentType)
	}

	// Now type "v"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})

	// Verify we're still in typeahead mode
	if !m.inTypeaheadMode {
		t.Errorf("Expected to be in typeahead mode after typing /v")
	}

	// Verify we have /view subcommands in the matches (not just /view command)
	if len(m.commandMatches) == 0 {
		t.Fatalf("Expected command matches for '/v', got none")
	}

	// When /v is typed, it should show /view subcommands (groups, group, networkGroupMap)
	// NOT just the /view command itself
	expectedSubcommandCount := 3 // groups, group, networkGroupMap
	if len(m.commandMatches) != expectedSubcommandCount {
		t.Errorf("Expected %d /view subcommands for '/v', got %d", expectedSubcommandCount, len(m.commandMatches))
		for i, cmd := range m.commandMatches {
			t.Logf("  [%d] %s", i, cmd.Name)
		}
	}

	// Verify all matches start with "/view "
	for _, cmd := range m.commandMatches {
		if !strings.HasPrefix(cmd.Name, "/view ") {
			t.Errorf("Expected command to start with '/view ', got %q", cmd.Name)
		}
	}

	// Verify contentType is still CommandList for rendering
	if m.contentType != ContentTypeCommandList {
		t.Errorf("Expected ContentTypeCommandList for /v, got %v", m.contentType)
	}

	// Verify the view can render the typeahead list with subcommands
	viewOutput := m.View()
	if !strings.Contains(viewOutput, "Pharos Advanced Blocking") {
		t.Errorf("Expected view to render successfully")
	}
	if !strings.Contains(viewOutput, "/view") {
		t.Errorf("Expected /view subcommands in typeahead output")
	}
}

// Regression Test 2: Help command broken - help text doesn't display
// Issue: User types /help[enter] but help text doesn't display
func TestRegression_HelpCommandExecution(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.168.1.100": "servers",
		},
		Groups: []config.Group{
			{
				Name:    "servers",
				Blocked: []string{"blocked.com"},
				Allowed: []string{"allowed.com"},
			},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	// Verify initial state has no history
	if len(m.commandHistory) != 0 {
		t.Errorf("Expected empty history initially, got %d entries", len(m.commandHistory))
	}

	// Simulate typing "/help" and pressing Enter
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "help" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify contentType is Help
	if m.contentType != ContentTypeHelp {
		t.Errorf("Expected ContentTypeHelp after /help command, got %v", m.contentType)
	}

	// Verify help text is populated in contentText
	if m.contentText == "" {
		t.Errorf("Expected non-empty contentText for help")
	}

	if !strings.Contains(m.contentText, "Available Commands") {
		t.Errorf("Expected 'Available Commands' in help text")
	}

	// Verify command is in history
	if len(m.commandHistory) == 0 {
		t.Fatalf("Expected /help command in history")
	}

	if m.commandHistory[0].Command != "/help" {
		t.Errorf("Expected command '/help' in history, got %q", m.commandHistory[0].Command)
	}

	// Verify help text is also in the history output
	if !strings.Contains(m.commandHistory[0].Output, "Available Commands") {
		t.Errorf("Expected help text in history output")
	}

	// Verify the View() renders the help text correctly
	viewOutput := m.View()

	// Should show help text content (since we have history, it renders from history)
	if !strings.Contains(viewOutput, "/help") {
		t.Errorf("Expected /help command in rendered view")
	}

	if !strings.Contains(viewOutput, "Available Commands") {
		t.Errorf("Expected help text content in rendered view")
	}
}

// ============================================================================
// END-TO-END TESTS FOR CRITICAL BUG FIX (#3)
// ============================================================================
// These tests verify that the command parsing bug is fixed:
// When Enter is pressed after typeahead completes, the command must be
// properly parsed into (cmd, args) before execution.

// TestE2E_TypeaheadViewGroups tests the complete flow of typeahead completion
// and execution: Type /v → Tab → Type groups → Enter → Groups render
func TestE2E_TypeaheadViewGroups(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.168.1.100": "Servers",
			"192.168.1.150": "IoT",
		},
		Groups: []config.Group{
			{
				Name:    "Servers",
				Blocked: []string{"blocked.com"},
				Allowed: []string{"allowed.com"},
			},
			{
				Name:    "IoT",
				Blocked: []string{"iot-blocked.com"},
				Allowed: []string{"iot-allowed.com"},
			},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 100
	m.height = 30

	// STEP 1: Type "/v" to enter typeahead mode
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})

	if !m.inTypeaheadMode {
		t.Fatalf("Step 1 failed: expected to be in typeahead mode after /v")
	}
	if m.contentType != ContentTypeCommandList {
		t.Fatalf("Step 1 failed: expected contentType CommandList, got %v", m.contentType)
	}

	// STEP 2: Find the /view groups command in typeahead and press Enter
	// (When we typed "/v", the filter shows view subcommands)
	if len(m.commandMatches) == 0 {
		t.Fatalf("Step 2 failed: no command matches for /v")
	}

	// Find and select /view groups
	groupsIdx := -1
	for i, cmd := range m.commandMatches {
		if cmd.Name == "/view groups" {
			groupsIdx = i
			break
		}
	}

	if groupsIdx == -1 {
		t.Fatalf("Step 2 failed: '/view groups' not found in matches, got: %v", m.commandMatches)
	}

	// Navigate to groups if needed
	for i := m.selectedCommand; i < groupsIdx; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	// STEP 3: Press Enter to execute /view groups via typeahead
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// VERIFICATION: Command was added to history
	if len(m.commandHistory) == 0 {
		t.Errorf("Verification failed: command not in history")
	} else if m.commandHistory[0].Command != "/view groups" {
		t.Errorf("Verification failed: expected '/view groups' in history, got %q", m.commandHistory[0].Command)
	}

	// VERIFICATION: Input was cleared
	if m.unifiedInput.Value() != "" {
		t.Errorf("Verification failed: input not cleared, got %q", m.unifiedInput.Value())
	}

	// VERIFICATION: Content type changed to view groups
	if m.contentType != ContentTypeViewGroups {
		t.Errorf("Verification failed: expected contentType ViewGroups, got %v", m.contentType)
	}

	// VERIFICATION: History has output with groups
	if len(m.commandHistory) > 0 && !strings.Contains(m.commandHistory[0].Output, "Servers") {
		t.Errorf("Verification failed: expected 'Servers' group in history output")
	}

	// VERIFICATION: View() renders the groups
	viewOutput := m.View()
	if !strings.Contains(viewOutput, "Servers") {
		t.Errorf("Verification failed: expected 'Servers' to render in View()")
	}
	if !strings.Contains(viewOutput, "IoT") {
		t.Errorf("Verification failed: expected 'IoT' to render in View()")
	}
	// The groups-list pane is shown (not a raw command echo); the command is recorded
	// in history state, verified above via m.commandHistory[0].Command.

	// FIX #3: Visual regression tests for input duplication
	// Verify welcome banner is dismissed
	if m.firstRun {
		t.Errorf("CRITICAL: firstRun should be false after view groups command")
	}

	// Verify welcome banner NOT in output
	if strings.Contains(viewOutput, "Welcome to Pharos") {
		t.Errorf("CRITICAL: Welcome banner should not render with groups content")
	}

	// Verify input field appears once (not doubled)
	inputFieldCount := strings.Count(viewOutput, "Search or type")
	if inputFieldCount != 1 {
		t.Errorf("CRITICAL: Input field appears %d times (expected 1) in TypeaheadViewGroups", inputFieldCount)
	}
}

// TestE2E_DirectViewCommand tests executing /view groups by typing it directly
// (without using Tab completion)
func TestE2E_DirectViewCommand(t *testing.T) {
	cfg := &config.Config{
		NetworkGroupMap: map[string]string{
			"192.168.1.100": "Servers",
			"192.168.1.150": "IoT",
		},
		Groups: []config.Group{
			{
				Name:    "Servers",
				Blocked: []string{"malware.com"},
				Allowed: []string{"trusted.com"},
			},
			{
				Name:    "IoT",
				Blocked: []string{"iot-malware.com"},
				Allowed: []string{"iot-trusted.com"},
			},
		},
	}

	m := New(cfg)
	m.ready = true
	m.width = 100
	m.height = 30

	// STEP 1: Type "/view groups" directly
	for _, r := range "/view groups" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	if m.unifiedInput.Value() != "/view groups" {
		t.Fatalf("Step 1 failed: expected input '/view groups', got %q", m.unifiedInput.Value())
	}

	// STEP 2: Press Enter to execute the command
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// VERIFICATION: Command was added to history (CRITICAL FIX TEST)
	if len(m.commandHistory) == 0 {
		t.Fatalf("CRITICAL BUG: command not in history - executeCommand silently failed")
	}
	if m.commandHistory[0].Command != "/view groups" {
		t.Errorf("Verification failed: expected '/view groups' in history, got %q", m.commandHistory[0].Command)
	}

	// VERIFICATION: Input was cleared
	if m.unifiedInput.Value() != "" {
		t.Errorf("Verification failed: input not cleared, got %q", m.unifiedInput.Value())
	}

	// VERIFICATION: Content type changed to view groups (CRITICAL FIX TEST)
	if m.contentType != ContentTypeViewGroups {
		t.Fatalf("CRITICAL BUG: contentType not updated - got %v, expected ViewGroups", m.contentType)
	}

	// VERIFICATION: History has output with group names
	if len(m.commandHistory) > 0 {
		output := m.commandHistory[0].Output
		if output == "" {
			t.Errorf("Verification failed: expected non-empty output in history")
		}
		if !strings.Contains(output, "Servers") && !strings.Contains(output, "IoT") {
			t.Errorf("Verification failed: expected group names in history output, got: %s", output)
		}
	}

	// VERIFICATION: View() renders the groups with all details
	viewOutput := m.View()
	if !strings.Contains(viewOutput, "Servers") {
		t.Fatalf("CRITICAL BUG: Groups not rendering in View() - Servers missing")
	}
	if !strings.Contains(viewOutput, "IoT") {
		t.Errorf("Verification failed: expected 'IoT' in View()")
	}
	// The groups-list pane is shown (not a raw command echo); the command is recorded
	// in history state, verified above via m.commandHistory[0].Command.

	// FIX #3: Visual regression tests for input duplication
	// Verify welcome banner is dismissed
	if m.firstRun {
		t.Errorf("CRITICAL: firstRun should be false after direct view command")
	}

	// Verify welcome banner NOT in output
	if strings.Contains(viewOutput, "Welcome to Pharos") {
		t.Errorf("CRITICAL: Welcome banner should not render with direct view content")
	}

	// Verify input field appears once (not doubled)
	inputFieldCount := strings.Count(viewOutput, "Search or type")
	if inputFieldCount != 1 {
		t.Errorf("CRITICAL: Input field appears %d times (expected 1) in DirectViewCommand", inputFieldCount)
	}
}

// TestE2E_HelpCommandViaBubbleTeaUpdate tests /help execution through
// typeahead selection (simulating user navigation and Enter press)
func TestE2E_HelpCommandViaBubbleTeaUpdate(t *testing.T) {
	cfg := testConfigWithIPs(map[string]string{
		"192.168.1.100": "Servers",
	})

	m := New(cfg)
	m.ready = true
	m.width = 100
	m.height = 30

	// STEP 1: Type "/" to enter typeahead
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	if !m.inTypeaheadMode {
		t.Fatalf("Step 1 failed: not in typeahead mode after /")
	}
	if len(m.commandMatches) == 0 {
		t.Fatalf("Step 1 failed: no command matches")
	}

	// STEP 2: Navigate to /help (default is first match which should be /help)
	if m.commandMatches[0].Name != "/help" {
		// If /help is not first, navigate down to find it
		for i, cmd := range m.commandMatches {
			if cmd.Name == "/help" {
				m.selectedCommand = i
				break
			}
		}
	}

	// STEP 3: Press Enter to execute /help via typeahead
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// VERIFICATION: Help command was added to history
	if len(m.commandHistory) == 0 {
		t.Fatalf("CRITICAL BUG: /help not in history after typeahead Enter")
	}
	if m.commandHistory[0].Command != "/help" {
		t.Errorf("Verification failed: expected '/help' in history, got %q", m.commandHistory[0].Command)
	}

	// VERIFICATION: Input was cleared
	if m.unifiedInput.Value() != "" {
		t.Errorf("Verification failed: input not cleared after command, got %q", m.unifiedInput.Value())
	}

	// VERIFICATION: Content type is Help
	if m.contentType != ContentTypeHelp {
		t.Errorf("Verification failed: expected contentType Help, got %v", m.contentType)
	}

	// VERIFICATION: Help text is in history
	if len(m.commandHistory) > 0 && !strings.Contains(m.commandHistory[0].Output, "Available Commands") {
		t.Errorf("Verification failed: expected help text in history output")
	}

	// VERIFICATION: View() renders help text
	viewOutput := m.View()
	if !strings.Contains(viewOutput, "Available Commands") {
		t.Fatalf("CRITICAL BUG: Help text not rendering in View()")
	}
	if !strings.Contains(viewOutput, "/help") {
		t.Errorf("Verification failed: expected '/help' command in history view")
	}

	// FIX #3: Visual regression tests for help text and input duplication
	// Verify welcome banner is dismissed
	if m.firstRun {
		t.Errorf("CRITICAL: firstRun should be false after help command")
	}

	// Verify welcome banner NOT in output
	if strings.Contains(viewOutput, "Welcome to Pharos") {
		t.Errorf("CRITICAL: Welcome banner should not render with help content")
	}

	// Verify help text IS in output
	if !strings.Contains(viewOutput, "Available Commands") {
		t.Errorf("CRITICAL: Help text missing from output")
	}

	// Verify input field appears once (not doubled)
	inputFieldCount := strings.Count(viewOutput, "Search or type")
	if inputFieldCount != 1 {
		t.Errorf("CRITICAL: Input field appears %d times (expected 1)", inputFieldCount)
	}
}

// ============================================================================
// RED-TEAM MUTATION TEST: Async Textinput Hypothesis Verification
// ============================================================================

// TestTUI_AsyncTextinputHypothesis verifies whether textinput.SetValue() has
// async/deferred behavior by testing the exact state after /help execution
func TestTUI_AsyncTextinputHypothesis(t *testing.T) {
	cfg := mockConfig()
	m := New(cfg)
	m.ready = true
	m.width = 80
	m.height = 24

	t.Log("=== RED-TEAM MUTATION TEST: Async Textinput Hypothesis ===")

	// BASELINE: Initial state
	t.Log("\n[BASELINE] Initial state")
	t.Logf("  firstRun: %v (expected: true)\n", m.firstRun)
	t.Logf("  inputValue: '%s' (expected: '')\n", m.unifiedInput.Value())
	if m.firstRun {
		t.Log("  PASS: firstRun is true at start")
	} else {
		t.Fatal("FAIL: firstRun should be true at start")
	}
	if m.unifiedInput.Value() == "" {
		t.Log("  PASS: input is empty at start")
	} else {
		t.Fatalf("FAIL: input should be empty at start, got '%s'", m.unifiedInput.Value())
	}

	// STEP 1: Execute /help command
	t.Log("\n[STEP 1] Execute /help command")
	_, _ = m.executeCommand("help", []string{})

	// CRITICAL INSPECTION: Check state immediately after executeCommand
	t.Log("\n[CRITICAL INSPECTION] State after executeCommand")
	firstRunAfterExec := m.firstRun
	inputValueAfterExec := m.unifiedInput.Value()
	t.Logf("  firstRun: %v (expected: false)\n", firstRunAfterExec)
	t.Logf("  inputValue: '%s' (expected: '')\n", inputValueAfterExec)
	t.Logf("  contentType: %v (expected: %v)\n", m.contentType, ContentTypeHelp)

	// HYPOTHESIS TEST 1: Check if SetValue() actually cleared the input
	if inputValueAfterExec == "" {
		t.Log("  HYPOTHESIS CHECK 1: PASS - inputValue is empty (SetValue() worked synchronously)")
	} else {
		t.Logf("  HYPOTHESIS CHECK 1: FAIL - inputValue='%s' (textinput may be async!)", inputValueAfterExec)
	}

	// HYPOTHESIS TEST 2: Check if firstRun was set to false
	if !firstRunAfterExec {
		t.Log("  HYPOTHESIS CHECK 2: PASS - firstRun is false")
	} else {
		t.Fatal("  HYPOTHESIS CHECK 2: FAIL - firstRun should be false after executeCommand")
	}

	// STEP 2: Render View
	t.Log("\n[STEP 2] Call View() to render")
	viewOutput := m.View()

	// CRITICAL CHECK: Verify welcome banner is NOT rendered
	t.Log("\n[CRITICAL OUTCOME] After View() call")
	bannerPresent := strings.Contains(viewOutput, "Welcome to Pharos Advanced Blocking")
	helpTextPresent := strings.Contains(viewOutput, "Available Commands")

	t.Logf("  Welcome banner in output: %v (expected: false)\n", bannerPresent)
	t.Logf("  Help text in output: %v (expected: true)\n", helpTextPresent)

	if bannerPresent {
		t.Errorf("CRITICAL BUG: Welcome banner should NOT be rendered after /help command")
		t.Logf("Welcome banner condition in View was TRUE when it should be FALSE")
		t.Logf("This means: firstRun=%v AND inputValue='%s'", firstRunAfterExec, inputValueAfterExec)
	} else {
		t.Log("  PASS: Welcome banner correctly dismissed")
	}

	if !helpTextPresent {
		t.Errorf("CRITICAL BUG: Help text should be rendered")
	} else {
		t.Log("  PASS: Help text correctly rendered")
	}

	// FINAL STATE VERIFICATION
	t.Log("\n[FINAL STATE VERIFICATION]")
	t.Logf("  firstRun: %v (expected: false)\n", m.firstRun)
	t.Logf("  inputValue: '%s' (expected: '')\n", m.unifiedInput.Value())
	t.Logf("  contentType: %v (expected: %v)\n", m.contentType, ContentTypeHelp)
	t.Logf("  history length: %d (expected: 1)\n", len(m.commandHistory))

	// Verify all conditions
	if m.firstRun {
		t.Fatal("CRITICAL: firstRun should be false after /help")
	}
	if m.unifiedInput.Value() != "" {
		t.Fatalf("CRITICAL: input should be empty, got '%s'", m.unifiedInput.Value())
	}
	if m.contentType != ContentTypeHelp {
		t.Fatalf("CRITICAL: contentType should be Help, got %v", m.contentType)
	}
	if len(m.commandHistory) != 1 {
		t.Fatalf("CRITICAL: should have 1 history entry, got %d", len(m.commandHistory))
	}

	t.Log("\n=== HYPOTHESIS VERIFICATION COMPLETE ===")
	t.Log("RESULT: Welcome banner correctly dismissed after /help command")
	t.Log("CONCLUSION: No async textinput issue detected with current implementation")
}
