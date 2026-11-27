package source

// 源适配层对外的统一模型：CatalogItem / DramaDetail / MediaStream。
// 上层只认这三个模型，站点字段差异在适配器边界处抹平。

type CatalogItem struct {
	ID       string `json:"id"`
	SourceID string `json:"sourceId,omitempty"`
	Title    string `json:"title"`
	Cover    string `json:"cover,omitempty"`
	Desc     string `json:"desc,omitempty"`
	VIP      *bool  `json:"vip,omitempty"`
	Episode  int    `json:"episode,omitempty"`
	Category string `json:"category,omitempty"`
}

type Episode struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Number int    `json:"number,omitempty"`
	VIP    bool   `json:"vip,omitempty"`
}

type DramaDetail struct {
	Item     CatalogItem `json:"item"`
	Intro    string      `json:"intro,omitempty"`
	Episodes []Episode   `json:"episodes"`
}

type StreamRequest struct {
	DramaID    string  `json:"dramaId"`
	DramaTitle string  `json:"dramaTitle,omitempty"`
	EpisodeID  string  `json:"episodeId"`
	Index      int     `json:"index,omitempty"`
	Episode    Episode `json:"episode"`
}

type MediaStream struct {
	URL     string            `json:"url"`
	Type    string            `json:"type,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Meta struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Version           string   `json:"version,omitempty"`
	Adult             bool     `json:"adult"`
	Capabilities      []string `json:"capabilities,omitempty"`
	AllowEmptyCatalog bool     `json:"allowEmptyCatalog,omitempty"`
}

const (
	CapabilityCatalog = "catalog"
	CapabilityDetail  = "detail"
	CapabilityStream  = "stream"
	CapabilitySearch  = "search"
)

func (m Meta) Supports(capability string) bool {
	if len(m.Capabilities) == 0 {
		return true
	}
	for _, item := range m.Capabilities {
		if item == capability {
			return true
		}
	}
	return false
}
