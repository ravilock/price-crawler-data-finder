package tests

import (
	"testing"

	"price-crawler/services/data-finder/internal/normalize"
)

// These examples define the normalization boundary for T030. Values must not be
// guessed when source evidence is absent or malformed.
func TestGTIN(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		valid bool
	}{
		{"4006381333931", true}, {"4006381333932", false},
		{"400638133393", false}, {"40063813339x1", false}, {"", false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			_, err := normalize.GTIN(tc.raw)
			if (err == nil) != tc.valid {
				t.Fatalf("GTIN(%q): error = %v, want valid %v", tc.raw, err, tc.valid)
			}
		})
	}
}

func TestBRLPrice(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		cents int64
		valid bool
	}{
		{"R$ 1.234,56", 123456, true}, {"R$ 0,00", 0, true},
		{"R$ 12,999", 0, false}, {"R$ -1,00", 0, false}, {"sob consulta", 0, false}, {"", 0, false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			cents, err := normalize.BRLCents(tc.raw)
			if (err == nil) != tc.valid || (tc.valid && cents != tc.cents) {
				t.Fatalf("BRLCents(%q) = %d, %v; want %d valid=%v", tc.raw, cents, err, tc.cents, tc.valid)
			}
		})
	}
}

func TestOfferFields(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"em estoque", "available"}, {"indisponível", "unavailable"}, {"", "unknown"}, {"consulte", "unknown"},
	} {
		if got := normalize.Availability(tc.raw); got != tc.want {
			t.Errorf("Availability(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
	for _, tc := range []struct{ raw, want string }{{"à vista", "cash"}, {"", ""}} {
		if got := normalize.PaymentMethod(tc.raw); got != tc.want {
			t.Errorf("PaymentMethod(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}
