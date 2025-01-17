package selector

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/flightctl/flightctl/internal/flterrors"
	"github.com/samber/lo"
	gormschema "gorm.io/gorm/schema"
)

var (
	cacheStore           = &sync.Map{}
	schemaTypeResolution = map[gormschema.DataType]SelectorFieldType{
		gormschema.Bool:   Bool,
		gormschema.Int:    Int,
		gormschema.Float:  Float,
		gormschema.String: String,
		gormschema.Time:   Time,
		"boolean[]":       BoolArray,
		"integer[]":       IntArray,
		"smallint[]":      SmallIntArray,
		"bigint[]":        BigIntArray,
		"real[]":          FloatArray,
		"text[]":          TextArray,
		"timestamp[]":     TimestampArray,
		"jsonb":           Jsonb,
	}

	castTypeResolution = map[string]SelectorFieldType{
		"boolean":   Bool,
		"integer":   Int,
		"smallint":  SmallInt,
		"bigInt":    BigInt,
		"float":     Float,
		"timestamp": Time,
		"string":    String,
	}
)

// FieldNameResolver defines an interface for resolving custom field name mappings.
// This is useful for advanced cases where certain fields map to other names
// or when dealing with complex schemas that require custom resolution logic.
type FieldNameResolver interface {
	// ResolveCustomSelector resolves a custom selector name to a slice of selector names
	// that correspond to actual fields in the model. This allows for mapping of selectors
	// to their respective fields, enabling more dynamic queries.
	ResolveCustomSelector(selector SelectorFieldName) []SelectorFieldName

	// ListCustomSelectors returns a list of custom selectors that can be resolved by the implementing model.
	ListCustomSelectors() []SelectorFieldName
}

// selectorFieldResolver is a struct that provides the ability to resolve selector fields to
// their corresponding schema fields. It holds a map of schema fields and optionally a field
// resolver for advanced cases.
type selectorFieldResolver struct {
	schemaFields  map[SelectorFieldName]*gormschema.Field
	fieldResolver FieldNameResolver
}

// SelectorFieldResolver initializes a new selectorFieldResolver. It resolves schema fields from the provided model.
// If the model implements FieldNameResolver, it will be used to resolve custom field names.
func SelectorFieldResolver(model any) (*selectorFieldResolver, error) {
	resolved, err := ResolveFieldsFromSchema(model)
	if err != nil {
		return nil, err
	}

	fr := &selectorFieldResolver{schemaFields: resolved}
	if resolver, ok := model.(FieldNameResolver); ok {
		fr.fieldResolver = resolver
	}
	return fr, nil
}

// ResolveNames maps a selector field name to its corresponding database field names.
// See ResolveFields for more details on the resolution process.
func (sr *selectorFieldResolver) ResolveNames(field SelectorFieldName) ([]string, error) {
	resolvedFields, err := sr.ResolveFields(field)
	if err != nil {
		return nil, err
	}

	fields := make([]string, 0, len(resolvedFields))
	for _, selectorField := range resolvedFields {
		fields = append(fields, selectorField.DBName)
	}
	return fields, nil
}

// ResolveFields resolves a selector field name to its corresponding schema field(s).
// It supports resolving JSONB fields and custom field resolutions if a fieldResolver is present.
// It returns a slice of resolved SelectorField or an error if the field cannot be resolved.
func (sr *selectorFieldResolver) ResolveFields(field SelectorFieldName) ([]*SelectorField, error) {
	resolve := func(fn SelectorFieldName) (*SelectorField, error) {
		if resolvedField, exists := sr.schemaFields[fn]; exists {
			fieldType, ok := schemaTypeResolution[resolvedField.DataType]
			if !ok {
				return nil, fmt.Errorf("unknown or unsupported schema type for field: %s", fn)
			}

			if fieldType.IsArray() {
				fieldKind := resolvedField.StructField.Type.Kind()
				if fieldKind != reflect.Array && fieldKind != reflect.Slice {
					return nil, fmt.Errorf("field %s is expected to be an array or slice, but got %s", fn, fieldKind.String())
				}
			}

			return &SelectorField{
				DBName:      resolvedField.DBName,
				Type:        fieldType,
				DataType:    resolvedField.DataType,
				StructField: resolvedField.StructField,
			}, nil
		}

		// Handle JSONB fields
		jsonbField := strings.TrimSpace(string(fn))
		fieldParts := strings.Split(jsonbField, ".")
		if len(fieldParts) > 1 {
			// Iterate through schema fields to find the matching JSONB field
			for _, schemaField := range sr.schemaFields {
				if schemaField.DataType == "jsonb" && schemaField.DBName == fieldParts[0] {
					// Check if there's a "::" in the JSONB field name
					if strings.Contains(jsonbField, "::") {
						parts := strings.Split(jsonbField, "::")
						if len(parts) != 2 {
							return nil, fmt.Errorf("invalid field format: %s", jsonbField)
						}
						fieldName := parts[0]
						suffix := parts[1]

						// Check if the suffix exists in the type resolutions map
						if fieldType, ok := castTypeResolution[suffix]; ok {
							return &SelectorField{
								DBName:      fieldName,
								Type:        fieldType,
								DataType:    schemaField.DataType,
								StructField: schemaField.StructField,
							}, nil
						} else {
							return nil, fmt.Errorf("unknown or unsupported suffix %q for field %q. Expect: %v",
								suffix, schemaField.DBName, lo.MapToSlice(castTypeResolution,
									func(k string, v SelectorFieldType) string { return k }))
						}
					}

					// Original logic if no "::" is present
					return &SelectorField{
						DBName:      jsonbField,
						Type:        Jsonb,
						DataType:    schemaField.DataType,
						StructField: schemaField.StructField,
					}, nil
				}
			}
		}
		return nil, nil
	}

	resolvedField, err := resolve(field)
	if err != nil {
		return nil, sr.newUnsupportedFieldError(field, err)
	}

	if resolvedField != nil {
		return []*SelectorField{resolvedField}, nil
	}

	if sr.fieldResolver != nil {
		refs := sr.fieldResolver.ResolveCustomSelector(field)
		if len(refs) > 0 {
			fields := make([]*SelectorField, 0, len(refs))
			for _, ref := range refs {
				resolvedField, err := resolve(ref)
				if err != nil {
					return nil, sr.newUnsupportedFieldError(ref, err)
				}

				if resolvedField == nil {
					return nil, sr.newUnsupportedFieldError(ref, nil)
				}
				fields = append(fields, resolvedField)
			}
			return fields, nil
		}
	}

	return nil, sr.newUnsupportedFieldError(field, nil)
}

// ListFields returns a list of all schema fields managed by the selectorFieldResolver.
// If there are no fields, it returns nil.
func (sr *selectorFieldResolver) ListFields() []*gormschema.Field {
	if len(sr.schemaFields) == 0 {
		return nil
	}

	fields := make([]*gormschema.Field, 0, len(sr.schemaFields))
	for _, field := range sr.schemaFields {
		fields = append(fields, field)
	}

	return fields
}

// ListSelectors returns a list of all selector field names managed by the selectorFieldResolver.
// If there are no selector fields, it returns nil.
func (sr *selectorFieldResolver) ListSelectors() []SelectorFieldName {
	if len(sr.schemaFields) == 0 {
		return nil
	}

	selectors := make([]SelectorFieldName, 0, len(sr.schemaFields))
	for selector := range sr.schemaFields {
		selectors = append(selectors, selector)
	}

	return selectors
}

// newUnsupportedFieldError creates a new SelectorError indicating an unsupported field,
// and includes a list of all supported selector fields only if no prior error is provided.
func (sr *selectorFieldResolver) newUnsupportedFieldError(field SelectorFieldName, err error) error {
	if err == nil {
		supportedFields := sr.ListSelectors()

		if sr.fieldResolver != nil {
			supportedFields = append(supportedFields, sr.fieldResolver.ListCustomSelectors()...)
		}

		if len(supportedFields) == 0 {
			supportedFields = []SelectorFieldName{}
		}

		return NewSelectorError(flterrors.ErrFieldSelectorUnknownField,
			fmt.Errorf("unable to resolve field name %q. Supported fields are: %v", field, supportedFields))
	}

	return NewSelectorError(flterrors.ErrFieldSelectorUnknownField,
		fmt.Errorf("unable to resolve field name %q: %w", field, err))
}

// ResolveFieldsFromSchema parses the schema of the given model and extracts the fields annotated with
// the `selector` tag. This is useful for determining which fields can be used in selector queries.
func ResolveFieldsFromSchema(dest any) (map[SelectorFieldName]*gormschema.Field, error) {
	schema, err := gormschema.ParseWithSpecialTableName(dest, cacheStore, gormschema.NamingStrategy{IdentifierMaxLength: 64}, "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse schema: %w", err)
	}

	fieldMap := make(map[SelectorFieldName]*gormschema.Field)
	for _, field := range schema.Fields {
		if selector := field.StructField.Tag.Get("selector"); selector != "" {
			fieldMap[SelectorFieldName(selector)] = field
		}
	}
	return fieldMap, nil
}

// SelectorError represents an error related to a selector, wrapping another error.
type SelectorError struct {
	SelectorError error
	OriginalError error
}

// NewSelectorError creates a new SelectorError.
func NewSelectorError(selectorError, originalError error) *SelectorError {
	return &SelectorError{
		SelectorError: selectorError,
		OriginalError: originalError,
	}
}

// Error returns the string representation of the SelectorError.
func (e *SelectorError) Error() string {
	return fmt.Sprintf("%s: %v", e.SelectorError.Error(), e.OriginalError)
}

// Unwrap returns the original error.
func (e *SelectorError) Unwrap() error {
	return e.OriginalError
}

// AsSelectorError checks if an error is of type SelectorError and assigns it to target.
func AsSelectorError(err error, target any) bool {
	if target == nil {
		return false
	}
	// Ensure target is a pointer to the correct type
	switch t := target.(type) {
	case **SelectorError:
		return errors.As(err, t)
	default:
		return false
	}
}

// IsSelectorError checks if an error is of type SelectorError.
func IsSelectorError(err error) bool {
	var selectorErr *SelectorError
	return errors.As(err, &selectorErr)
}
