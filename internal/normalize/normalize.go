// Package normalize transforms bounded source evidence into deterministic facts.
package normalize

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var errInvalid = errors.New("invalid value")
var decimalPricePattern = regexp.MustCompile(`^[0-9]+\.[0-9]{2}$`)
var integerPricePattern = regexp.MustCompile(`^[0-9]+$`)
var brlPattern = regexp.MustCompile(`^(?:R\$\s*)?((?:[0-9]{1,3}(?:\.[0-9]{3})+)|(?:[0-9]+)),([0-9]{2})$`)

func GTIN(raw string) (string, error) {
	if len(raw) != 8 && len(raw) != 12 && len(raw) != 13 && len(raw) != 14 {
		return "", errInvalid
	}
	sum := 0
	for i := len(raw) - 2; i >= 0; i-- {
		if raw[i] < '0' || raw[i] > '9' {
			return "", errInvalid
		}
		weight := 1
		if (len(raw)-2-i)%2 == 0 {
			weight = 3
		}
		sum += int(raw[i]-'0') * weight
	}
	last := raw[len(raw)-1]
	if last < '0' || last > '9' || int(last-'0') != (10-sum%10)%10 {
		return "", errInvalid
	}
	return raw, nil
}
func BRLCents(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if decimalPricePattern.MatchString(raw) {
		parts := strings.Split(raw, ".")
		raw = parts[0] + "," + parts[1]
	} else if integerPricePattern.MatchString(raw) {
		raw += ",00"
	}
	m := brlPattern.FindStringSubmatch(raw)
	if m == nil {
		return 0, errInvalid
	}
	units, err := strconv.ParseInt(strings.ReplaceAll(m[1], ".", ""), 10, 64)
	if err != nil || units > math.MaxInt64/100 {
		return 0, errInvalid
	}
	fraction, _ := strconv.ParseInt(m[2], 10, 64)
	if units == math.MaxInt64/100 && fraction > math.MaxInt64%100 {
		return 0, errInvalid
	}
	return units*100 + fraction, nil
}
func Availability(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "em estoque", "disponível", "disponivel", "available", "in stock", "instock", "https://schema.org/instock", "http://schema.org/instock":
		return "available"
	case "indisponível", "indisponivel", "fora de estoque", "unavailable", "out of stock", "outofstock", "https://schema.org/outofstock", "http://schema.org/outofstock":
		return "unavailable"
	default:
		return "unknown"
	}
}
func PaymentMethod(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "à vista", "a vista", "cash", "pix", "boleto":
		return "cash"
	case "parcelado", "installment", "installments", "a prazo":
		return "installment"
	default:
		return ""
	}
}
