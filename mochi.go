package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type MochiClient struct {
	client *http.Client
	key    string
}

func NewMochiClient(key string) *MochiClient {
	return &MochiClient{
		client: &http.Client{},
		key:    key,
	}
}

func (mc *MochiClient) ListDecks() ([]Deck, error) {
	decks := Pagination[Deck]{}
	if err := mc.getJSON("https://app.mochi.cards/api/decks", &decks); err != nil {
		return nil, err
	}
	return decks.Docs, nil
}

func (mc *MochiClient) postJSON(path string, payload any, into any) error {
	req, err := http.NewRequest("POST", path, nil)
	if err != nil {
		return err
	}
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	if err := enc.Encode(payload); err != nil {
		return err
	}
	req.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))

	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth(mc.key, "")
	resp, err := mc.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed: %s", string(bodyText))
	}
	return json.Unmarshal(bodyText, into)
}

func (mc *MochiClient) getJSON(path string, into any) error {
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(mc.key, "")
	resp, err := mc.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(bodyText, into)
}

func (mc *MochiClient) CreateCard(card Card) (*Card, error) {
	var result Card
	if err := mc.postJSON("https://app.mochi.cards/api/cards", card, &result); err != nil {
		return &result, err
	}
	return &result, nil
}

type Pagination[T any] struct {
	Bookmark string `json:"bookmark"`
	Docs     []T    `json:"docs"`
}

type Deck struct {
	Name       string `json:"name"`
	ID         string `json:"id"`
	AnkiDeckID string `json:"anki/deck-id"`
	Sort       int    `json:"sort"`
	CardsView  string `json:"cards-view"`
	TemplateID string `json:"template-id"`
	Archived   bool   `json:"archived?"`
}

type MochiTime struct {
	Date time.Time `json:"date"`
}

type Field struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

type Card struct {
	ID            string           `json:"id,omitempty"`
	Content       string           `json:"content,omitempty"`
	DeckID        string           `json:"deck-id"`
	TemplateID    string           `json:"template-id"`
	Fields        map[string]Field `json:"fields"`
	ReviewReverse bool             `json:"review-reverse?"`
	Archived      bool             `json:"archived?"`
	Pos           string           `json:"pos,omitempty"`
	UpdatedAt     *MochiTime       `json:"updated-at,omitempty"`
	Tags          []string         `json:"tags,omitempty"`
	Name          *string          `json:"name,omitempty"`
	Reviews       []any            `json:"reviews"`
	CreatedAt     *MochiTime       `json:"created-at,omitempty"`
	New           *bool            `json:"new?,omitempty"`
}

type Template struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Pos     string `json:"pos"`
	Fields  map[string]struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Pos     string `json:"pos"`
		Options struct {
			MultiLine bool `json:"multi-line?"`
		} `json:"options"`
	} `json:"fields"`
}

func (mc *MochiClient) ListTemplates() ([]Template, error) {
	decks := Pagination[Template]{}
	if err := mc.getJSON("https://app.mochi.cards/api/templates", &decks); err != nil {
		return nil, err
	}
	return decks.Docs, nil
}

func (mc *MochiClient) ListAllCards() ([]Card, error) {
	decks := Pagination[Card]{}
	if err := mc.getJSON("https://app.mochi.cards/api/cards", &decks); err != nil {
		return nil, err
	}
	return decks.Docs, nil
}

func (mc *MochiClient) ListCardsInDeck(deckID string) ([]Card, error) {
	decks := Pagination[Card]{}
	// TODO uriencode
	url := fmt.Sprintf("https://app.mochi.cards/api/cards?deck-id=%s", deckID)
	if err := mc.getJSON(url, &decks); err != nil {
		return nil, err
	}
	return decks.Docs, nil
}
