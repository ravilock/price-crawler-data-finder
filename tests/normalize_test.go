package tests

import (
	"testing"

	"price-crawler/services/data-finder/internal/normalize"
)

func TestGTIN(t *testing.T) {
	for _, tc := range []struct {
		input string
		valid bool
	}{{"7894900011517", true}, {"7894900011518", false}, {"123", false}, {"789490001151x", false}} {
		if got := normalize.GTIN(tc.input); got.Valid != tc.valid {
			t.Errorf("GTIN(%q): %+v", tc.input, got)
		}
	}
}

func TestPriceAndAvailability(t *testing.T) {
	p := normalize.Price("R$ 1.299,90", "pix")
	if !p.Valid || p.Cents != 129990 || p.PaymentMethod != "pix" {
		t.Fatalf("price: %+v", p)
	}
	for _, input := range []string{"R$ 1.2,90", "", "R$ 12,999"} {
		if normalize.Price(input, "cash").Valid {
			t.Errorf("accepted %q", input)
		}
	}
	if normalize.Price("R$ 10,00", "").Valid {
		t.Error("payment method required")
	}
	for input, want := range map[string]string{"em estoque": "available", "esgotado": "unavailable", "talvez": "unknown"} {
		if got := normalize.Availability(input); got != want {
			t.Errorf("%q: %q", input, got)
		}
	}
}
