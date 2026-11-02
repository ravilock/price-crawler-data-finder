package messaging

import (
	"encoding/json"
	"testing"
	"time"
)

func TestProcessRetainsTraceAndRejectsInvalidInput(t *testing.T) {
	id := "00000000-0000-4000-8000-000000000001"
	raw := "R$ 12,34"
	cash := "à vista"
	payload, _ := json.Marshal(request{CrawlRequestID: id, PageParsedMessageID: id, NormalizationProfile: "v1", Page: Page{CrawlRequestID: id, RequestedURL: "https://store.example/a", FinalURL: "https://store.example/a", Outcome: "partial", ObservedAt: time.Now(), ParserVersion: "1", Fields: []PageField{{Name: "price", RawValue: &raw, EvidenceRef: ".price"}, {Name: "payment_method", RawValue: &cash, EvidenceRef: ".cash"}}}})
	e := Envelope{MessageID: id, MessageType: "product.find.requested", SchemaVersion: 1, OccurredAt: time.Now(), OrganizationID: id, CorrelationID: id, Payload: payload}
	encoded, _ := json.Marshal(e)
	result, err := Process(encoded)
	if err != nil || result.OrganizationID != id || result.CorrelationID != id || result.CausationID != id {
		t.Fatalf("result %+v err %v", result, err)
	}
	var out struct {
		Offer struct {
			PriceCents *int64 `json:"price_cents"`
		} `json:"offer"`
	}
	if err = json.Unmarshal(result.Payload, &out); err != nil || out.Offer.PriceCents == nil || *out.Offer.PriceCents != 1234 {
		t.Fatalf("result %s: %v", result.Payload, err)
	}
	e.SchemaVersion = 2
	encoded, _ = json.Marshal(e)
	if _, err = Process(encoded); err == nil {
		t.Fatal("accepted unsupported version")
	}
	e.SchemaVersion = 1
	e.OrganizationID = ""
	encoded, _ = json.Marshal(e)
	if _, err = Process(encoded); err == nil {
		t.Fatal("accepted missing tenant")
	}
}

func TestStructuredDataProducesConservativeEvidence(t *testing.T) {
	id := "00000000-0000-4000-8000-000000000001"
	page := Page{CrawlRequestID: id, RequestedURL: "https://store.example/p", FinalURL: "https://store.example/p", Outcome: "partial", ObservedAt: time.Now(), ParserVersion: "1", Fields: []PageField{}, StructuredData: []map[string]any{{"name": "Widget", "gtin13": "4006381333931", "offers": map[string]any{"price": "100.00", "priceCurrency": "BRL"}}}}
	payload, _ := json.Marshal(request{CrawlRequestID: id, PageParsedMessageID: id, Page: page})
	envelope, _ := json.Marshal(Envelope{MessageID: id, MessageType: "product.find.requested", SchemaVersion: 1, OccurredAt: time.Now(), OrganizationID: id, CorrelationID: id, Payload: payload})
	out, err := Process(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Fields map[string]struct {
			Status          string `json:"status"`
			NormalizedValue any    `json:"normalized_value"`
		} `json:"fields"`
		Offer struct {
			PriceStatus string `json:"price_status"`
			PriceCents  *int64 `json:"price_cents"`
		} `json:"offer"`
	}
	if err = json.Unmarshal(out.Payload, &result); err != nil {
		t.Fatal(err)
	}
	if result.Fields["title"].NormalizedValue != "Widget" || result.Fields["gtin"].Status != "valid" || result.Offer.PriceStatus != "uncertain" || result.Offer.PriceCents != nil {
		t.Fatalf("invalid result: %s", out.Payload)
	}
}
