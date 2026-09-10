package tool

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// FunctionTool is a tool implemented as a Go function
type FunctionTool struct {
	name        string
	description string
	function    interface{}
	schema      map[string]interface{}
}

// NewFunctionTool creates a new function tool
func NewFunctionTool(name, description string, fn interface{}) *FunctionTool {
	// Validate that fn is a function
	fnType := reflect.TypeOf(fn)
	if fnType.Kind() != reflect.Func {
		panic("function tool must be a function")
	}

	// Generate schema from function signature
	schema := generateSchemaFromFunction(fnType)

	return &FunctionTool{
		name:        name,
		description: description,
		function:    fn,
		schema:      schema,
	}
}

// GetName returns the name of the tool
func (t *FunctionTool) GetName() string {
	return t.name
}

// GetDescription returns the description of the tool
func (t *FunctionTool) GetDescription() string {
	return t.description
}

// GetParametersSchema returns the JSON schema for the tool parameters
func (t *FunctionTool) GetParametersSchema() map[string]interface{} {
	return t.schema
}

// Execute executes the tool with the given parameters
func (t *FunctionTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	fnType := reflect.TypeOf(t.function)
	fnValue := reflect.ValueOf(t.function)

	// Check if the function accepts a context as the first parameter
	hasContext := fnType.NumIn() > 0 && fnType.In(0).Implements(reflect.TypeOf((*context.Context)(nil)).Elem())

	// Prepare arguments
	args := make([]reflect.Value, fnType.NumIn())

	// Set context if the function accepts it
	argIndex := 0
	if hasContext {
		args[0] = reflect.ValueOf(ctx)
		argIndex = 1
	}

	// Set parameters based on function signature
	for i := argIndex; i < fnType.NumIn(); i++ {
		paramType := fnType.In(i)

		// If the function expects a map[string]interface{} directly
		if i == argIndex && paramType.Kind() == reflect.Map &&
			paramType.Key().Kind() == reflect.String &&
			paramType.Elem().Kind() == reflect.Interface {
			args[i] = reflect.ValueOf(params)
			continue
		}

		// Handle struct parameter - map params to struct fields
		if paramType.Kind() == reflect.Struct {
			structValue := reflect.New(paramType).Elem()

			// For each field in the struct, check if we have a corresponding parameter
			for j := 0; j < paramType.NumField(); j++ {
				field := paramType.Field(j)

				// Get the JSON tag if available
				jsonTag := field.Tag.Get("json")
				if jsonTag == "" {
					jsonTag = field.Name
				} else {
					// Handle json tag options like `json:"name,omitempty"`
					parts := strings.Split(jsonTag, ",")
					jsonTag = parts[0]
				}

				// Check if we have a parameter with this name
				if paramValue, ok := params[jsonTag]; ok {
					// Try to set the field
					fieldValue := structValue.Field(j)
					if fieldValue.CanSet() {
						// Convert the parameter value to the field type
						convertedValue, err := convertToType(paramValue, field.Type)
						if err != nil {
							return nil, fmt.Errorf("failed to convert parameter %s: %w", jsonTag, err)
						}

						fieldValue.Set(reflect.ValueOf(convertedValue))
					}
				}
			}

			args[i] = structValue
			continue
		}

		// For a single parameter function with a primitive type, try to use the first parameter or a parameter with the same name
		paramName := ""
		// Only try to access struct fields if the parameter type is a struct
		if paramType.Kind() == reflect.Struct {
			for j := 0; j < paramType.NumField(); j++ {
				field := paramType.Field(j)
				jsonTag := field.Tag.Get("json")
				if jsonTag != "" {
					parts := strings.Split(jsonTag, ",")
					jsonTag = parts[0]
					if _, ok := params[jsonTag]; ok {
						paramName = jsonTag
						break
					}
				}
			}
		}

		if paramName == "" && len(params) > 0 {
			// Just use the first parameter
			for name := range params {
				paramName = name
				break
			}
		}

		if paramName != "" {
			if paramValue, ok := params[paramName]; ok {
				// Try to convert the parameter value to the expected type
				convertedValue, err := convertToType(paramValue, paramType)
				if err != nil {
					return nil, fmt.Errorf("failed to convert parameter %s: %w", paramName, err)
				}

				args[i] = reflect.ValueOf(convertedValue)
				continue
			}
		}

		// If we couldn't find a parameter, use the zero value for the type
		args[i] = reflect.Zero(paramType)
	}

	// Call the function
	results := fnValue.Call(args)

	// Handle return values
	if len(results) == 0 {
		return nil, nil
	} else if len(results) == 1 {
		return results[0].Interface(), nil
	} else {
		// Assume the last result is an error
		errVal := results[len(results)-1]
		if errVal.IsNil() {
			return results[0].Interface(), nil
		}
		return results[0].Interface(), errVal.Interface().(error)
	}
}

// convertToType attempts to convert a value to the specified type
func convertToType(value interface{}, targetType reflect.Type) (interface{}, error) {
	// Handle nil special case
	if value == nil {
		return reflect.Zero(targetType).Interface(), nil
	}

	// Get the value's type
	valueType := reflect.TypeOf(value)

	// If the value is already assignable to the target type, return it
	if valueType.AssignableTo(targetType) {
		return value, nil
	}

	// Handle some common conversions
	switch targetType.Kind() {
	case reflect.String:
		// Convert to string
		return fmt.Sprintf("%v", value), nil

	case reflect.Bool:
		// Try to convert to bool
		switch v := value.(type) {
		case bool:
			return v, nil
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
			return reflect.ValueOf(v).Int() != 0, nil
		case string:
			b, err := strconv.ParseBool(v)
			if err != nil {
				return false, fmt.Errorf("cannot convert %v to bool: %w", value, err)
			}
			return b, nil
		default:
			return false, fmt.Errorf("cannot convert %v to bool", value)
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Try to convert to int
		switch v := value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			intVal := reflect.ValueOf(v).Int()
			return reflect.ValueOf(intVal).Convert(targetType).Interface(), nil
		case float32, float64:
			floatVal := reflect.ValueOf(v).Float()
			return reflect.ValueOf(int64(floatVal)).Convert(targetType).Interface(), nil
		case string:
			i, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("cannot convert %v to int: %w", value, err)
			}
			return reflect.ValueOf(i).Convert(targetType).Interface(), nil
		default:
			return 0, fmt.Errorf("cannot convert %v to int", value)
		}

	case reflect.Float32, reflect.Float64:
		// Try to convert to float
		switch v := value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			intVal := reflect.ValueOf(v).Int()
			return reflect.ValueOf(float64(intVal)).Convert(targetType).Interface(), nil
		case float32, float64:
			floatVal := reflect.ValueOf(v).Float()
			return reflect.ValueOf(floatVal).Convert(targetType).Interface(), nil
		case string:
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return 0.0, fmt.Errorf("cannot convert %v to float: %w", value, err)
			}
			return reflect.ValueOf(f).Convert(targetType).Interface(), nil
		default:
			return 0.0, fmt.Errorf("cannot convert %v to float", value)
		}

	case reflect.Slice:
		// Try to convert to slice
		switch v := value.(type) {
		case []interface{}:
			elemType := targetType.Elem()
			sliceValue := reflect.MakeSlice(targetType, len(v), len(v))

			for i, elem := range v {
				convertedElem, err := convertToType(elem, elemType)
				if err != nil {
					return nil, fmt.Errorf("cannot convert slice element %d: %w", i, err)
				}
				sliceValue.Index(i).Set(reflect.ValueOf(convertedElem))
			}

			return sliceValue.Interface(), nil
		default:
			return nil, fmt.Errorf("cannot convert %v to slice", value)
		}

	case reflect.Map:
		// Try to convert to map
		if targetType.Key().Kind() == reflect.String {
			switch v := value.(type) {
			case map[string]interface{}:
				elemType := targetType.Elem()
				mapValue := reflect.MakeMap(targetType)

				for key, elem := range v {
					convertedElem, err := convertToType(elem, elemType)
					if err != nil {
						return nil, fmt.Errorf("cannot convert map element %s: %w", key, err)
					}
					mapValue.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(convertedElem))
				}

				return mapValue.Interface(), nil
			default:
				return nil, fmt.Errorf("cannot convert %v to map", value)
			}
		}
	}

	// If we couldn't convert, return an error
	return nil, fmt.Errorf("cannot convert %v (type %T) to %v", value, value, targetType)
}

// generateSchemaFromFunction generates a JSON schema from a function signature
func generateSchemaFromFunction(fnType reflect.Type) map[string]interface{} {
	// Initialize schema
	schema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
		"required":   []string{},
	}

	// Check if the function accepts a context as the first parameter
	hasContext := fnType.NumIn() > 0 && fnType.In(0).Implements(reflect.TypeOf((*context.Context)(nil)).Elem())

	// Start from the first non-context parameter
	startIndex := 0
	if hasContext {
		startIndex = 1
	}

	// If the function has no parameters beyond context, return empty schema
	if fnType.NumIn() <= startIndex {
		return schema
	}

	// Get the first parameter type after context (if any)
	paramType := fnType.In(startIndex)

	// If the parameter is a map[string]interface{}, we can't infer the schema
	if paramType.Kind() == reflect.Map &&
		paramType.Key().Kind() == reflect.String &&
		paramType.Elem().Kind() == reflect.Interface {
		// Generic map, can't infer schema
		return schema
	}

	// If the parameter is a struct, create a schema from its fields
	if paramType.Kind() == reflect.Struct {
		for i := 0; i < paramType.NumField(); i++ {
			field := paramType.Field(i)

			// Skip unexported fields
			if field.PkgPath != "" {
				continue
			}

			// Get the field name from JSON tag or fallback to field name
			fieldName := field.Name
			jsonTag := field.Tag.Get("json")
			if jsonTag != "" {
				// Handle json tag options like `json:"name,omitempty"`
				parts := strings.Split(jsonTag, ",")
				fieldName = parts[0]

				// Skip if the field is explicitly omitted with "-"
				if fieldName == "-" {
					continue
				}

				// Check if the field is required (not marked as omitempty)
				isRequired := true
				for _, part := range parts[1:] {
					if part == "omitempty" {
						isRequired = false
						break
					}
				}

				if isRequired {
					schema["required"] = append(schema["required"].([]string), fieldName)
				}
			} else {
				// If no JSON tag, assume it's required
				schema["required"] = append(schema["required"].([]string), fieldName)
			}

			// Get the field schema
			fieldSchema := getTypeSchema(field.Type)

			// Add description from doc tag if available
			if docTag := field.Tag.Get("doc"); docTag != "" {
				fieldSchema["description"] = docTag
			}

			// Add the field to properties
			schema["properties"].(map[string]interface{})[fieldName] = fieldSchema
		}
	} else {
		// For other parameter types, create a single property schema
		propName := "value"
		propSchema := getTypeSchema(paramType)
		schema["properties"].(map[string]interface{})[propName] = propSchema
		schema["required"] = append(schema["required"].([]string), propName)
	}

	return schema
}

// getTypeSchema returns the JSON schema for a Go type
func getTypeSchema(t reflect.Type) map[string]interface{} {
	schema := make(map[string]interface{})

	// Handle pointers
	if t.Kind() == reflect.Pointer {
		elemSchema := getTypeSchema(t.Elem())

		// For pointers, the field is nullable
		if enum, ok := elemSchema["enum"]; ok {
			// If the schema has enum values, add null to the enum
			enumValues := enum.([]interface{})
			enumValues = append(enumValues, nil)
			elemSchema["enum"] = enumValues
		}

		return elemSchema
	}

	// Handle different types
	switch t.Kind() {
	case reflect.Bool:
		schema["type"] = "boolean"

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema["type"] = "integer"

	case reflect.Float32, reflect.Float64:
		schema["type"] = "number"

	case reflect.String:
		schema["type"] = "string"

	case reflect.Slice, reflect.Array:
		schema["type"] = "array"
		schema["items"] = getTypeSchema(t.Elem())

	case reflect.Map:
		schema["type"] = "object"
		if t.Key().Kind() == reflect.String {
			schema["additionalProperties"] = getTypeSchema(t.Elem())
		} else {
			// Non-string keyed maps are not well represented in JSON Schema
			schema["additionalProperties"] = true
		}

	case reflect.Struct:
		schema["type"] = "object"
		schema["properties"] = make(map[string]interface{})
		schema["required"] = []string{}

		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)

			// Skip unexported fields
			if field.PkgPath != "" {
				continue
			}

			// Get the field name from JSON tag or fallback to field name
			fieldName := field.Name
			jsonTag := field.Tag.Get("json")
			if jsonTag != "" {
				// Handle json tag options like `json:"name,omitempty"`
				parts := strings.Split(jsonTag, ",")
				fieldName = parts[0]

				// Skip if the field is explicitly omitted with "-"
				if fieldName == "-" {
					continue
				}

				// Check if the field is required (not marked as omitempty)
				isRequired := true
				for _, part := range parts[1:] {
					if part == "omitempty" {
						isRequired = false
						break
					}
				}

				if isRequired {
					schema["required"] = append(schema["required"].([]string), fieldName)
				}
			} else {
				// If no JSON tag, assume it's required
				schema["required"] = append(schema["required"].([]string), fieldName)
			}

			// Get the field schema
			fieldSchema := getTypeSchema(field.Type)

			// Add description from doc tag if available
			if docTag := field.Tag.Get("doc"); docTag != "" {
				fieldSchema["description"] = docTag
			}

			// Add the field to properties
			schema["properties"].(map[string]interface{})[fieldName] = fieldSchema
		}

	default:
		// For unknown types, fallback to string
		schema["type"] = "string"
	}

	return schema
}

// WithSchema sets a custom schema for the tool parameters
func (t *FunctionTool) WithSchema(schema map[string]interface{}) *FunctionTool {
	t.schema = schema
	return t
}

// WithDescription updates the description of the tool
func (t *FunctionTool) WithDescription(description string) *FunctionTool {
	t.description = description
	return t
}

// WithName updates the name of the tool
func (t *FunctionTool) WithName(name string) *FunctionTool {
	t.name = name
	return t
}
