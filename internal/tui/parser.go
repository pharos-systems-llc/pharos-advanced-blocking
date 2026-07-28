package tui

import (
	"strings"
)

// InputType represents the type of input the user entered
type InputType int

const (
	InputTypeEmpty InputType = iota
	InputTypeSearch
	InputTypeCommand
)

// ParseResult contains the parsed input
type ParseResult struct {
	Type  InputType
	Query string // For search: the search term; for command: the command name
	Args  []string
}

// ParseUnifiedInput parses the raw input string and determines if it's a search or command
func ParseUnifiedInput(rawInput string) ParseResult {
	trimmed := strings.TrimSpace(rawInput)

	if trimmed == "" {
		return ParseResult{Type: InputTypeEmpty, Query: "", Args: []string{}}
	}

	// If it starts with "/" it's a command
	if strings.HasPrefix(trimmed, "/") {
		return parseCommand(trimmed)
	}

	// Otherwise it's an implicit search
	return ParseResult{Type: InputTypeSearch, Query: trimmed, Args: []string{}}
}

// parseCommand parses a slash command (internal helper)
func parseCommand(input string) ParseResult {
	// Remove leading slash
	trimmed := strings.TrimPrefix(input, "/")
	parts := strings.Fields(trimmed)

	if len(parts) == 0 {
		// Just "/" with no command
		return ParseResult{Type: InputTypeCommand, Query: "", Args: []string{}}
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	return ParseResult{Type: InputTypeCommand, Query: cmd, Args: args}
}
