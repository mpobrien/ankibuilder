package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/goccy/go-yaml"
)

const pageSize = 5

type Phrase struct {
	Source string // Spanish sentence (plain, no ** markers)
	Target string // English translation
}

type chatModel struct {
	textInput       textinput.Model
	spinner         spinner.Model
	lastTranslation *Translation
	lastWord        string
	lastPhrases     []Phrase
	shownEntries    int
	wr              *WordReference
	width           int
	busy            bool
	busyMsg         string
}

func newChatModel(wr *WordReference) chatModel {
	ti := textinput.New()
	ti.Placeholder = "Enter a word or /command (/help for list)"
	ti.Prompt = "❯ "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#fad07a"))
	ti.CharLimit = 256
	ti.Focus()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#fad07a"))

	return chatModel{
		textInput: ti,
		spinner:   s,
		wr:        wr,
	}
}

func (m *chatModel) setWidth(width int) {
	m.width = width
	m.textInput.Width = width - 4
}

func (m *chatModel) setBusy(busy bool, msg ...string) tea.Cmd {
	m.busy = busy
	if busy {
		m.textInput.Blur()
		if len(msg) > 0 {
			m.busyMsg = msg[0]
		} else {
			m.busyMsg = "Working"
		}
		return m.spinner.Tick
	}
	m.busyMsg = ""
	m.textInput.Focus()
	return nil
}

func (m chatModel) update(msg tea.Msg) (chatModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.busy {
				return m, nil
			}
			input := strings.TrimSpace(m.textInput.Value())
			if input == "" {
				return m, nil
			}
			m.textInput.Reset()
			cmds = append(cmds, tea.Println(echoStyle.Render("> "+input)))

			if strings.HasPrefix(input, "/") {
				cmds = append(cmds, m.handleCommand(input)...)
				return m, tea.Batch(cmds...)
			}

			// Word lookup
			cmds = append(cmds, m.setBusy(true, "Looking up"))
			cmds = append(cmds, translateCmd(m.wr, input))
			return m, tea.Batch(cmds...)
		}

	case translateResultMsg:
		m.setBusy(false)
		if msg.err != nil {
			return m, tea.Println(errStyle.Render("Error: " + msg.err.Error()))
		}
		m.lastTranslation = msg.translation
		m.lastWord = msg.word
		m.shownEntries = 0
		entries := flattenEntries(msg.translation)
		end := min(pageSize, len(entries))
		m.shownEntries = end
		output := renderEntries(msg.word, entries, 0, end)
		if end < len(entries) {
			output += "\n" + dimStyle.Render(fmt.Sprintf("  Showing %d of %d — /more for next page, /all for everything", end, len(entries)))
		}
		return m, tea.Println(output)

	case listDecksResultMsg:
		m.setBusy(false)
		if msg.err != nil {
			return m, tea.Println(errStyle.Render("Error: " + msg.err.Error()))
		}
		return m, tea.Println(renderDecks(msg.decks))

	case listTemplatesResultMsg:
		m.setBusy(false)
		if msg.err != nil {
			return m, tea.Println(errStyle.Render("Error: " + msg.err.Error()))
		}
		return m, tea.Println(renderTemplates(msg.templates))

	case editorFinishedMsg:
		if msg.err != nil {
			m.setBusy(false)
			return m, tea.Println(errStyle.Render("Editor error: " + msg.err.Error()))
		}
		cmds = append(cmds, m.setBusy(true, "Creating cards"))
		cmds = append(cmds, createCardsCmd(msg.content))
		return m, tea.Batch(cmds...)

	case addCardResultMsg:
		m.setBusy(false)
		if msg.err != nil {
			return m, tea.Println(errStyle.Render("Error: " + msg.err.Error()))
		}
		return m, tea.Println(successStyle.Render(fmt.Sprintf("Successfully created %d card(s).", msg.count)))

	case phrasesResultMsg:
		m.setBusy(false)
		if msg.err != nil {
			return m, tea.Println(errStyle.Render("Error: " + msg.err.Error()))
		}
		m.lastPhrases = parsePhrases(msg.phrases)
		return m, tea.Println(renderPhrases(m.lastPhrases))

	case spinner.TickMsg:
		if m.busy {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	// Pass remaining messages to textinput
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m chatModel) view() string {
	if m.busy {
		return m.spinner.View() + dimStyle.Render(m.busyMsg+"...")
	}
	var hints []string
	if m.lastTranslation != nil {
		entries := flattenEntries(m.lastTranslation)
		if m.shownEntries < len(entries) {
			hints = append(hints, "/more", "/all")
		}
		hints = append(hints, "/add", "/phrases")
	}
	if len(m.lastPhrases) > 0 {
		hints = append(hints, "/cards")
	}
	hints = append(hints, "/help")
	return m.textInput.View() + "\n" + dimStyle.Render(strings.Join(hints, " "))
}

func (m *chatModel) handleCommand(input string) []tea.Cmd {
	parts := strings.Fields(input)
	switch parts[0] {
	case "/help":
		help := helpStyle.Render("Commands:") + "\n" +
			"  /more        — show next page of results\n" +
			"  /all         — show all remaining results\n" +
			"  /decks       — list decks\n" +
			"  /templates   — list templates\n" +
			"  /add [n]     — add card from translation row n\n" +
			"  /phrases <n> — generate example sentences for entry n\n" +
			"  /cards [n]   — create cards from phrase n (e.g. /cards 1 3 5)\n" +
			"  /help        — show this help"
		return []tea.Cmd{tea.Println(help)}

	case "/more":
		if m.lastTranslation == nil {
			return []tea.Cmd{tea.Println(errStyle.Render("No results to show."))}
		}
		entries := flattenEntries(m.lastTranslation)
		if m.shownEntries >= len(entries) {
			return []tea.Cmd{tea.Println(dimStyle.Render("No more results."))}
		}
		start := m.shownEntries
		end := min(start+pageSize, len(entries))
		m.shownEntries = end
		output := renderEntries(m.lastWord, entries, start, end)
		if end < len(entries) {
			output += "\n" + dimStyle.Render(fmt.Sprintf("  Showing %d of %d — /more for next page, /all for everything", end, len(entries)))
		}
		return []tea.Cmd{tea.Println(output)}

	case "/all":
		if m.lastTranslation == nil {
			return []tea.Cmd{tea.Println(errStyle.Render("No results to show."))}
		}
		entries := flattenEntries(m.lastTranslation)
		if m.shownEntries >= len(entries) {
			return []tea.Cmd{tea.Println(dimStyle.Render("No more results."))}
		}
		start := m.shownEntries
		m.shownEntries = len(entries)
		return []tea.Cmd{tea.Println(renderEntries(m.lastWord, entries, start, len(entries)))}

	case "/decks":
		return []tea.Cmd{m.setBusy(true, "Loading decks"), listDecksCmd()}

	case "/templates":
		return []tea.Cmd{m.setBusy(true, "Loading templates"), listTemplatesCmd()}

	case "/add":
		if m.lastTranslation == nil {
			return []tea.Cmd{tea.Println(errStyle.Render("No translation to add from. Look up a word first."))}
		}
		if len(parts) < 2 {
			return []tea.Cmd{tea.Println(errStyle.Render("Usage: /add [index...]"))}
		}
		return []tea.Cmd{m.prepareAdd(parts[1:])}

	case "/phrases":
		if m.lastTranslation == nil {
			return []tea.Cmd{tea.Println(errStyle.Render("No translation available. Look up a word first."))}
		}
		if len(parts) < 2 {
			return []tea.Cmd{tea.Println(errStyle.Render("Usage: /phrases <n>"))}
		}
		n, err := strconv.Atoi(parts[1])
		if err != nil {
			return []tea.Cmd{tea.Println(errStyle.Render(fmt.Sprintf("%q is not a number", parts[1])))}
		}
		entries := flattenEntries(m.lastTranslation)
		idx := n - 1
		if idx < 0 || idx >= len(entries) {
			return []tea.Cmd{tea.Println(errStyle.Render(fmt.Sprintf("Invalid index: %d (must be 1-%d)", n, len(entries))))}
		}
		return []tea.Cmd{m.setBusy(true, "Generating example sentences"), phrasesCmd(entries[idx])}

	case "/cards", "/card":
		if len(m.lastPhrases) == 0 {
			return []tea.Cmd{tea.Println(errStyle.Render("No phrases available. Use /phrases <n> first."))}
		}
		if len(parts) < 2 {
			return []tea.Cmd{tea.Println(errStyle.Render("Usage: /cards [n...] (e.g. /cards 1 3 5)"))}
		}
		var phrases []Phrase
		for _, arg := range parts[1:] {
			n, err := strconv.Atoi(arg)
			if err != nil {
				return []tea.Cmd{tea.Println(errStyle.Render(fmt.Sprintf("%q is not a number", arg)))}
			}
			idx := n - 1
			if idx < 0 || idx >= len(m.lastPhrases) {
				return []tea.Cmd{tea.Println(errStyle.Render(fmt.Sprintf("Invalid index: %d (must be 1-%d)", n, len(m.lastPhrases))))}
			}
			phrases = append(phrases, m.lastPhrases[idx])
		}
		return []tea.Cmd{m.setBusy(true, "Creating cards"), createPhraseCardsCmd(phrases)}

	default:
		return []tea.Cmd{tea.Println(errStyle.Render(fmt.Sprintf("Unknown command: %s", parts[0])))}
	}
}

func (m *chatModel) prepareAdd(params []string) tea.Cmd {
	allEntries := []ParsedEntry{}
	for _, section := range m.lastTranslation.Translations {
		for _, entry := range section.Entries {
			allEntries = append(allEntries, entry)
		}
	}

	paramNums := []int{}
	for _, param := range params {
		n, err := strconv.Atoi(param)
		if err != nil {
			return tea.Println(errStyle.Render(fmt.Sprintf("%q is not a number", param)))
		}
		paramNums = append(paramNums, n)
	}

	out := &bytes.Buffer{}
	enc := yaml.NewEncoder(out, yaml.UseLiteralStyleIfMultiline(true))
	for _, n := range paramNums {
		idx := n - 1
		if idx < 0 || idx >= len(allEntries) {
			return tea.Println(errStyle.Render(fmt.Sprintf("Invalid index: %d", n)))
		}
		entry := allEntries[idx]
		templ := EditTemplate{
			TargetLang: entry.FromWord.String(),
			SourceLang: strings.Join(Map(entry.ToWords, func(tw ToWord) string {
				return tw.String()
			}), "\n"),
		}
		if len(entry.FromExample) > 0 {
			templ.SourceExample = entry.FromExample
		}
		if len(entry.ToExample) > 0 {
			templ.TargetExample = entry.ToExample[0]
		}
		if err := enc.Encode(templ); err != nil {
			return tea.Println(errStyle.Render(fmt.Sprintf("YAML encode error: %s", err)))
		}
	}

	tmpFile, err := os.CreateTemp("", "wr_*.yml")
	if err != nil {
		return tea.Println(errStyle.Render(fmt.Sprintf("Failed to create temp file: %s", err)))
	}
	if _, err := io.WriteString(tmpFile, out.String()); err != nil {
		return tea.Println(errStyle.Render(fmt.Sprintf("Failed to write temp file: %s", err)))
	}
	tmpFile.Close()
	tmpPath := tmpFile.Name()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}

	m.setBusy(true)
	c := exec.Command(editor, tmpPath)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return editorFinishedMsg{err: err}
		}
		content, readErr := os.ReadFile(tmpPath)
		os.Remove(tmpPath)
		return editorFinishedMsg{content: string(content), err: readErr}
	})
}

// Styles

var (
	echoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#fad07a")).Bold(true)
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00"))
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#8197bf")).Bold(true)
)

// Rendering helpers

func flattenEntries(t *Translation) []ParsedEntry {
	var entries []ParsedEntry
	for _, section := range t.Translations {
		entries = append(entries, section.Entries...)
	}
	return entries
}

func renderEntries(word string, entries []ParsedEntry, start, end int) string {
	wordStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#fad07a")).Bold(true).Underline(true)
	idxStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Bold(true)
	fromExStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#70b950"))
	toExStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#c6b6ee")).Italic(true)
	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8197bf"))
	bar := barStyle.Render("│")

	var sb strings.Builder
	if start == 0 {
		sb.WriteString(wordStyle.Render(word) + "\n\n")
	}

	for i := start; i < end; i++ {
		entry := entries[i]
		var block strings.Builder
		block.WriteString(fmt.Sprintf("%s %s\n", idxStyle.Render(fmt.Sprintf("%d.", i+1)), entry.FromWord.ColorString()))
		for _, tw := range entry.ToWords {
			block.WriteString(fmt.Sprintf("   %s\n", tw.ColorString()))
		}
		if entry.FromExample != "" {
			block.WriteString(fmt.Sprintf("\n   %s\n", fromExStyle.Render(entry.FromExample)))
		}
		for _, ex := range entry.ToExample {
			block.WriteString(fmt.Sprintf("   %s\n", toExStyle.Render(ex)))
		}

		lines := strings.Split(strings.TrimRight(block.String(), "\n"), "\n")
		for _, line := range lines {
			sb.WriteString(fmt.Sprintf(" %s %s\n", bar, line))
		}
		sb.WriteString(fmt.Sprintf(" %s\n", renderFadeLine()))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func renderDecks(decks []Deck) string {
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#fad07a")).Bold(true)
	idSt := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff"))

	var sb strings.Builder
	for i, deck := range decks {
		sb.WriteString(fmt.Sprintf("%s %s\n", nameStyle.Render(deck.Name), idSt.Render(deck.ID)))
		if deck.TemplateID != "" {
			sb.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("template"), idSt.Render(deck.TemplateID)))
		}
		if i < len(decks)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func renderTemplates(templates []Template) string {
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#fad07a")).Bold(true)
	idSt := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	fieldStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff"))
	fieldIDStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))

	var sb strings.Builder
	for i, tmpl := range templates {
		sb.WriteString(fmt.Sprintf("%s %s\n", nameStyle.Render(tmpl.Name), idSt.Render(tmpl.ID)))
		for _, field := range tmpl.Fields {
			sb.WriteString(fmt.Sprintf("  %s %s\n", fieldStyle.Render(field.Name), fieldIDStyle.Render(field.ID)))
		}
		if i < len(templates)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func parsePhrases(raw string) []Phrase {
	var phrases []Phrase
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "- ")
		// Strip ** markers for stored plain text
		plain := strings.ReplaceAll(line, "**", "")
		if parts := strings.SplitN(plain, " — ", 2); len(parts) == 2 {
			phrases = append(phrases, Phrase{Source: parts[0], Target: parts[1]})
		}
	}
	return phrases
}

func renderPhrases(phrases []Phrase) string {
	fromStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#70b950"))
	toStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#c6b6ee")).Italic(true)
	idxStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Bold(true)

	var sb strings.Builder
	for i, p := range phrases {
		sb.WriteString(fmt.Sprintf("  %s %s — %s\n",
			idxStyle.Render(fmt.Sprintf("%d.", i+1)),
			fromStyle.Render(p.Source),
			toStyle.Render(p.Target),
		))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func renderFadeLine() string {
	// Base color #8197bf = rgb(129, 151, 191), fade toward black
	const steps = 12
	r0, g0, b0 := 129, 151, 191
	var sb strings.Builder
	// The corner piece at full color
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#8197bf")).Render("╰"))
	for i := range steps {
		t := float64(i) / float64(steps-1)
		r := int(float64(r0) * (1 - t))
		g := int(float64(g0) * (1 - t))
		b := int(float64(b0) * (1 - t))
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b)))
		sb.WriteString(style.Render("─"))
	}
	return sb.String()
}

// Utilities


func Map[T any, U any](input []T, fn func(T) U) []U {
	result := make([]U, len(input))
	for i, v := range input {
		result[i] = fn(v)
	}
	return result
}
