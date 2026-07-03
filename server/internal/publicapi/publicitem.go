package publicapi

import (
	"time"

	"aihot-server/internal/items"
)

// PublicItem is the browser-visible projection of items.Item. Wire field names
// mirror docs/references/openapi.yaml components.schemas.Item. Internal scoring /
// clustering columns are intentionally omitted.
type PublicItem struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	TitleEN     *string    `json:"title_en,omitempty"`
	URL         string     `json:"url"`
	Permalink   string     `json:"permalink"`
	Source      string     `json:"source"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
	Summary     *string    `json:"summary,omitempty"`
	Category    *string    `json:"category,omitempty"`
	Score       *int       `json:"score,omitempty"`
	Selected    bool       `json:"selected"`
}

// ItemList is the /api/public/items response envelope (openapi ItemList).
type ItemList struct {
	Count      int          `json:"count"`
	HasNext    bool         `json:"hasNext"`
	NextCursor *string      `json:"nextCursor"`
	Items      []PublicItem `json:"items"`
}

func toPublic(it items.Item) PublicItem {
	return PublicItem{
		ID:          it.ID,
		Title:       it.Title,
		TitleEN:     it.TitleEN,
		URL:         it.URL,
		Permalink:   it.Permalink,
		Source:      it.Source,
		PublishedAt: it.PublishedAt,
		Summary:     it.Summary,
		Category:    it.Category,
		Score:       it.Score,
		Selected:    it.Selected,
	}
}
