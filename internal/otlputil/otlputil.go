// Package otlputil contains the small OTLP conversion helpers shared by the
// ClickHouse and VictoriaMetrics writer adapters. It does not own schema
// decisions: callers pick the column or label layout they need.
package otlputil

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

// AttributeMap converts OTLP attributes into a flat string map. Non-scalar
// values are rendered as compact JSON so the result is deterministic and does
// not silently drop data.
func AttributeMap(attributes []*commonpb.KeyValue) map[string]string {
	if len(attributes) == 0 {
		return nil
	}
	out := make(map[string]string, len(attributes))
	for _, attribute := range attributes {
		if attribute == nil || attribute.GetKey() == "" {
			continue
		}
		out[attribute.GetKey()] = ValueString(attribute.GetValue())
	}
	return out
}

// ValueString renders an OTLP AnyValue as a string. Scalars use their natural
// representation; arrays and key/value lists use compact JSON; bytes use
// base64 so the value stays printable.
func ValueString(value *commonpb.AnyValue) string {
	if value == nil {
		return ""
	}
	switch typed := value.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return typed.StringValue
	case *commonpb.AnyValue_BoolValue:
		if typed.BoolValue {
			return "true"
		}
		return "false"
	case *commonpb.AnyValue_IntValue:
		return strconv.FormatInt(typed.IntValue, 10)
	case *commonpb.AnyValue_DoubleValue:
		return strconv.FormatFloat(typed.DoubleValue, 'g', -1, 64)
	case *commonpb.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(typed.BytesValue)
	case *commonpb.AnyValue_ArrayValue:
		values := make([]any, 0, len(typed.ArrayValue.GetValues()))
		for _, item := range typed.ArrayValue.GetValues() {
			values = append(values, plainValue(item))
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			return ""
		}
		return string(encoded)
	case *commonpb.AnyValue_KvlistValue:
		values := make(map[string]any, len(typed.KvlistValue.GetValues()))
		for _, item := range typed.KvlistValue.GetValues() {
			if item == nil || item.GetKey() == "" {
				continue
			}
			values[item.GetKey()] = plainValue(item.GetValue())
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			return ""
		}
		return string(encoded)
	default:
		return ""
	}
}

// plainValue converts an AnyValue into a JSON-marshalable value.
func plainValue(value *commonpb.AnyValue) any {
	if value == nil {
		return nil
	}
	switch typed := value.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return typed.StringValue
	case *commonpb.AnyValue_BoolValue:
		return typed.BoolValue
	case *commonpb.AnyValue_IntValue:
		return typed.IntValue
	case *commonpb.AnyValue_DoubleValue:
		return typed.DoubleValue
	case *commonpb.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(typed.BytesValue)
	case *commonpb.AnyValue_ArrayValue:
		values := make([]any, 0, len(typed.ArrayValue.GetValues()))
		for _, item := range typed.ArrayValue.GetValues() {
			values = append(values, plainValue(item))
		}
		return values
	case *commonpb.AnyValue_KvlistValue:
		values := make(map[string]any, len(typed.KvlistValue.GetValues()))
		for _, item := range typed.KvlistValue.GetValues() {
			if item == nil || item.GetKey() == "" {
				continue
			}
			values[item.GetKey()] = plainValue(item.GetValue())
		}
		return values
	default:
		return nil
	}
}

// ServiceName returns the resource's service.name attribute.
func ServiceName(resource *resourcepb.Resource) string {
	for _, attribute := range resource.GetAttributes() {
		if attribute.GetKey() == "service.name" {
			return ValueString(attribute.GetValue())
		}
	}
	return ""
}

// ResourceLabels converts resource attributes into metric label names and
// values. Dots and other characters that are not valid in a Prometheus label
// name become underscores; duplicate sanitized names keep the first value.
func ResourceLabels(resource *resourcepb.Resource) []Label {
	attributes := resource.GetAttributes()
	if len(attributes) == 0 {
		return nil
	}
	labels := make([]Label, 0, len(attributes))
	seen := make(map[string]struct{}, len(attributes))
	for _, attribute := range attributes {
		name := SanitizeLabelName(attribute.GetKey())
		if name == "" || name == "__name__" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		labels = append(labels, Label{Name: name, Value: ValueString(attribute.GetValue())})
	}
	return labels
}

// Label is a metric label pair. It mirrors the remote write label without
// forcing callers to depend on the codec package.
type Label struct {
	Name  string
	Value string
}

// SanitizeLabelName maps an OTLP attribute key to a valid Prometheus label
// name. Empty keys and the reserved __name__ label are rejected.
func SanitizeLabelName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(name))
	for index, r := range name {
		valid := r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || index > 0 && r >= '0' && r <= '9'
		if valid {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('_')
	}
	sanitized := builder.String()
	if sanitized == "" {
		return ""
	}
	first := sanitized[0]
	if first >= '0' && first <= '9' {
		sanitized = "_" + sanitized
	}
	return sanitized
}
