package pricing

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed books/anthropic_api.json books/openai_api.json books/copilot_credits.json
var bookFiles embed.FS

// AnthropicModelRate is one Anthropic model's API list price, USD per
// million tokens, split the way platform.claude.com/docs/en/about-claude/pricing
// itself splits prompt caching: a 5-minute and a 1-hour cache-write rate
// (the model chooses which one applies) and one cache-read (hit) rate. Unlike
// the old family-generic ModelPrice/CacheReadMult pair, the read rate is not
// a single global multiplier: Claude Opus 5.5 reads at 0.05x its base input
// price and Claude Fable 5.1 at 0.025x, both cheaper than the 0.1x every
// other model in this book uses, so each model's own absolute rate is stored
// here rather than derived.
type AnthropicModelRate struct {
	Label        string  `json:"label"`
	In           float64 `json:"in"`
	CacheWrite5m float64 `json:"cache_write_5m"`
	CacheWrite1h float64 `json:"cache_write_1h"`
	CacheRead    float64 `json:"cache_read"`
	Out          float64 `json:"out"`
}

// AnthropicAPIBook is the Anthropic API list-price book: C1's first kind,
// "API list price per vendor and model", for vendor "anthropic". Date and
// Source record when and where the numbers below were last checked, printed
// by `burnmon-cli price-check`.
type AnthropicAPIBook struct {
	Date   string                         `json:"date"`
	Source string                         `json:"source"`
	Models map[string]AnthropicModelRate  `json:"models"`
}

// OpenAIModelRate is one OpenAI/Codex model's API list price, USD per
// million tokens: standard-tier, short-context rates from
// developers.openai.com/api/docs/pricing.
type OpenAIModelRate struct {
	Label    string  `json:"label"`
	In       float64 `json:"in"`
	CachedIn float64 `json:"cached_in"`
	Out      float64 `json:"out"`
	// Source and Date are set only for a model whose rate comes from a page
	// other than the book's own source (OpenAI's model page); empty means
	// the book's Source and Date hold.
	Source string `json:"source,omitempty"`
	Date   string `json:"date,omitempty"`
}

// OpenAIAPIBook is the OpenAI API list-price book, for vendor "openai".
type OpenAIAPIBook struct {
	Date   string                     `json:"date"`
	Source string                     `json:"source"`
	Models map[string]OpenAIModelRate `json:"models"`
}

// CreditModelRate is one model's GitHub Copilot AI-credit rate, expressed as
// USD-equivalent per million tokens (the docs page prices every model in USD
// per million tokens, then converts the total to credits at CreditUSD): the
// same shape as the API books so CostForEvents can price a turn the same way
// and only relabel the total as credits at the end.
type CreditModelRate struct {
	Label      string  `json:"label"`
	In         float64 `json:"in"`
	CachedIn   float64 `json:"cached_in"`
	CacheWrite float64 `json:"cache_write"`
	Out        float64 `json:"out"`
}

// CreditPlan is one Copilot individual plan's monthly AI-credit allotment,
// auxiliary information (not used by CostForEvents, C2's cost function)
// printed by `burnmon-cli price-check` alongside the per-model rates. GitHub
// Copilot Business and Enterprise plans are not listed here: their seat
// credit allotments sit behind a "for businesses" tab that a plain page
// fetch does not render, so they are VERIFY (SESSION_LOG.md), not guessed.
type CreditPlan struct {
	MonthlyUSD     float64 `json:"monthly_usd"`
	MonthlyCredits float64 `json:"monthly_credits"`
}

// CreditBook is C1's second kind, "plan credits where the vendor publishes a
// table": GitHub Copilot's AI credits, for vendor "github". CreditUSD is the
// dollar value of one credit (1 credit = $0.01, docs.github.com/.../models-and-pricing).
type CreditBook struct {
	Date        string                      `json:"date"`
	Source      string                      `json:"source"`
	CreditUSD   float64                     `json:"credit_usd"`
	PlansSource string                      `json:"plans_source"`
	Plans       map[string]CreditPlan       `json:"plans"`
	Models      map[string]CreditModelRate  `json:"models"`
}

func loadEmbeddedBook(name string, v any) {
	b, err := bookFiles.ReadFile("books/" + name)
	if err != nil {
		panic(fmt.Sprintf("pricing: embedded book %s missing: %v", name, err))
	}
	if err := json.Unmarshal(b, v); err != nil {
		panic(fmt.Sprintf("pricing: embedded book %s invalid: %v", name, err))
	}
}

// defaultAnthropicAPIBook, defaultOpenAIAPIBook and defaultCreditBook parse
// the compiled-in JSON books embedded above. Panicking on a parse failure is
// deliberate: a malformed embedded book is a build-time bug, not a runtime
// condition any caller can recover from.
func defaultAnthropicAPIBook() AnthropicAPIBook {
	var b AnthropicAPIBook
	loadEmbeddedBook("anthropic_api.json", &b)
	return b
}

func defaultOpenAIAPIBook() OpenAIAPIBook {
	var b OpenAIAPIBook
	loadEmbeddedBook("openai_api.json", &b)
	return b
}

func defaultCreditBook() CreditBook {
	var b CreditBook
	loadEmbeddedBook("copilot_credits.json", &b)
	return b
}
