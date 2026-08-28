package types

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// WebSearchConfig represents the web search configuration for a tenant
type WebSearchConfig struct {
	// Deprecated: Use WebSearchProviderEntity.Parameters.APIKey instead.
	Provider string `json:"provider,omitempty"`
	// Deprecated: Use WebSearchProviderEntity.Parameters.APIKey instead.
	APIKey string `json:"api_key,omitempty"`

	MaxResults        int      `json:"max_results"`        // maximum number of search results
	IncludeDate       bool     `json:"include_date"`       // whether to include the date
	CompressionMethod string   `json:"compression_method"` // compression method: none, summary, extract, rag
	Blacklist         []string `json:"blacklist"`          // blacklist rule list
	// RAG compression-related config
	EmbeddingModelID   string `json:"embedding_model_id,omitempty"`  // embedding model ID (used for RAG compression)
	EmbeddingDimension int    `json:"embedding_dimension,omitempty"` // embedding dimension (used for RAG compression)
	RerankModelID      string `json:"rerank_model_id,omitempty"`     // rerank model ID (used for RAG compression)
	DocumentFragments  int    `json:"document_fragments,omitempty"`  // document fragment count (used for RAG compression)
	ProxyURL           string `json:"proxy_url,omitempty"`           // Optional per-request proxy override; normally empty — use WebSearchProviderEntity.Parameters.proxy_url. Merged at call time when set.
}

const (
	DefaultWebSearchMaxResults        = 10
	DefaultWebSearchCompressionMethod = "none"
)

// DefaultWebSearchConfig returns the shared default tenant-level web search configuration.
func DefaultWebSearchConfig() *WebSearchConfig {
	return &WebSearchConfig{
		MaxResults:        DefaultWebSearchMaxResults,
		IncludeDate:       false,
		CompressionMethod: DefaultWebSearchCompressionMethod,
		Blacklist:         []string{},
	}
}

// EffectiveWebSearchConfig normalizes a possibly empty config to the effective runtime config.
func EffectiveWebSearchConfig(cfg *WebSearchConfig) *WebSearchConfig {
	if cfg == nil {
		return DefaultWebSearchConfig()
	}

	normalized := *cfg
	if normalized.MaxResults <= 0 {
		normalized.MaxResults = DefaultWebSearchMaxResults
	}
	if normalized.CompressionMethod == "" {
		normalized.CompressionMethod = DefaultWebSearchCompressionMethod
	}
	if normalized.Blacklist == nil {
		normalized.Blacklist = []string{}
	}

	return &normalized
}

// Value implements driver.Valuer interface for WebSearchConfig
func (c WebSearchConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements sql.Scanner interface for WebSearchConfig
func (c *WebSearchConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}

// WebSearchResult represents a single web search result
type WebSearchResult struct {
	Title       string     `json:"title"`                  // 搜索结果标题
	URL         string     `json:"url"`                    // 结果URL
	Snippet     string     `json:"snippet"`                // 摘要片段
	Content     string     `json:"content"`                // 完整内容（可选，需要额外抓取）
	Source      string     `json:"source"`                 // 来源（如：duckduckgo等）
	PublishedAt *time.Time `json:"published_at,omitempty"` // 发布时间（如果有）
}

// WebSearchProviderInfo represents information about a web search provider
type WebSearchProviderInfo struct {
	ID             string `json:"id"`                // 提供商ID
	Name           string `json:"name"`              // 提供商名称
	Free           bool   `json:"free"`              // 是否免费
	RequiresAPIKey bool   `json:"requires_api_key"`  // 是否需要API密钥
	Description    string `json:"description"`       // 描述
	APIURL         string `json:"api_url,omitempty"` // API地址（可选）
}
