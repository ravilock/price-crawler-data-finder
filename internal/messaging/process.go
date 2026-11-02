package messaging

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	"price-crawler/services/data-finder/internal/normalize"
)

const maxEnvelope = 1 << 20

var uuid = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type Envelope struct {
	MessageID      string          `json:"message_id"`
	MessageType    string          `json:"message_type"`
	SchemaVersion  int             `json:"schema_version"`
	OccurredAt     time.Time       `json:"occurred_at"`
	OrganizationID string          `json:"organization_id"`
	CorrelationID  string          `json:"correlation_id"`
	CausationID    string          `json:"causation_id,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	Payload        json.RawMessage `json:"payload"`
}
type PageField struct {
	Name        string  `json:"name"`
	RawValue    *string `json:"raw_value"`
	EvidenceRef string  `json:"evidence_ref"`
	SourceKind  string  `json:"source_kind,omitempty"`
}
type metadata struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type Page struct {
	CrawlRequestID  string           `json:"crawl_request_id"`
	RequestedURL    string           `json:"requested_url"`
	FinalURL        string           `json:"final_url"`
	Outcome         string           `json:"outcome"`
	ObservedAt      time.Time        `json:"observed_at"`
	ParserVersion   string           `json:"parser_version"`
	HTTPStatus      *int             `json:"http_status,omitempty"`
	ContentType     *string          `json:"content_type,omitempty"`
	PageTitle       *string          `json:"page_title,omitempty"`
	PageDescription *string          `json:"page_description,omitempty"`
	StructuredData  []map[string]any `json:"structured_data,omitempty"`
	Metadata        []metadata       `json:"metadata,omitempty"`
	Fields          []PageField      `json:"fields"`
}
type request struct {
	CrawlRequestID       string `json:"crawl_request_id"`
	PageParsedMessageID  string `json:"page_parsed_message_id"`
	NormalizationProfile string `json:"normalization_profile"`
	Page                 Page   `json:"page"`
}

func decode(raw []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON")
	}
	return nil
}
func newID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	id[6] = (id[6] & 15) | 64
	id[8] = (id[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:]), nil
}

// Process never fetches URLs; it validates and normalizes a single bounded page.
func Process(raw []byte) (Envelope, error) {
	if len(raw) == 0 || len(raw) > maxEnvelope {
		return Envelope{}, errors.New("invalid envelope size")
	}
	var e Envelope
	if err := decode(raw, &e); err != nil {
		return Envelope{}, err
	}
	if !uuid.MatchString(e.MessageID) || !uuid.MatchString(e.OrganizationID) || !uuid.MatchString(e.CorrelationID) || e.SchemaVersion != 1 || e.MessageType != "product.find.requested" || e.OccurredAt.IsZero() || len(e.IdempotencyKey) > 200 || (e.CausationID != "" && !uuid.MatchString(e.CausationID)) {
		return Envelope{}, errors.New("invalid envelope")
	}
	var presence map[string]json.RawMessage
	if json.Unmarshal(e.Payload, &presence) != nil {
		return Envelope{}, errors.New("invalid payload")
	}
	for _, key := range []string{"crawl_request_id", "page_parsed_message_id", "normalization_profile", "page"} {
		if _, ok := presence[key]; !ok {
			return Envelope{}, errors.New("missing payload field")
		}
	}
	var pagePresence map[string]json.RawMessage
	if json.Unmarshal(presence["page"], &pagePresence) != nil {
		return Envelope{}, errors.New("invalid page")
	}
	for _, key := range []string{"crawl_request_id", "requested_url", "final_url", "outcome", "observed_at", "parser_version", "fields"} {
		if _, ok := pagePresence[key]; !ok {
			return Envelope{}, errors.New("missing page field")
		}
	}
	var fieldsPresence []map[string]json.RawMessage
	if json.Unmarshal(pagePresence["fields"], &fieldsPresence) != nil || fieldsPresence == nil {
		return Envelope{}, errors.New("invalid page fields")
	}
	for _, item := range fieldsPresence {
		for _, key := range []string{"name", "raw_value", "evidence_ref"} {
			if _, ok := item[key]; !ok {
				return Envelope{}, errors.New("missing page field property")
			}
		}
	}
	var r request
	if err := decode(e.Payload, &r); err != nil {
		return Envelope{}, err
	}
	p := r.Page
	if !uuid.MatchString(r.CrawlRequestID) || !uuid.MatchString(r.PageParsedMessageID) || r.CrawlRequestID != p.CrawlRequestID || len(r.NormalizationProfile) > 100 || p.ObservedAt.IsZero() || p.ParserVersion == "" || len(p.ParserVersion) > 100 || p.Fields == nil || len(p.Fields) > 500 || len(p.Metadata) > 200 || len(p.StructuredData) > 100 || (p.Outcome != "complete" && p.Outcome != "partial" && p.Outcome != "blocked" && p.Outcome != "unsupported" && p.Outcome != "failed") {
		return Envelope{}, errors.New("invalid page")
	}
	if p.HTTPStatus != nil && (*p.HTTPStatus < 100 || *p.HTTPStatus > 599) || p.ContentType != nil && len(*p.ContentType) > 200 {
		return Envelope{}, errors.New("invalid response metadata")
	}
	for _, m := range p.Metadata {
		if len(m.Name) > 200 || len(m.Value) > 5000 {
			return Envelope{}, errors.New("invalid metadata")
		}
	}
	for _, rawURL := range []string{p.RequestedURL, p.FinalURL} {
		u, err := url.Parse(rawURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return Envelope{}, errors.New("invalid page URL")
		}
	}
	input := normalize.Input{CrawlRequestID: r.CrawlRequestID, Fields: map[string]normalize.Evidence{}}
	if p.PageTitle != nil {
		if len(*p.PageTitle) > 1000 {
			return Envelope{}, errors.New("title too long")
		}
		input.Fields["title"] = normalize.Evidence{Raw: *p.PageTitle, Locator: "page_title"}
	}
	if p.PageDescription != nil {
		if len(*p.PageDescription) > 5000 {
			return Envelope{}, errors.New("description too long")
		}
		input.Fields["description"] = normalize.Evidence{Raw: *p.PageDescription, Locator: "page_description"}
	}
	for _, m := range p.Metadata {
		item := normalize.Evidence{Raw: m.Value, Locator: "meta:" + m.Name}
		switch strings.ToLower(m.Name) {
		case "og:title":
			if _, ok := input.Fields["title"]; !ok {
				input.Fields["title"] = item
			}
		case "og:description":
			if _, ok := input.Fields["description"]; !ok {
				input.Fields["description"] = item
			}
		case "product:price:amount":
			if input.Price.Raw == "" {
				input.Price = item
			}
		case "product:price:currency":
			if input.Currency.Raw == "" {
				input.Currency = item
			}
		case "product:availability":
			if input.Availability.Raw == "" {
				input.Availability = item
			}
		}
	}
	for index, object := range p.StructuredData {
		loc := fmt.Sprintf("structured_data[%d]", index)
		textField := func(key string) normalize.Evidence {
			value, _ := object[key].(string)
			return normalize.Evidence{Raw: value, Locator: loc + "." + key}
		}
		for _, mapping := range [][2]string{{"name", "title"}, {"description", "description"}, {"gtin", "gtin"}, {"gtin8", "gtin"}, {"gtin12", "gtin"}, {"gtin13", "gtin"}, {"gtin14", "gtin"}, {"mpn", "manufacturer_model"}} {
			item := textField(mapping[0])
			if item.Raw != "" {
				if _, ok := input.Fields[mapping[1]]; !ok {
					input.Fields[mapping[1]] = item
				}
			}
		}

		brand := textField("brand")
		if nested, ok := object["brand"].(map[string]any); ok {
			if name, ok := nested["name"].(string); ok {
				brand.Raw = name
				brand.Locator += ".name"
			}
		}
		if brand.Raw != "" {
			if _, ok := input.Fields["brand"]; !ok {
				input.Fields["brand"] = brand
			}
		}
		if offer, ok := object["offers"].(map[string]any); ok {
			if currency, ok := offer["priceCurrency"].(string); ok && input.Currency.Raw == "" {
				input.Currency = normalize.Evidence{Raw: currency, Locator: loc + ".offers.priceCurrency"}
			}
			if value, ok := offer["price"].(json.Number); ok && input.Price.Raw == "" {
				input.Price = normalize.Evidence{Raw: value.String(), Locator: loc + ".offers.price"}
			}
			if value, ok := offer["price"].(string); ok && input.Price.Raw == "" {
				input.Price = normalize.Evidence{Raw: value, Locator: loc + ".offers.price"}
			}
			if value, ok := offer["availability"].(string); ok && input.Availability.Raw == "" {
				input.Availability = normalize.Evidence{Raw: value, Locator: loc + ".offers.availability"}
			}
		}
	}
	for _, f := range p.Fields {
		if len(f.Name) > 200 || len(f.EvidenceRef) > 500 || (f.RawValue != nil && len(*f.RawValue) > 10000) || (f.SourceKind != "" && f.SourceKind != "json_ld" && f.SourceKind != "meta" && f.SourceKind != "dom" && f.SourceKind != "text") {
			return Envelope{}, errors.New("invalid page field")
		}
		if f.RawValue == nil {
			continue
		}
		item := normalize.Evidence{Raw: *f.RawValue, Locator: f.EvidenceRef}
		switch f.Name {
		case "title", "description", "brand", "manufacturer_model", "gtin":
			if _, exists := input.Fields[f.Name]; !exists {
				input.Fields[f.Name] = item
			}
		case "price":
			input.Price = item
		case "currency":
			input.Currency = item
		case "payment_method":
			input.Payment = item
		case "availability":
			input.Availability = item
		default:
			input.Attributes = append(input.Attributes, normalize.InputAttribute{Name: f.Name, Evidence: item})
		}
	}
	output := normalize.Normalize(input)
	payload, err := json.Marshal(output)
	if err != nil {
		return Envelope{}, err
	}
	id, err := newID()
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{MessageID: id, MessageType: "product.information.found", SchemaVersion: 1, OccurredAt: time.Now().UTC(), OrganizationID: e.OrganizationID, CorrelationID: e.CorrelationID, CausationID: e.MessageID, Payload: payload}, nil
}
