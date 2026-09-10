package openai

import "strings"

// SchemaCompatibility selects how tool JSON schemas are adapted before they are
// sent to an OpenAI-compatible API.
type SchemaCompatibility string

const (
	// SchemaCompatibilityDefault applies only safe normalization: schemas keep
	// all of their keywords, but an object schema without properties is sent as
	// "no parameters" and required entries that do not exist are dropped.
	SchemaCompatibilityDefault SchemaCompatibility = "default"

	// SchemaCompatibilityStrict additionally removes JSON Schema keywords that
	// stricter OpenAI-compatible endpoints reject. Gemini's OpenAI compatibility
	// layer, for example, answers 400 Bad Request when a function declaration
	// contains keywords such as additionalProperties or $schema, or when a
	// parameter object has no properties. See issue #23.
	SchemaCompatibilityStrict SchemaCompatibility = "strict"

	// SchemaCompatibilityNone sends schemas exactly as they were provided.
	SchemaCompatibilityNone SchemaCompatibility = "none"
)

// unsupportedSchemaKeywords are JSON Schema keywords that strict
// OpenAI-compatible endpoints (Gemini in particular) do not accept.
var unsupportedSchemaKeywords = []string{
	"$schema",
	"$id",
	"$ref",
	"$defs",
	"definitions",
	"additionalProperties",
	"patternProperties",
	"const",
	"default",
	"examples",
	"title",
	"exclusiveMinimum",
	"exclusiveMaximum",
	"multipleOf",
	"allOf",
	"oneOf",
	"not",
}

// supportedFormats are the "format" values accepted by strict endpoints.
var supportedFormats = map[string]bool{
	"date-time": true,
	"enum":      true,
	"int32":     true,
	"int64":     true,
	"float":     true,
	"double":    true,
}

// detectSchemaCompatibility derives the compatibility mode from a base URL, so
// that pointing the provider at a known-strict endpoint just works.
func detectSchemaCompatibility(baseURL string) SchemaCompatibility {
	if strings.Contains(strings.ToLower(baseURL), "generativelanguage.googleapis.com") {
		return SchemaCompatibilityStrict
	}
	return SchemaCompatibilityDefault
}

// adaptToolSchema prepares a tool parameter schema for the target API.
//
// It returns nil when the schema describes a function without parameters. The
// input schema is never modified; a copy is returned when changes are needed.
func adaptToolSchema(schema map[string]interface{}, mode SchemaCompatibility) map[string]interface{} {
	if mode == SchemaCompatibilityNone {
		return schema
	}
	if schema == nil {
		return nil
	}

	adapted := adaptSchemaNode(schema, mode)

	// A parameter object without properties is invalid on strict endpoints and
	// carries no information anywhere else: send the function without parameters.
	if isEmptyObjectSchema(adapted) {
		return nil
	}

	return adapted
}

// adaptSchemaNode recursively normalizes a single schema node.
func adaptSchemaNode(schema map[string]interface{}, mode SchemaCompatibility) map[string]interface{} {
	result := make(map[string]interface{}, len(schema))
	for key, value := range schema {
		result[key] = value
	}

	if mode == SchemaCompatibilityStrict {
		for _, keyword := range unsupportedSchemaKeywords {
			delete(result, keyword)
		}
		if format, ok := result["format"].(string); ok && !supportedFormats[format] {
			delete(result, "format")
		}
	}

	// Recurse into nested properties
	if properties, ok := toStringMap(result["properties"]); ok {
		adaptedProps := make(map[string]interface{}, len(properties))
		for name, prop := range properties {
			if propSchema, ok := toStringMap(prop); ok {
				adaptedProps[name] = adaptSchemaNode(propSchema, mode)
				continue
			}
			adaptedProps[name] = prop
		}
		result["properties"] = adaptedProps

		// An object schema must declare its type
		if _, ok := result["type"]; !ok {
			result["type"] = "object"
		}
	}

	// Recurse into array items
	if items, ok := toStringMap(result["items"]); ok {
		result["items"] = adaptSchemaNode(items, mode)
	}

	// Recurse into anyOf branches (supported by strict endpoints)
	if branches, ok := result["anyOf"].([]interface{}); ok {
		adaptedBranches := make([]interface{}, 0, len(branches))
		for _, branch := range branches {
			if branchSchema, ok := toStringMap(branch); ok {
				adaptedBranches = append(adaptedBranches, adaptSchemaNode(branchSchema, mode))
				continue
			}
			adaptedBranches = append(adaptedBranches, branch)
		}
		result["anyOf"] = adaptedBranches
	}

	normalizeRequired(result)

	return result
}

// normalizeRequired drops required entries that have no matching property and
// removes the keyword entirely when nothing is required. Both cases are
// rejected by strict endpoints and are meaningless for the model.
func normalizeRequired(schema map[string]interface{}) {
	required, ok := toStringSlice(schema["required"])
	if !ok {
		return
	}

	properties, _ := toStringMap(schema["properties"])

	filtered := make([]string, 0, len(required))
	for _, name := range required {
		if properties == nil {
			continue
		}
		if _, exists := properties[name]; exists {
			filtered = append(filtered, name)
		}
	}

	if len(filtered) == 0 {
		delete(schema, "required")
		return
	}
	schema["required"] = filtered
}

// isEmptyObjectSchema reports whether the schema describes an object without
// any properties.
func isEmptyObjectSchema(schema map[string]interface{}) bool {
	if schema == nil {
		return true
	}

	schemaType, _ := schema["type"].(string)
	if schemaType != "" && schemaType != "object" {
		return false
	}

	properties, ok := toStringMap(schema["properties"])
	if ok && len(properties) > 0 {
		return false
	}

	// Anything that carries no properties and no other structural keyword is an
	// empty object schema
	for key := range schema {
		switch key {
		case "type", "properties", "required", "description":
			continue
		default:
			return false
		}
	}

	return true
}

// toStringMap converts a value to map[string]interface{} when possible.
func toStringMap(value interface{}) (map[string]interface{}, bool) {
	m, ok := value.(map[string]interface{})
	return m, ok
}

// toStringSlice converts a []string or []interface{} of strings to []string.
func toStringSlice(value interface{}) ([]string, bool) {
	switch v := value.(type) {
	case []string:
		return v, true
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			result = append(result, s)
		}
		return result, true
	default:
		return nil, false
	}
}
