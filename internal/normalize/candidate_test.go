package normalize

import "testing"

func TestCandidatePreservesEvidenceAndPayment(t *testing.T) {
	input := Input{CrawlRequestID: "id", Fields: map[string]Evidence{"title": {Raw: " Widget ", Locator: "title"}, "gtin": {Raw: "4006381333932", Locator: ".gtin"}}, Attributes: []InputAttribute{{Name: "capacity", Evidence: Evidence{Raw: "16", Locator: ".capacity"}}}, Price: Evidence{Raw: "R$ 1.234,56", Locator: ".price"}, Payment: Evidence{Raw: "parcelado"}, Availability: Evidence{Raw: "indisponível"}}
	got := Normalize(input)
	if got.Fields["title"].NormalizedValue != "Widget" || got.Fields["gtin"].Status != "invalid" || got.Fields["brand"].Status != "missing" {
		t.Fatalf("fields: %+v", got.Fields)
	}
	if len(got.Attributes) != 1 || got.Attributes[0].Name != "capacity" || got.Attributes[0].Value != "16" {
		t.Fatalf("attributes: %+v", got.Attributes)
	}
	if got.Offer.PriceCents == nil || *got.Offer.PriceCents != 123456 || got.Offer.PaymentMethod == nil || *got.Offer.PaymentMethod != "installment" || got.Offer.Availability != "unavailable" || *got.Offer.PriceField.EvidenceRef != ".price" {
		t.Fatalf("offer: %+v", got.Offer)
	}
	input.Payment.Raw = ""
	got = Normalize(input)
	if got.Offer.PriceStatus != "uncertain" || got.Offer.PriceCents != nil {
		t.Fatalf("unknown payment verified: %+v", got.Offer)
	}
	input.Price.Raw = "not money"
	got = Normalize(input)
	if got.Offer.PriceStatus != "invalid" || got.Offer.PriceCents != nil {
		t.Fatalf("invalid price verified: %+v", got.Offer)
	}
}

func TestMachinePricesAndBounds(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		want  int64
		valid bool
	}{{"100.00", 10000, true}, {"12", 1200, true}, {"92233720368547758,08", 0, false}, {"12.999", 0, false}} {
		got, err := BRLCents(tc.raw)
		if (err == nil) != tc.valid || tc.valid && got != tc.want {
			t.Errorf("%q got %d %v", tc.raw, got, err)
		}
	}
}

func TestCurrencyMustBeEstablished(t *testing.T) {
	in := Input{Price: Evidence{Raw: "100.00"}, Payment: Evidence{Raw: "cash"}}
	if got := Normalize(in); got.Offer.PriceStatus != "uncertain" || got.Offer.PriceCents != nil {
		t.Fatalf("unqualified currency: %+v", got.Offer)
	}
	in.Currency.Raw = "USD"
	if got := Normalize(in); got.Offer.PriceStatus != "invalid" || got.Offer.PriceCents != nil {
		t.Fatalf("foreign currency: %+v", got.Offer)
	}
	in.Currency.Raw = "BRL"
	if got := Normalize(in); got.Offer.PriceStatus != "valid" || got.Offer.PriceCents == nil || *got.Offer.PriceCents != 10000 {
		t.Fatalf("BRL: %+v", got.Offer)
	}
}
