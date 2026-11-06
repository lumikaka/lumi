package vacsimport

// ConversionNotice records a target display adaptation. Full original labels
// remain in the verbatim manifest as well as this readable conversion report.
type ConversionNotice struct {
	Code     string `json:"code"`
	Location string `json:"location"`
	Limit    int    `json:"limit"`
	Original string `json:"original"`
	Display  string `json:"display"`
}

func adaptDisplayTitles(m Manifest) (Manifest, []ConversionNotice) {
	notices := []ConversionNotice{}
	adapt := func(text string, limit int, location string) string {
		chars := []rune(text)
		if len(chars) <= limit {
			return text
		}
		display := string(chars[:limit-1]) + "…"
		notices = append(notices, ConversionNotice{Code: "display_title_shortened", Location: location, Limit: limit, Original: text, Display: display})
		return display
	}
	m.Name = adapt(m.Name, 120, "project/"+m.ProjectID)
	for ci := range m.Chapters {
		for pi := range m.Chapters[ci].Sections {
			page := &m.Chapters[ci].Sections[pi]
			page.Title = adapt(page.Title, 160, "chapter/"+m.Chapters[ci].ID+"/page/"+page.ID)
		}
	}
	return m, notices
}
