package items

import "time"

// Category slugs (match docs/references/openapi.yaml Item.category enum).
const (
	CategoryAIModels   = "ai-models"
	CategoryAIProducts = "ai-products"
	CategoryIndustry   = "industry"
	CategoryPaper      = "paper"
	CategoryTip        = "tip"
)

// epoch is the sort value used for items with NULL published_at (they sort last).
var epoch = time.Unix(0, 0).UTC()

// Item is the full internal row model. The public API (P1.4) projects a subset.
// Nullable columns are pointers.
type Item struct {
	ID             string
	Title          string
	TitleEN        *string
	URL            string
	Permalink      string
	Source         string
	SourceKind     string
	PublishedAt    *time.Time
	TimelineAt     *time.Time
	Summary        *string
	Body           *string
	Reason         *string
	ImageURL       *string
	VideoURL       *string
	Category       *string
	Score          *int
	AIRelevance    *int
	AISelected     *bool
	Selected       bool
	ClusterID      *string
	DuplicateOfID  *string
	ClusterPrimary *bool
	Present        bool
}

// SortKey is the ordering value: published_at, or epoch when null.
func (i Item) SortKey() time.Time {
	if i.PublishedAt != nil {
		return *i.PublishedAt
	}
	return epoch
}
