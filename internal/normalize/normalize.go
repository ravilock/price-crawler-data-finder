// Package normalize implements deterministic source-field normalization.
package normalize

import (
	"regexp"
	"strconv"
	"strings"
)

type Identifier struct {
	Raw    string
	Value  string
	Valid  bool
	Reason string
}

func GTIN(raw string) Identifier {
	result := Identifier{Raw: raw, Value: strings.TrimSpace(raw)}
	s := result.Value
	if len(s) != 8 && len(s) != 12 && len(s) != 13 && len(s) != 14 {
		result.Reason = "invalid length"
		return result
	}
	sum := 0
	for i := len(s) - 2; i >= 0; i-- {
		if s[i] < '0' || s[i] > '9' {
			result.Reason = "non-digit"
			return result
		}
		weight := 1
		if (len(s)-2-i)%2 == 0 {
			weight = 3
		}
		sum += int(s[i]-'0') * weight
	}
	if s[len(s)-1] < '0' || s[len(s)-1] > '9' || (10-sum%10)%10 != int(s[len(s)-1]-'0') {
		result.Reason = "invalid check digit"
		return result
	}
	result.Valid = true
	return result
}

type Money struct {
	Raw           string
	Cents         int64
	Currency      string
	PaymentMethod string
	Valid         bool
	Reason        string
}

var brl = regexp.MustCompile(`^(?:R\$\s*)?((?:[0-9]{1,3}(?:\.[0-9]{3})+|[0-9]+)),([0-9]{2})$`)

func Price(raw, payment string) Money {
	p := Money{Raw: raw, Currency: "BRL", PaymentMethod: strings.TrimSpace(payment)}
	m := brl.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil || p.PaymentMethod == "" {
		p.Reason = "invalid price or missing payment method"
		return p
	}
	cents, err := strconv.ParseInt(strings.ReplaceAll(m[1], ".", "")+m[2], 10, 64)
	if err != nil {
		p.Reason = "price out of range"
		return p
	}
	p.Cents = cents
	p.Valid = true
	return p
}
func Availability(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "em estoque", "disponível", "disponivel", "in stock", "available":
		return "available"
	case "esgotado", "indisponível", "indisponivel", "out of stock", "unavailable":
		return "unavailable"
	default:
		return "unknown"
	}
}
