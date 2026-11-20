package pricing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadOverridesOneAnthropicModelKeepsTheRest guards C1's "overridable
// from burnmon.json with the existing override behaviour kept": a
// burnmon.json naming only one Anthropic model must not drop the others
// from the compiled-in book, the same partial-merge behaviour Prices and
// OpenAIPrices already have (encoding/json merges into a non-nil map).
func TestLoadOverridesOneAnthropicModelKeepsTheRest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "burnmon.json")
	override := map[string]any{
		"anthropic_book": map[string]any{
			"date":   "2026-10-01",
			"source": "https://example.invalid/recalibrated",
			"models": map[string]any{
				"claude-sonnet-5": map[string]any{"label": "Claude Sonnet 5", "in": 99.0, "cache_write_5m": 0, "cache_write_1h": 0, "cache_read": 0, "out": 99.0},
			},
		},
	}
	b, err := json.Marshal(override)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AnthropicBook.Models["claude-sonnet-5"].In != 99.0 {
		t.Fatalf("overridden model In = %v, want 99", cfg.AnthropicBook.Models["claude-sonnet-5"].In)
	}
	if _, ok := cfg.AnthropicBook.Models["claude-opus-5-5"]; !ok {
		t.Fatal("un-named model claude-opus-5-5 was dropped by a partial override, want it kept")
	}
	if cfg.AnthropicBook.Date != "2026-10-01" {
		t.Fatalf("book date = %q, want overridden date", cfg.AnthropicBook.Date)
	}
}

// TestDefaultsBooksAreNonEmpty guards the embedded JSON parsing itself: a
// broken embed (missing file, malformed JSON) would leave these maps empty
// rather than fail loudly at Load time, so the test checks contents
// directly.
func TestDefaultsBooksAreNonEmpty(t *testing.T) {
	cfg := Defaults()
	if len(cfg.AnthropicBook.Models) == 0 {
		t.Error("AnthropicBook.Models is empty")
	}
	if cfg.AnthropicBook.Date == "" || cfg.AnthropicBook.Source == "" {
		t.Error("AnthropicBook is missing date or source")
	}
	if len(cfg.OpenAIBook.Models) == 0 {
		t.Error("OpenAIBook.Models is empty")
	}
	if cfg.OpenAIBook.Date == "" || cfg.OpenAIBook.Source == "" {
		t.Error("OpenAIBook is missing date or source")
	}
	if len(cfg.CopilotCredits.Models) == 0 {
		t.Error("CopilotCredits.Models is empty")
	}
	if cfg.CopilotCredits.Date == "" || cfg.CopilotCredits.Source == "" {
		t.Error("CopilotCredits is missing date or source")
	}
	if cfg.CopilotCredits.CreditUnitUSD() != 0.01 {
		t.Errorf("CopilotCredits.CreditUnitUSD() = %v, want 0.01", cfg.CopilotCredits.CreditUnitUSD())
	}
}

// TestSubscriptionConfiguredFalseForDefaults guards SubscriptionConfigured's
// "example" exclusion directly (cost_test.go exercises it indirectly through
// the headline rule).
func TestSubscriptionConfiguredFalseForDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.SubscriptionConfigured() {
		t.Fatal("Defaults' illustrative example calibration must not count as configured")
	}
}

// TestOpenAIBookPricesTerraAndLuna pins the rates OpenAI publishes on its own
// model pages for gpt-5.6-terra and gpt-5.6-luna (USD per million tokens,
// read 2026-11-20), and the book says where and when for these two.
func TestOpenAIBookPricesTerraAndLuna(t *testing.T) {
	cfg := Defaults()
	want := map[string]OpenAIModelRate{
		"gpt-5.6-terra": {Label: "GPT-5.6 Terra", In: 2.00, CachedIn: 0.20, Out: 12.00},
		"gpt-5.6-luna":  {Label: "GPT-5.6 Luna", In: 0.20, CachedIn: 0.02, Out: 1.20},
	}
	for id, w := range want {
		got, ok := cfg.OpenAIBook.Models[id]
		if !ok {
			t.Errorf("%s: not in the OpenAI book", id)
			continue
		}
		if got.In != w.In || got.CachedIn != w.CachedIn || got.Out != w.Out {
			t.Errorf("%s: rate = %+v, want %+v", id, got, w)
		}
		if !strings.Contains(got.Source, "developers.openai.com/api/docs/models/"+id) || got.Date != "2026-11-20" {
			t.Errorf("%s: source %q date %q, want its model page and 2026-11-20", id, got.Source, got.Date)
		}
	}
}
