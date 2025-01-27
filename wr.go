package main

import (
	"bufio"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

const (
	WR_URL          = "https://www.wordreference.com/"
	TRANSLATION_URL = WR_URL + "%s/%s"
)

type WordReference struct {
	DictCode  string
	FromLang  string
	ToLang    string
	UserAgent string
}

type Translation struct {
	Word         string
	FromLang     string
	ToLang       string
	URL          string
	Translations []TranslationSection
}

type TranslationSection struct {
	Title   string
	Entries []ParsedEntry `json:"entries"`
}

// Fetches available dictionaries with optional language filtering.
func getAvailableDicts(langFilter string) (map[string]map[string]string, error) {
	resp, err := http.Get(WR_URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	dicts := make(map[string]map[string]string)

	doc.Find("select#fSelect optgroup option").Each(func(_ int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		id, exists := s.Attr("id")
		if !exists || !regexp.MustCompile(`.*-.*`).MatchString(text) {
			return
		}

		parts := strings.Split(text, "-")
		fromLangLabel := strings.TrimSpace(parts[0])
		toLangLabel := strings.TrimSpace(parts[1])
		fromLangCode := id[:2]
		toLangCode := id[2:]

		if langFilter != "" && !strings.Contains(strings.ToLower(langFilter), strings.ToLower(fromLangCode)) &&
			!strings.Contains(strings.ToLower(langFilter), strings.ToLower(fromLangLabel)) &&
			!strings.Contains(strings.ToLower(langFilter), strings.ToLower(toLangCode)) &&
			!strings.Contains(strings.ToLower(langFilter), strings.ToLower(toLangLabel)) {
			return
		}

		fromToLangCode := fromLangCode + toLangCode
		dicts[fromToLangCode] = map[string]string{
			"from": fromLangLabel,
			"to":   toLangLabel,
		}
	})

	return dicts, nil
}

// Initializes a WordReference object with validation.
func NewWordReference(fromLang, toLang string) (*WordReference, error) {
	dictCode := strings.ToLower(fromLang + toLang)
	availableDicts, err := getAvailableDicts("")
	if err != nil {
		return nil, err
	}

	if _, ok := availableDicts[dictCode]; !ok {
		return nil, fmt.Errorf("%s is not available as a translation dictionary", dictCode)
	}

	return &WordReference{
		DictCode:  dictCode,
		FromLang:  availableDicts[dictCode]["from"],
		ToLang:    availableDicts[dictCode]["to"],
		UserAgent: "GoHttpClient",
	}, nil
}

func (wr *WordReference) Translate(word string) (*Translation, error) {
	url := fmt.Sprintf(TRANSLATION_URL, wr.DictCode, word)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", wr.UserAgent)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check if the word is not found
	if noEntry := doc.Find("p#noEntryFound").Text(); noEntry != "" {
		return nil, errors.New(noEntry)
	}

	translation := &Translation{
		Word:         word,
		FromLang:     wr.FromLang,
		ToLang:       wr.ToLang,
		URL:          url,
		Translations: []TranslationSection{},
	}

	// Parse translation tables
	doc.Find("table.WRD").Each(func(_ int, table *goquery.Selection) {
		var entries []ParsedEntry
		var entryGroup []*goquery.Selection
		lastRowClass := ""

		// Process rows
		table.Find("tr.even, tr.odd").Each(func(i int, row *goquery.Selection) {
			currentRowClass, _ := row.Attr("class")
			// If row class changes, process the current group
			if currentRowClass != lastRowClass && len(entryGroup) > 0 {
				parsedEntry := parseEntry(entryGroup)
				entries = append(entries, parsedEntry)
				entryGroup = []*goquery.Selection{} // Reset the group
			}
			entryGroup = append(entryGroup, row)
			lastRowClass = currentRowClass
		})

		// Process the last group if it exists
		if len(entryGroup) > 0 {
			parsedEntry := parseEntry(entryGroup)
			entries = append(entries, parsedEntry)
		}

		// Get section title
		sectionTitle := table.Find("tr td").AttrOr("title", "Untitled Section")

		// Create a new section or find an existing one
		var section *TranslationSection
		for i := range translation.Translations {
			if translation.Translations[i].Title == sectionTitle {
				section = &translation.Translations[i]
				break
			}
		}

		if section == nil {
			newSection := TranslationSection{
				Title:   sectionTitle,
				Entries: []ParsedEntry{},
			}
			translation.Translations = append(translation.Translations, newSection)
			section = &translation.Translations[len(translation.Translations)-1]
		}

		section.Entries = entries
	})

	return translation, nil
}

type ParsedEntry struct {
	FromWord    FromWord `json:"from_word"`
	ToWords     []ToWord `json:"to_word"`
	Context     string   `json:"context"`
	FromExample string   `json:"from_example"`
	ToExample   []string `json:"to_example"`
}

type FromWord struct {
	Source  string
	Grammar string
}

func (fw FromWord) String() string {
	return fmt.Sprintf("%s (%s)", fw.Source, fw.Grammar)
}

func (fw FromWord) ColorString() string {
	sourceStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ffffff"))
	grammarStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#555555")).Italic(true)
	return fmt.Sprintf("%s %s",
		sourceStyle.Render(fw.Source),
		grammarStyle.Render(fw.Grammar),
	)
}

// Parses the source word from an entry.
func parseFromWord(entry []*goquery.Selection) FromWord {
	source := strings.TrimSpace(entry[0].Find("td.FrWrd strong").Text())
	source = strings.ReplaceAll(source, "⇒", "")
	var grammar string
	if em := entry[0].Find("td.FrWrd em.POS2"); em.Length() > 0 {
		grammar = strings.TrimSpace(em.Text())
	}
	return FromWord{source, grammar}
}

type ToWord struct {
	Meaning string
	Notes   string
	Grammar string
}

func (tw ToWord) ColorString() string {
	meaningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ffffff"))
	grammarStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#555555")).Italic(true)
	if len(tw.Notes) > 0 {
		return fmt.Sprintf("%s %s (%s)",
			meaningStyle.Render(tw.Meaning),
			grammarStyle.Render(tw.Grammar),
			meaningStyle.Render(tw.Notes),
		)
	}
	return fmt.Sprintf("%s %s",
		meaningStyle.Render(tw.Meaning),
		grammarStyle.Render(tw.Grammar),
	)
}

// Parses the target words from an entry.
func parseToWord(entry []*goquery.Selection) []ToWord {
	out := []ToWord{}
	for _, tr := range entry {
		if td := tr.Find("td.ToWrd"); td.Length() > 0 {
			var grammar, notes string
			if em := td.Find("em.POS2"); em.Length() > 0 {
				grammar = strings.TrimSpace(em.Text())
				em.Remove()
			}
			meaning := strings.TrimSpace(td.Text())
			meaning = strings.ReplaceAll(meaning, "⇒", "")

			if span := tr.Find("span.dsense i"); span.Length() > 0 {
				notes = strings.TrimSpace(span.Text())
			}
			out = append(out, ToWord{
				meaning,
				notes,
				grammar,
			})
		}
	}
	return out
}

// Parses the context from an entry.
func parseContext(entry []*goquery.Selection) string {
	context := entry[0].Find("td:nth-child(2)").Text()
	re := regexp.MustCompile(`$begin:math:text$(.*?)$end:math:text$`)
	if matches := re.FindStringSubmatch(context); len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// Parses the example from the source language.
func parseFromExample(entry []*goquery.Selection) string {
	for _, tr := range entry {
		if aux := tr.Find("td.FrEx"); aux.Length() > 0 {
			return strings.TrimSpace(aux.Text())
		}
	}
	return ""
}

// Parses the examples from the target language.
func parseToExample(entry []*goquery.Selection) []string {
	toExample := []string{}
	re := regexp.MustCompile(`ⓘ[^.]+\. *`)
	for _, tr := range entry {
		if aux := tr.Find("td.ToEx"); aux.Length() > 0 {
			example := strings.TrimSpace(aux.Text())
			example = re.ReplaceAllString(example, "")
			toExample = append(toExample, example)
		}
	}
	return toExample
}

// Parses a full entry into a structured format.
func parseEntry(entry []*goquery.Selection) ParsedEntry {
	return ParsedEntry{
		FromWord:    parseFromWord(entry),
		ToWords:     parseToWord(entry),
		Context:     parseContext(entry),
		FromExample: parseFromExample(entry),
		ToExample:   parseToExample(entry),
	}
}

func main() {
	// Example usage
	wr, err := NewWordReference("en", "es")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	headerStyle := lipgloss.NewStyle().
		PaddingLeft(2).
		PaddingRight(2).Foreground(lipgloss.Color("#ffffff")).Bold(true)
	cellStyle := lipgloss.NewStyle().
		PaddingLeft(2).
		PaddingRight(2).Foreground(lipgloss.Color("#ffffff")).PaddingBottom(2)

	promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#fad07a"))
	for {
		fmt.Print(promptStyle.Render("Enter word to look up: "))
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if len(input) == 0 {
			continue
		}

		translation, err := wr.Translate(input)
		if err != nil {
			fmt.Println("Error:", err)
			continue
		}

		inputWordStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#fad07a")).Bold(true).Padding(1).Underline(true)
		fmt.Println(inputWordStyle.Render(input))

		t := table.New().
			Border(lipgloss.NormalBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#8197bf"))).
			StyleFunc(func(row, col int) lipgloss.Style {
				if row < 0 {
					return headerStyle
				}
				return cellStyle
			}).
			Headers("Word", "Definition")
		for _, section := range translation.Translations {
			for _, entry := range section.Entries {
				row := EntryToRow(entry)
				t.Row(row...)
			}
		}

		fmt.Println(t)
	}

}

func EntryToRow(entry ParsedEntry) []string {
	defCol := strings.Join(Map(entry.ToWords, func(tw ToWord) string {
		return tw.ColorString()
	}), "\n")
	if len(entry.FromExample) > 0 || len(entry.ToExample) > 0 {
		defCol = defCol + "\n\n"
	}
	if len(entry.FromExample) > 0 {
		fromExStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#70b950"))
		defCol = defCol + fmt.Sprintf("%s", fromExStyle.Render(entry.FromExample))
	}
	if len(entry.ToExample) > 0 {
		toExStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c6b6ee")).Italic(true)
		defCol = defCol + "\n" +
			strings.Join(Map(entry.ToExample, func(v string) string {
				return toExStyle.Render(v)
			}), "\n")
	}

	return []string{
		entry.FromWord.ColorString(),
		defCol,
	}
}

// Map applies a function to each element of a slice and returns a new slice of the results.
func Map[T any, U any](input []T, fn func(T) U) []U {
	result := make([]U, len(input))
	for i, v := range input {
		result[i] = fn(v)
	}
	return result
}
