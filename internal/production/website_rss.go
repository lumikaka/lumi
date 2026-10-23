package production

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"
)

// The publishing address is deliberately not guessed from the local preview.
// Replace this token in a deployment copy after choosing the site's public URL.
const websiteSiteURLToken = "{site_url}"

// Feed dates live in the existing manifest in SQLite, not a second public
// history file. Unchanged exports keep their dates and stable chapter GUIDs.
type websiteFeedState struct {
	ContentHash string                        `json:"content_hash"`
	UpdatedAt   time.Time                     `json:"updated_at"`
	Chapters    map[string]websiteFeedChapter `json:"chapters"`
}

type websiteFeedChapter struct {
	ContentHash string    `json:"content_hash"`
	PublishedAt time.Time `json:"published_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type websiteRSS struct {
	XMLName xml.Name          `xml:"rss"`
	Version string            `xml:"version,attr"`
	Atom    string            `xml:"xmlns:atom,attr"`
	Channel websiteRSSChannel `xml:"channel"`
}

type websiteRSSChannel struct {
	Title         string           `xml:"title"`
	Link          string           `xml:"link"`
	Description   string           `xml:"description"`
	Generator     string           `xml:"generator"`
	LastBuildDate string           `xml:"lastBuildDate"`
	Self          websiteRSSLink   `xml:"atom:link"`
	Items         []websiteRSSItem `xml:"item"`
}

type websiteRSSLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type websiteRSSItem struct {
	Title       string         `xml:"title"`
	Link        string         `xml:"link"`
	GUID        websiteRSSGUID `xml:"guid"`
	Description string         `xml:"description"`
	PubDate     string         `xml:"pubDate"`
	Updated     string         `xml:"atom:updated"`
}

type websiteRSSGUID struct {
	IsPermaLink string `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

func renderWebsiteRSS(site websitePresentation, previous websiteFeedState, now time.Time) ([]byte, websiteFeedState, error) {
	encoded, err := json.Marshal(site)
	if err != nil {
		return nil, websiteFeedState{}, err
	}
	state := websiteFeedState{ContentHash: websiteDigest(encoded), UpdatedAt: now.UTC(), Chapters: map[string]websiteFeedChapter{}}
	if previous.ContentHash == state.ContentHash && !previous.UpdatedAt.IsZero() {
		state.UpdatedAt = previous.UpdatedAt
	}
	description := "作品章节更新 / Chapter updates"
	if site.MissingCount > 0 {
		description += fmt.Sprintf(" · 此版本仅包含已有图片，另有 %d 张尚未收录 / %d unavailable images omitted", site.MissingCount, site.MissingCount)
	}
	feed := websiteRSS{Version: "2.0", Atom: "http://www.w3.org/2005/Atom", Channel: websiteRSSChannel{
		Title: site.Title, Link: websiteSiteURLToken + "/index.html", Description: description,
		Generator: "Lumi", LastBuildDate: state.UpdatedAt.UTC().Format(time.RFC1123Z),
		Self: websiteRSSLink{Href: websiteSiteURLToken + "/rss.xml", Rel: "self", Type: "application/rss+xml"},
	}}
	for _, chapter := range site.Chapters {
		// An empty chapter is navigable in the website, but is not a release.
		if len(chapter.Pages) == 0 {
			continue
		}
		encoded, err := json.Marshal(chapter)
		if err != nil {
			return nil, websiteFeedState{}, err
		}
		entry := websiteFeedChapter{ContentHash: websiteDigest(encoded), PublishedAt: now.UTC(), UpdatedAt: now.UTC()}
		if old, ok := previous.Chapters[chapter.UUID]; ok && !old.PublishedAt.IsZero() {
			entry.PublishedAt = old.PublishedAt
			if old.ContentHash == entry.ContentHash && !old.UpdatedAt.IsZero() {
				entry.UpdatedAt = old.UpdatedAt
			}
		}
		state.Chapters[chapter.UUID] = entry
		parts := []string{}
		for _, part := range []string{chapter.Code, chapter.Title} {
			if strings.TrimSpace(part) != "" {
				parts = append(parts, part)
			}
		}
		title := strings.Join(parts, " · ")
		if title == "" {
			title = "未命名章节 / Untitled chapter"
		}
		link := websiteSiteURLToken + "/index.html#chapter=" + url.QueryEscape(chapter.UUID) + "&page=1"
		// Description is HTML within XML. Escape user text for HTML first;
		// encoding/xml then escapes the complete description for the XML layer.
		summary := fmt.Sprintf("<p>%d 张图片 / %d images</p><p><a href=\"%s\">打开阅读 / Read chapter</a></p>", len(chapter.Pages), len(chapter.Pages), html.EscapeString(link))
		summary += "<p><img src=\"" + websiteSiteURLToken + "/" + html.EscapeString(chapter.Pages[0].Source) + "\" alt=\"" + html.EscapeString(title) + "\"></p>"
		feed.Channel.Items = append(feed.Channel.Items, websiteRSSItem{
			Title: title, Link: link, GUID: websiteRSSGUID{IsPermaLink: "false", Value: "urn:uuid:" + chapter.UUID},
			Description: summary, PubDate: entry.PublishedAt.UTC().Format(time.RFC1123Z), Updated: entry.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	encoded, err = xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return nil, websiteFeedState{}, err
	}
	return append([]byte(xml.Header), append(encoded, '\n')...), state, nil
}
