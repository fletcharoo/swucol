// Package tui provides the terminal user interface for swucol using bubbletea.
package tui

import (
	"fmt"
	"strings"
	"swucol/models"
	"swucol/store"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// searchBarStyle defines the styling for the search bar container.
var searchBarStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("62")).
	Padding(0, 1)

// resultStyle defines the styling for unselected search results (light grey).
var resultStyle = lipgloss.NewStyle().
	PaddingLeft(2).
	Foreground(lipgloss.Color("247"))

// selectedResultStyle defines the styling for the selected search result (white).
var selectedResultStyle = lipgloss.NewStyle().
	PaddingLeft(2).
	Foreground(lipgloss.Color("255"))

// Model represents the main TUI application state.
type Model struct {
	store          *store.Store
	searchInput    textinput.Model
	searchResults  []models.Card
	cursorPosition int // -1 = search bar, 0+ = result index
	width          int
	height         int
}

// New creates and returns a new TUI model with an initialized search bar.
func New(s *store.Store) Model {
	if s == nil {
		panic("store cannot be nil")
	}

	searchInput := textinput.New()
	searchInput.Placeholder = "Search cards..."
	searchInput.Focus()
	searchInput.CharLimit = 256
	searchInput.Width = 50

	return Model{
		store:          s,
		searchInput:    searchInput,
		searchResults:  []models.Card{},
		cursorPosition: -1, // Start in search bar
	}
}

// Init implements tea.Model and returns the initial command.
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements tea.Model and handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyDown:
			// Move cursor down into results
			if len(m.searchResults) > 0 && m.cursorPosition < len(m.searchResults)-1 {
				m.cursorPosition++
				if m.cursorPosition >= 0 {
					m.searchInput.Blur()
				}
			}
			return m, nil

		case tea.KeyUp:
			// Move cursor up, back to search bar if at top
			if m.cursorPosition > -1 {
				m.cursorPosition--
				if m.cursorPosition == -1 {
					m.searchInput.Focus()
				}
			}
			return m, nil

		case tea.KeyRunes:
			// Handle + and - keys for increment/decrement when cursor is on a result
			if m.cursorPosition >= 0 && m.cursorPosition < len(m.searchResults) {
				card := m.searchResults[m.cursorPosition]
				switch string(msg.Runes) {
				case "+", "=": // = is the unshifted + key
					_ = m.store.IncrementCardOwned(card.Name)
					m.performSearch() // Refresh results to show updated count
					return m, nil
				case "-", "_": // _ is the shifted - key
					_ = m.store.DecrementCardOwned(card.Name)
					m.performSearch() // Refresh results to show updated count
					return m, nil
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Update search input width to span most of the terminal width
		m.searchInput.Width = msg.Width - 6 // Account for border and padding
		if m.searchInput.Width < 20 {
			m.searchInput.Width = 20
		}
	}

	// Only process search input if cursor is in search bar
	if m.cursorPosition == -1 {
		// Store the previous value to detect changes
		previousValue := m.searchInput.Value()

		m.searchInput, cmd = m.searchInput.Update(msg)

		// If the search input value changed, perform a new search
		currentValue := m.searchInput.Value()
		if currentValue != previousValue {
			m.performSearch()
			// Reset cursor to search bar when results change
			m.cursorPosition = -1
		}
	}

	return m, cmd
}

// performSearch executes a fuzzy search based on the current input value.
func (m *Model) performSearch() {
	query := strings.TrimSpace(m.searchInput.Value())

	// If the search bar is empty, show no results
	if query == "" {
		m.searchResults = []models.Card{}
		return
	}

	// Perform fuzzy search using the store
	results, err := m.store.Search(query)
	if err != nil {
		// On error, clear results (could be improved with error display)
		m.searchResults = []models.Card{}
		return
	}

	m.searchResults = results
}

// View implements tea.Model and renders the TUI.
func (m Model) View() string {
	var builder strings.Builder

	// Render the search bar at the top
	searchBar := searchBarStyle.Width(m.width - 4).Render(m.searchInput.View())
	builder.WriteString(searchBar)
	builder.WriteString("\n")

	// Calculate how many results we can display
	// Search bar takes 3 lines (top border, content, bottom border) + 1 newline after
	searchBarHeight := 4
	availableLines := m.height - searchBarHeight
	if availableLines < 0 {
		availableLines = 0
	}

	// Render search results below the search bar, limited to available space
	resultsToShow := m.searchResults
	if len(resultsToShow) > availableLines {
		resultsToShow = resultsToShow[:availableLines]
	}

	// Build results section
	var resultsBuilder strings.Builder
	for i, card := range resultsToShow {
		resultLine := fmt.Sprintf("%d %-7s %s", card.Owned, card.Type, card.Name)

		// Use selected style if cursor is on this result
		if i == m.cursorPosition {
			resultsBuilder.WriteString(selectedResultStyle.Render(resultLine))
		} else {
			resultsBuilder.WriteString(resultStyle.Render(resultLine))
		}

		if i < len(resultsToShow)-1 {
			resultsBuilder.WriteString("\n")
		}
	}

	// Use lipgloss to set a fixed height for the results area to clear stale content
	resultsArea := lipgloss.NewStyle().
		Height(availableLines).
		Render(resultsBuilder.String())

	builder.WriteString(resultsArea)

	return builder.String()
}

// Run starts the TUI application with the provided store and returns any error that occurs.
func Run(s *store.Store) error {
	if s == nil {
		return fmt.Errorf("store cannot be nil")
	}

	program := tea.NewProgram(New(s), tea.WithAltScreen())
	_, err := program.Run()
	return err
}
