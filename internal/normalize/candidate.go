package normalize

import (
	"strings"
	"unicode/utf8"
)

// Evidence captures a bounded source value and its locator. Empty input remains
// missing; unknown fields must not be promoted to verified facts.
type Evidence struct{ Raw, Locator string }
type Candidate struct {
	RawValue        any     `json:"raw_value"`
	NormalizedValue any     `json:"normalized_value"`
	EvidenceRef     *string `json:"evidence_ref"`
	Status          string  `json:"status"`
	Confidence      float64 `json:"confidence"`
	Reason          *string `json:"reason"`
}
type Attribute struct {
	Name  string    `json:"name"`
	Value any       `json:"value"`
	Unit  *string   `json:"unit"`
	Field Candidate `json:"field"`
}
type Offer struct {
	PriceCents    *int64    `json:"price_cents"`
	Currency      string    `json:"currency"`
	PaymentMethod *string   `json:"payment_method"`
	Availability  string    `json:"availability"`
	PriceStatus   string    `json:"price_status"`
	PriceField    Candidate `json:"price_field"`
}
type Result struct {
	CrawlRequestID       string               `json:"crawl_request_id"`
	NormalizationVersion string               `json:"normalization_version"`
	Fields               map[string]Candidate `json:"fields"`
	Attributes           []Attribute          `json:"attributes"`
	Offer                Offer                `json:"offer"`
}
type InputAttribute struct {
	Name     string
	Evidence Evidence
	Unit     *string
}
type Input struct {
	CrawlRequestID                         string
	Fields                                 map[string]Evidence
	Attributes                             []InputAttribute
	Price, Payment, Availability, Currency Evidence
}

func bounded(raw string, n int) string {
	if len(raw) <= n {
		return raw
	}
	raw = raw[:n]
	for !utf8.ValidString(raw) {
		raw = raw[:len(raw)-1]
	}
	return raw
}
func field(e Evidence, value any, status, reason string) Candidate {
	var locator, why *string
	if e.Locator != "" {
		v := bounded(e.Locator, 500)
		locator = &v
	}
	if reason != "" {
		v := reason
		why = &v
	}
	var raw any
	if e.Raw != "" {
		raw = bounded(e.Raw, 1024)
	}
	confidence := 0.0
	if status == "valid" {
		confidence = 1
	}
	return Candidate{RawValue: raw, NormalizedValue: value, EvidenceRef: locator, Status: status, Confidence: confidence, Reason: why}
}

// Normalize preserves missing and invalid facts rather than inferring them.
func Normalize(in Input) Result {
	result := Result{CrawlRequestID: in.CrawlRequestID, NormalizationVersion: "1", Fields: map[string]Candidate{}, Attributes: []Attribute{}, Offer: Offer{Currency: "BRL", Availability: Availability(in.Availability.Raw)}}
	for _, name := range []string{"title", "description", "brand", "manufacturer_model", "gtin"} {
		e := in.Fields[name]
		raw := strings.TrimSpace(e.Raw)
		if raw == "" {
			result.Fields[name] = field(e, nil, "missing", "source field absent")
			continue
		}
		if name == "gtin" {
			v, err := GTIN(raw)
			if err != nil {
				result.Fields[name] = field(e, nil, "invalid", "invalid GTIN check digit or shape")
				continue
			}
			result.Fields[name] = field(e, v, "valid", "")
			continue
		}
		result.Fields[name] = field(e, bounded(raw, 1024), "valid", "")
	}
	for _, a := range in.Attributes {
		if strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.Evidence.Raw) == "" {
			continue
		}
		v := bounded(strings.TrimSpace(a.Evidence.Raw), 1024)
		result.Attributes = append(result.Attributes, Attribute{Name: bounded(strings.TrimSpace(a.Name), 100), Value: v, Unit: a.Unit, Field: field(a.Evidence, v, "valid", "")})
	}
	payment := PaymentMethod(in.Payment.Raw)
	if payment != "" {
		result.Offer.PaymentMethod = &payment
	}
	status := "missing"
	reason := "source price absent"
	var value any
	if strings.TrimSpace(in.Price.Raw) != "" {
		cents, err := BRLCents(in.Price.Raw)
		if err != nil {
			status = "invalid"
			reason = "invalid BRL price"
		} else if in.Currency.Raw != "" && !strings.EqualFold(strings.TrimSpace(in.Currency.Raw), "BRL") {
			status = "invalid"
			reason = "unsupported currency"
		} else if in.Currency.Raw == "" && !strings.HasPrefix(strings.TrimSpace(in.Price.Raw), "R$") {
			status = "uncertain"
			reason = "currency not established"
		} else if payment == "" {
			status = "uncertain"
			reason = "payment method not established"
		} else {
			status = "valid"
			reason = ""
			value = cents
			result.Offer.PriceCents = &cents
		}
	}
	result.Offer.PriceStatus = status
	result.Offer.PriceField = field(in.Price, value, status, reason)
	result.Fields["price"] = result.Offer.PriceField
	if strings.TrimSpace(in.Payment.Raw) == "" {
		result.Fields["payment_method"] = field(in.Payment, nil, "missing", "payment method absent")
	} else if payment == "" {
		result.Fields["payment_method"] = field(in.Payment, nil, "uncertain", "unrecognized payment method")
	} else {
		result.Fields["payment_method"] = field(in.Payment, payment, "valid", "")
	}
	if strings.TrimSpace(in.Availability.Raw) == "" {
		result.Fields["availability"] = field(in.Availability, nil, "missing", "availability absent")
	} else if result.Offer.Availability == "unknown" {
		result.Fields["availability"] = field(in.Availability, "unknown", "uncertain", "unrecognized availability")
	} else {
		result.Fields["availability"] = field(in.Availability, result.Offer.Availability, "valid", "")
	}
	return result
}
