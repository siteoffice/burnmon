package pricing

import (
	"sort"

	"burnmon/internal/schema"
)

// Basis is one way of pricing a set of events: the vendor's published API
// list price, a plan-credit conversion, or a subscription's share of the
// invoice. C2 (spec 2.1): "Each event gets cost on all three bases where a
// book covers it, computed on read, never stored."
type Basis string

const (
	BasisAPIList      Basis = "api_list"
	BasisCredits      Basis = "credits"
	BasisSubscription Basis = "subscription"
)

// BasisCost is one cost figure on one Basis. Credits is only meaningful when
// Basis is BasisCredits (the same total, expressed in the vendor's own
// credit unit rather than USD). JSON tags added in v0.3 C3 (live.Session's
// BusinessCost is the first field to carry this over the wire to the
// template): every other exported struct in this codebase uses explicit
// snake_case tags, so this matches rather than falling back to Go's default
// capitalised field names.
type BasisCost struct {
	Basis   Basis   `json:"basis"`
	Label   string  `json:"label"`
	USD     float64 `json:"usd"`
	Credits float64 `json:"credits,omitempty"`
}

// VendorCost is CostForEvents' per-vendor result: every basis that vendor's
// price books cover, in a fixed order (credits, subscription, api list), and
// the headline basis this vendor's config prefers. Headline is nil when no
// book at all covers the vendor ("tokens only": Hermes today, and any future
// vendor with no book).
type VendorCost struct {
	Vendor   string      `json:"vendor"`
	Bases    []BasisCost `json:"bases"`
	Headline *BasisCost  `json:"headline,omitempty"`
}

// SubscriptionConfigured reports whether Subscription carries a real
// calibration rather than Defaults' illustrative example: the headline rule
// ("subscription share when configured, else API list labelled upper
// bound") reads this before offering the subscription basis at all.
func (c *Config) SubscriptionConfigured() bool {
	return c.Subscription.CalibratedOn != "" && c.Subscription.CalibratedOn != "example"
}

// CostForEvents is C2's one Go function: cost on every basis a book covers
// for a set of events, grouped by vendor, plus the headline basis per
// vendor given cfg. One function backs Now, History, Sessions and export, so
// the numbers agree everywhere (spec 2.1 C2).
func (c *Config) CostForEvents(events []schema.Event) []VendorCost {
	byVendor := map[string][]schema.Event{}
	for _, e := range events {
		byVendor[e.Vendor] = append(byVendor[e.Vendor], e)
	}
	vendors := make([]string, 0, len(byVendor))
	for v := range byVendor {
		vendors = append(vendors, v)
	}
	sort.Strings(vendors)

	out := make([]VendorCost, 0, len(vendors))
	for _, vendor := range vendors {
		out = append(out, c.vendorCost(vendor, byVendor[vendor]))
	}
	return out
}

func (c *Config) vendorCost(vendor string, events []schema.Event) VendorCost {
	vc := VendorCost{Vendor: vendor}

	switch vendor {
	case "anthropic":
		vc.Bases = append(vc.Bases, BasisCost{
			Basis: BasisAPIList, Label: "API list", USD: c.anthropicAPICostUSD(events),
		})
		if c.SubscriptionConfigured() {
			vc.Bases = append(vc.Bases, BasisCost{
				Basis: BasisSubscription, Label: "Subscription share", USD: c.anthropicSubscriptionCostUSD(events),
			})
		}
	case "openai":
		vc.Bases = append(vc.Bases, BasisCost{
			Basis: BasisAPIList, Label: "API list", USD: c.openAIAPICostUSD(events),
		})
	case "github":
		usd := c.copilotCreditCostUSD(events)
		vc.Bases = append(vc.Bases, BasisCost{
			Basis: BasisCredits, Label: "GitHub Copilot credits", USD: usd, Credits: usd / c.CopilotCredits.creditUSD(),
		})
	}
	// Any other vendor (e.g. "nous"/Hermes): no case matches, Bases stays
	// nil, Headline resolves to nil below. "Tokens only."

	vc.Headline = headlineBasis(vendor, vc.Bases)
	return vc
}

// headlineBasis picks the one basis a reader sees first, per spec 2.1 C2:
// "credits for Copilot, subscription share when configured, else API list
// labelled upper bound." vendor only selects the Copilot-first rule; every
// other vendor falls through subscription, then API list.
func headlineBasis(vendor string, bases []BasisCost) *BasisCost {
	find := func(b Basis) *BasisCost {
		for i := range bases {
			if bases[i].Basis == b {
				return &bases[i]
			}
		}
		return nil
	}
	if vendor == "github" {
		if b := find(BasisCredits); b != nil {
			return b
		}
	}
	if b := find(BasisSubscription); b != nil {
		return b
	}
	if b := find(BasisAPIList); b != nil {
		upper := *b
		upper.Label = "API list (upper bound)"
		return &upper
	}
	return nil
}

func cacheTokens(cacheWrite, cacheRead *int64) (write, read int64) {
	if cacheWrite != nil {
		write = *cacheWrite
	}
	if cacheRead != nil {
		read = *cacheRead
	}
	return write, read
}

// anthropicEventCostUSD prices one anthropic-vendor event at its exact
// model's AnthropicBook rate, cache writes at the 5-minute rate (the same
// convention the family-generic CacheWriteMult used: most Claude Code
// traffic re-primes well inside 5 minutes). ok is false when the model has
// no entry in the book (an id the live check never confirmed), in which
// case usd is 0: the book itself, printed by price-check, is where a
// missing model is visible.
func (c *Config) anthropicEventCostUSD(e schema.Event) (usd float64, label string, ok bool) {
	rate, ok := c.AnthropicBook.Models[e.Model]
	if !ok {
		return 0, e.Model, false
	}
	cw, cr := cacheTokens(e.CacheWrite, e.CacheRead)
	usd = (float64(e.Input)*rate.In +
		float64(cw)*rate.CacheWrite5m +
		float64(cr)*rate.CacheRead +
		float64(e.Output)*rate.Out) / 1e6
	return usd, rate.Label, true
}

// anthropicEventSubCostUSD is the share of the subscription one event
// accounts for, output-driven (cache traffic is not charged), keyed by exact
// model rather than family so it tracks AnthropicBook's per-model split. 0
// when the model has no book entry.
func (c *Config) anthropicEventSubCostUSD(e schema.Event) float64 {
	rate, ok := c.AnthropicBook.Models[e.Model]
	if !ok {
		return 0
	}
	return (float64(e.Output)*rate.Out + float64(e.Input)*rate.In) / 1e6 * c.Subscription.OutputCostFactor
}

// anthropicAPICostUSD sums anthropicEventCostUSD across every
// anthropic-vendor event in events.
func (c *Config) anthropicAPICostUSD(events []schema.Event) float64 {
	var total float64
	for _, e := range events {
		if e.Vendor != "anthropic" {
			continue
		}
		usd, _, _ := c.anthropicEventCostUSD(e)
		total += usd
	}
	return total
}

// anthropicSubscriptionCostUSD sums anthropicEventSubCostUSD across every
// anthropic-vendor event in events.
func (c *Config) anthropicSubscriptionCostUSD(events []schema.Event) float64 {
	var total float64
	for _, e := range events {
		if e.Vendor != "anthropic" {
			continue
		}
		total += c.anthropicEventSubCostUSD(e)
	}
	return total
}

// openAIEventCostUSD prices one openai-vendor event at its exact model's
// OpenAIBook rate. ok is false when the model has no book entry (unconfirmed
// at the live check, or no longer on OpenAI's pricing page), in which case
// usd is 0 and label is the old OpenAIPrices display label, or the raw id.
func (c *Config) openAIEventCostUSD(e schema.Event) (usd float64, label string, ok bool) {
	rate, ok := c.OpenAIBook.Models[e.Model]
	if !ok {
		return 0, c.OpenAILabel(e.Model), false
	}
	_, cr := cacheTokens(e.CacheWrite, e.CacheRead)
	usd = (float64(e.Input)*rate.In + float64(cr)*rate.CachedIn + float64(e.Output)*rate.Out) / 1e6
	return usd, rate.Label, true
}

// openAIAPICostUSD sums openAIEventCostUSD across every openai-vendor event
// in events.
func (c *Config) openAIAPICostUSD(events []schema.Event) float64 {
	var total float64
	for _, e := range events {
		if e.Vendor != "openai" {
			continue
		}
		usd, _, _ := c.openAIEventCostUSD(e)
		total += usd
	}
	return total
}

// copilotEventCostUSD prices one github-vendor event at its exact model's
// CopilotCredits rate; the USD total converts to credits in the caller
// (vendorCost) via CopilotCredits.creditUSD(). ok is false when the model
// has no book entry, in which case usd is 0.
func (c *Config) copilotEventCostUSD(e schema.Event) (usd float64, label string, ok bool) {
	rate, ok := c.CopilotCredits.Models[e.Model]
	if !ok {
		return 0, e.Model, false
	}
	cw, cr := cacheTokens(e.CacheWrite, e.CacheRead)
	usd = (float64(e.Input)*rate.In +
		float64(cw)*rate.CacheWrite +
		float64(cr)*rate.CachedIn +
		float64(e.Output)*rate.Out) / 1e6
	return usd, rate.Label, true
}

// copilotCreditCostUSD sums copilotEventCostUSD across every github-vendor
// event in events.
func (c *Config) copilotCreditCostUSD(events []schema.Event) float64 {
	var total float64
	for _, e := range events {
		if e.Vendor != "github" {
			continue
		}
		usd, _, _ := c.copilotEventCostUSD(e)
		total += usd
	}
	return total
}

// EventCost prices one event through the same per-vendor book rules
// CostForEvents applies in bulk (C2: "one Go function ... so the numbers
// agree everywhere"), now also usable per event by Sessions' own
// per-call/per-model breakdown (internal/dataset.buildSession), which used
// to fall back to Claude's family-generic price table for every non-OpenAI
// vendor: a Copilot or Hermes call priced as if it were Claude Sonnet. sub
// is the subscription-share figure (0 for a vendor with no subscription
// calibration). unpriced is true, and usd/sub are 0, when the vendor has no
// book at all (Hermes/"nous", or any future vendor) or the exact model has
// no entry in its vendor's book; label is the book's own display label for
// the model, or the raw model id when unpriced, still useful as a grouping
// key.
func (c *Config) EventCost(e schema.Event) (usd, sub float64, label string, unpriced bool) {
	switch e.Vendor {
	case "anthropic":
		var ok bool
		usd, label, ok = c.anthropicEventCostUSD(e)
		sub = c.anthropicEventSubCostUSD(e)
		return usd, sub, label, !ok
	case "openai":
		// The OpenAIBook, as in CostForEvents: the old OpenAIPrices table
		// still priced models the book leaves out, so Sessions and BurnMon
		// Dev showed more than History for the same Codex session.
		var ok bool
		usd, label, ok = c.openAIEventCostUSD(e)
		return usd, usd, label, !ok
	case "github":
		var ok bool
		usd, label, ok = c.copilotEventCostUSD(e)
		return usd, usd, label, !ok
	default:
		return 0, 0, e.Model, true
	}
}

// CopilotCreditsLeft is C3/K3's "credits left where the book knows them":
// Config.CopilotPlan's monthly credit allotment (CreditPlan.MonthlyCredits)
// minus the credits already spent across every github-vendor event in
// monthEvents (normally the current calendar month's events, any vendor:
// copilotCreditCostUSD itself ignores non-github events). ok is false, and
// left/plan are zero, when CopilotPlan is unset or names a tier the book
// does not carry: the caller shows no line at all rather than a guessed
// number, per the decision this session (SESSION_LOG.md).
func (c *Config) CopilotCreditsLeft(monthEvents []schema.Event) (left float64, plan CreditPlan, ok bool) {
	if c.CopilotPlan == "" {
		return 0, CreditPlan{}, false
	}
	plan, ok = c.CopilotCredits.Plans[c.CopilotPlan]
	if !ok {
		return 0, CreditPlan{}, false
	}
	usedUSD := c.copilotCreditCostUSD(monthEvents)
	usedCredits := usedUSD / c.CopilotCredits.creditUSD()
	return plan.MonthlyCredits - usedCredits, plan, true
}

func (b CreditBook) creditUSD() float64 {
	return b.CreditUnitUSD()
}

// CreditUnitUSD returns the dollar value of one credit, falling back to
// $0.01 (docs.github.com/.../models-and-pricing's stated rate) when CreditUSD
// is unset, e.g. a burnmon.json override that only touched Models.
func (b CreditBook) CreditUnitUSD() float64 {
	if b.CreditUSD > 0 {
		return b.CreditUSD
	}
	return 0.01
}
