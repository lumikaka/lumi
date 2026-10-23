package production

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type parsedWebsiteRSS struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Channel struct {
		// Match the namespaced link before encoding/xml's unqualified wildcard.
		Self struct {
			Href string `xml:"href,attr"`
		} `xml:"http://www.w3.org/2005/Atom link"`
		Title         string `xml:"title"`
		Link          string `xml:"link"`
		Description   string `xml:"description"`
		LastBuildDate string `xml:"lastBuildDate"`
		Items         []struct {
			Title       string         `xml:"title"`
			Link        string         `xml:"link"`
			Description string         `xml:"description"`
			PubDate     string         `xml:"pubDate"`
			Updated     string         `xml:"http://www.w3.org/2005/Atom updated"`
			GUID        websiteRSSGUID `xml:"guid"`
		} `xml:"item"`
	} `xml:"channel"`
}

func parseWebsiteRSS(t *testing.T, b []byte) parsedWebsiteRSS {
	t.Helper()
	var feed parsedWebsiteRSS
	if err := xml.Unmarshal(b, &feed); err != nil {
		t.Fatal(err)
	}
	return feed
}

func TestWebsiteRSSDatesEscapingOrderingAndDeploymentToken(t *testing.T) {
	now := time.Date(2026, 10, 23, 8, 9, 10, 0, time.UTC)
	first := websiteDisplayChapter{UUID: mustUUID(t), Code: "01", Title: `月光 & <img src=x onerror="bad"> ]]>`, Pages: []websiteDisplayPage{{Source: "media/one.jpg", Title: "One"}}}
	second := websiteDisplayChapter{UUID: mustUUID(t), Code: "02", Title: "Next", Pages: []websiteDisplayPage{{Source: "media/two.jpg"}}}
	empty := websiteDisplayChapter{UUID: mustUUID(t), Title: "Empty", Pages: []websiteDisplayPage{}}
	site := websitePresentation{Title: "星星 & <月亮>", Format: "vertical_strip", MissingCount: 3, Chapters: []websiteDisplayChapter{first, empty, second}}
	b, state, err := renderWebsiteRSS(site, websiteFeedState{}, now)
	if err != nil {
		t.Fatal(err)
	}
	// Deployment substitutes the token after a host and subpath are known.
	base := "https://example.com/books/moon"
	deployed := []byte(strings.ReplaceAll(string(b), websiteSiteURLToken, base))
	feed := parseWebsiteRSS(t, deployed)
	if feed.Version != "2.0" || len(feed.Channel.Items) != 2 || feed.Channel.Items[0].Title != "01 · "+first.Title {
		t.Fatalf("feed=%+v", feed)
	}
	if feed.Channel.Title != site.Title || feed.Channel.Link != base+"/index.html" || !strings.Contains(feed.Channel.Description, "3") || feed.Channel.LastBuildDate != now.Format(time.RFC1123Z) {
		t.Fatalf("channel metadata=%+v", feed.Channel)
	}
	if feed.Channel.Self.Href != base+"/rss.xml" {
		t.Fatalf("missing Atom self link: %+v", feed.Channel.Self)
	}
	item := feed.Channel.Items[0]
	if item.GUID.Value != "urn:uuid:"+first.UUID || item.GUID.IsPermaLink != "false" || item.PubDate != now.Format(time.RFC1123Z) || item.Updated != now.Format(time.RFC3339) {
		t.Fatalf("identity/dates=%+v", item)
	}
	if strings.Contains(item.Description, `<img src=x`) || !strings.Contains(item.Description, `&lt;img src=x onerror=&#34;bad&#34;&gt;`) || !strings.Contains(item.Description, base+"/media/one.jpg") {
		t.Fatalf("unsafe or incomplete HTML: %s", item.Description)
	}
	for _, item := range feed.Channel.Items {
		link, err := url.Parse(item.Link)
		if err != nil || link.Scheme != "https" || link.Host != "example.com" || link.Path != "/books/moon/index.html" || !strings.Contains(link.Fragment, "&page=1") {
			t.Fatalf("bad chapter link: %q %v", item.Link, err)
		}
	}
	// No-op builds are byte-identical, despite occurring later.
	unchanged, state, err := renderWebsiteRSS(site, state, now.Add(time.Hour))
	if err != nil || !bytes.Equal(b, unchanged) {
		t.Fatalf("unchanged RSS changed: %v", err)
	}
	// Reordering preserves item identities, publication dates and content dates.
	site.Chapters = []websiteDisplayChapter{second, first, empty}
	reordered, state, err := renderWebsiteRSS(site, state, now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	feed = parseWebsiteRSS(t, reordered)
	if feed.Channel.Items[0].GUID.Value != "urn:uuid:"+second.UUID || feed.Channel.Items[1].PubDate != item.PubDate || feed.Channel.Items[1].Updated != item.Updated {
		t.Fatalf("reorder republished chapters: %+v", feed.Channel.Items)
	}
	// Replacing one image updates only that chapter's content date.
	site.Chapters[1].Pages[0].Source = "media/replaced.jpg"
	changed, state, err := renderWebsiteRSS(site, state, now.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	feed = parseWebsiteRSS(t, changed)
	if feed.Channel.Items[1].Updated != now.Add(3*time.Hour).Format(time.RFC3339) || feed.Channel.Items[0].Updated != now.Format(time.RFC3339) || feed.Channel.Items[1].PubDate != item.PubDate {
		t.Fatalf("content dates=%+v", feed.Channel.Items)
	}
	// Deleted and empty chapters no longer produce feed items or public history.
	site.Chapters = []websiteDisplayChapter{second, empty}
	removed, _, err := renderWebsiteRSS(site, state, now.Add(4*time.Hour))
	if err != nil || bytes.Contains(removed, []byte(first.UUID)) || len(parseWebsiteRSS(t, removed).Channel.Items) != 1 {
		t.Fatalf("removed chapter retained: %v", err)
	}
}

func TestWebsiteRSSIsManagedFrozenAndRepaired(t *testing.T) {
	h := newProductionHarness(t)
	s := h.service
	c := websiteChapter(t, h)
	websiteSection(t, h, c.UUID, imageBytes(t, 20))
	s.now = func() time.Time { return time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC) }
	runWebsite(t, s)
	output := filepath.Join(s.store.Root(), "website")
	readFeed := func() []byte {
		t.Helper()
		f, err := s.OpenWebsiteContent(t.Context(), "rss.xml")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	original := readFeed()
	if len(parseWebsiteRSS(t, original).Channel.Items) != 1 {
		t.Fatal("missing RSS chapter")
	}
	index, err := os.ReadFile(filepath.Join(output, "index.html"))
	if err != nil || !bytes.Contains(index, []byte(`rel="alternate" type="application/rss+xml"`)) || !bytes.Contains(index, []byte(`href="./rss.xml"`)) {
		t.Fatalf("RSS not discoverable: %v", err)
	}
	readme, err := os.ReadFile(filepath.Join(output, "README.md"))
	if err != nil || !bytes.Contains(readme, []byte(websiteSiteURLToken)) {
		t.Fatalf("missing deployment instructions: %v", err)
	}
	s.now = func() time.Time { return time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC) }
	// Missing/corrupt feeds regenerate without recompressing images or resetting dates.
	for _, mode := range []string{"missing", "corrupt"} {
		if mode == "missing" {
			err = os.Remove(filepath.Join(output, "rss.xml"))
		} else {
			err = os.WriteFile(filepath.Join(output, "rss.xml"), []byte("broken"), 0644)
		}
		if err != nil {
			t.Fatal(err)
		}
		g := runWebsite(t, s)
		if g.ProcessedImages != 0 || g.ReusedImages != 1 || !bytes.Equal(original, readFeed()) {
			t.Fatal("feed repair changed dates or recompressed images")
		}
	}
	task, _ := freezeWebsite(t, s)
	if err = s.store.DB().Exec(`UPDATE chapters SET title='Edited after freeze' WHERE uuid=?`, c.UUID).Error; err != nil {
		t.Fatal(err)
	}
	if err = s.RenderWebsite(t.Context(), task, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, readFeed()) {
		t.Fatal("live edits contaminated frozen RSS")
	}
	status, err := s.WebsiteStatus(t.Context())
	if err != nil || !status.HasUpdates {
		t.Fatalf("missing update status: %+v %v", status, err)
	}
	runWebsite(t, s)
	if !bytes.Contains(readFeed(), []byte("Edited after freeze")) {
		t.Fatal("next export did not update RSS")
	}
}

func TestWebsiteRSSUpgradeKeepsExistingCompressedImages(t *testing.T) {
	h := newProductionHarness(t)
	s := h.service
	c := websiteChapter(t, h)
	websiteSection(t, h, c.UUID, imageBytes(t, 25))
	g := runWebsite(t, s)
	var manifest websiteManifest
	if err := json.Unmarshal([]byte(g.ManifestJSON), &manifest); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"rss.xml", "README.md"} {
		delete(manifest.Files, name)
		if err := os.Remove(filepath.Join(s.store.Root(), "website", name)); err != nil {
			t.Fatal(err)
		}
	}
	// Legacy manifests had only file entries and no RSS dates/template version.
	legacyManifest, _ := json.Marshal(map[string]any{"files": manifest.Files})
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(g.SnapshotJSON), &snapshot); err != nil {
		t.Fatal(err)
	}
	delete(snapshot, "template_version")
	legacySnapshot, _ := json.Marshal(snapshot)
	if err := s.store.DB().Exec(`UPDATE website_generations SET manifest_json=?,snapshot_json=?,snapshot_hash=? WHERE id=?`, string(legacyManifest), string(legacySnapshot), hashJSON(legacySnapshot), g.ID).Error; err != nil {
		t.Fatal(err)
	}
	status, err := s.WebsiteStatus(t.Context())
	if err != nil || !status.Available || !status.HasUpdates {
		t.Fatalf("legacy status=%+v %v", status, err)
	}
	updated := runWebsite(t, s)
	if updated.ProcessedImages != 0 || updated.ReusedImages != 1 {
		t.Fatalf("upgrade recompressed images: %+v", updated)
	}
	feed, err := os.ReadFile(filepath.Join(s.store.Root(), "website/rss.xml"))
	if err != nil || len(parseWebsiteRSS(t, feed).Channel.Items) != 1 {
		t.Fatalf("upgrade omitted RSS: %v", err)
	}
	status, err = s.WebsiteStatus(t.Context())
	if err != nil || !status.Available || status.HasUpdates {
		t.Fatalf("upgraded status=%+v %v", status, err)
	}
}
