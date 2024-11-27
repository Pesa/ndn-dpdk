package iface

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/usnistgov/ndn-dpdk/core/jsonhelper"
	"go.uber.org/zap"
)

// Locator identifies the endpoints of a face.
type Locator interface {
	// Scheme returns a string that identifies the type of this Locator.
	// Possible values must be registered through RegisterLocatorScheme().
	Scheme() string

	// Validate checks whether Locator fields are correct according to the chosen scheme.
	Validate() error

	// CreateFace creates a face from this Locator.
	CreateFace() (Face, error)
}

// locatorWithSchemeField indicates that "scheme" field should be kept in JSON serialization.
type locatorWithSchemeField interface {
	Locator
	WithSchemeField()
}

var locatorTypes = map[string]reflect.Type{}

// RegisterLocatorScheme registers Locator schemes.
func RegisterLocatorScheme[T Locator](schemes ...string) {
	typ := reflect.TypeFor[T]()
	if typ.Kind() != reflect.Struct {
		logger.Panic("Locator must be a struct", zap.Stringer("type", typ))
	}
	for _, scheme := range schemes {
		locatorTypes[scheme] = typ
	}
}

// LocatorWrapper wraps Locator to facilitate JSON serialization.
type LocatorWrapper struct {
	Locator
}

// MarshalJSON implements json.Marshaler interface.
func (locw LocatorWrapper) MarshalJSON() (data []byte, e error) {
	var kv map[string]any
	e = jsonhelper.Roundtrip(locw.Locator, &kv)
	if e != nil {
		return nil, e
	}
	if _, ok := kv["scheme"]; !ok {
		kv["scheme"] = locw.Scheme()
	}
	return json.Marshal(kv)
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (locw *LocatorWrapper) UnmarshalJSON(data []byte) error {
	var kv map[string]any
	if e := json.Unmarshal(data, &kv); e != nil {
		return e
	}
	scheme, _ := kv["scheme"].(string)

	typ, ok := locatorTypes[scheme]
	if !ok {
		return fmt.Errorf("unknown scheme %s", scheme)
	}

	ptr := reflect.New(typ)
	if _, keepSchemeField := ptr.Elem().Interface().(locatorWithSchemeField); !keepSchemeField {
		delete(kv, "scheme")
	}
	if e := jsonhelper.Roundtrip(kv, ptr.Interface(), jsonhelper.DisallowUnknownFields); e != nil {
		return e
	}

	loc := ptr.Elem().Interface().(Locator)
	if e := loc.Validate(); e != nil {
		return e
	}

	locw.Locator = loc
	return nil
}

// LocatorString converts a locator to JSON string.
func LocatorString(loc Locator) string {
	locw := LocatorWrapper{Locator: loc}
	j, _ := json.Marshal(locw)
	return string(j)
}
