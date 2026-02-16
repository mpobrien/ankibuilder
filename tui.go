package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/goccy/go-yaml"
)

type appMode int

const (
	modeChat appMode = iota
)

type model struct {
	mode         appMode
	chat         chatModel
	wr           *WordReference
	windowWidth  int
	windowHeight int
}

func newModel(wr *WordReference) model {
	return model{
		mode: modeChat,
		chat: newChatModel(wr),
		wr:   wr,
	}
}

func (m model) Init() tea.Cmd {
	return m.chat.textInput.Focus()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		m.chat.setWidth(msg.Width)
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.chat, cmd = m.chat.update(msg)
	return m, cmd
}

func (m model) View() string {
	return m.chat.view()
}

// Message types

type translateResultMsg struct {
	word        string
	translation *Translation
	err         error
}

type listDecksResultMsg struct {
	decks []Deck
	err   error
}

type listTemplatesResultMsg struct {
	templates []Template
	err       error
}

type addCardResultMsg struct {
	count int
	err   error
}

type editorFinishedMsg struct {
	content string
	err     error
}

type phrasesResultMsg struct {
	phrases string
	err     error
}

// Async commands

func translateCmd(wr *WordReference, word string) tea.Cmd {
	return func() tea.Msg {
		translation, err := wr.Translate(word)
		return translateResultMsg{word: word, translation: translation, err: err}
	}
}

func listDecksCmd() tea.Cmd {
	return func() tea.Msg {
		key, _ := os.LookupEnv("MOCHI_KEY")
		mc := NewMochiClient(key)
		decks, err := mc.ListDecks()
		return listDecksResultMsg{decks: decks, err: err}
	}
}

func listTemplatesCmd() tea.Cmd {
	return func() tea.Msg {
		key, _ := os.LookupEnv("MOCHI_KEY")
		mc := NewMochiClient(key)
		templates, err := mc.ListTemplates()
		return listTemplatesResultMsg{templates: templates, err: err}
	}
}

func phrasesCmd(entry ParsedEntry) tea.Cmd {
	return func() tea.Msg {
		client := NewOpenAIClient()
		systemPrompt := "You are a language learning assistant. Generate 5 short example sentences in Spanish that use the given word with the given meaning. Include an English translation for each. Wrap the target word/phrase in the Spanish sentence with **asterisks** for emphasis. Format each as: `- <Spanish sentence> — <English translation>`"
		meanings := strings.Join(Map(entry.ToWords, func(tw ToWord) string {
			return tw.Meaning
		}), ", ")
		userPrompt := fmt.Sprintf("Word: %s\nMeaning: %s", entry.FromWord.Source, meanings)
		result, err := client.ChatCompletion(systemPrompt, userPrompt)
		return phrasesResultMsg{phrases: result, err: err}
	}
}

func createPhraseCardsCmd(phrases []Phrase) tea.Cmd {
	return func() tea.Msg {
		key, _ := os.LookupEnv("MOCHI_KEY")
		mc := NewMochiClient(key)
		count := 0
		for _, p := range phrases {
			tmpl := &EditTemplate{
				TargetLang:    p.Source,
				SourceLang:    p.Target,
				TargetExample: p.Source,
				SourceExample: p.Target,
			}
			cards := generateCards(defaultDeckID, tmpl)
			for _, card := range cards {
				if _, err := mc.CreateCard(card); err != nil {
					return addCardResultMsg{count: count, err: fmt.Errorf("failed to create card: %w", err)}
				}
				count++
			}
		}
		return addCardResultMsg{count: count}
	}
}

func createCardsCmd(yamlContent string) tea.Cmd {
	return func() tea.Msg {
		var tmpl EditTemplate
		dec := yaml.NewDecoder(strings.NewReader(yamlContent))
		if err := dec.Decode(&tmpl); err != nil {
			return addCardResultMsg{err: fmt.Errorf("invalid yaml: %w", err)}
		}

		cards := generateCards(defaultDeckID, &tmpl)
		key, _ := os.LookupEnv("MOCHI_KEY")
		mc := NewMochiClient(key)
		count := 0
		for _, card := range cards {
			if _, err := mc.CreateCard(card); err != nil {
				return addCardResultMsg{count: count, err: fmt.Errorf("failed to create card: %w", err)}
			}
			count++
		}
		return addCardResultMsg{count: count}
	}
}
