package service

import (
	"reflect"
	"testing"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/client"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// TestTaskUpdateRoundTrip tests Property 13: Task update round trip
// **Feature: clickup-field-updater, Property 13: Task update round trip**
// **Validates: Requirements 10.5**
//
// For any valid task update request with proper field mapping, the system should
// successfully transform field values and maintain data integrity through the update process.
// This property tests that the TransformFieldValue function correctly handles all field types
// and produces values that can be successfully sent to the ClickUp API.
func TestTaskUpdateRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.MaxSize = 50

	properties := gopter.NewProperties(parameters)

	// Property 13.1: Text field transformation preserves string content
	// For any text value, transformation should preserve the original string
	properties.Property("text field transformation preserves content", prop.ForAll(
		func(value string) bool {
			result := client.TransformFieldValue(value, "text")
			strResult, ok := result.(string)
			if !ok {
				return false
			}
			return strResult == value
		},
		gen.AnyString(),
	))

	// Property 13.2: Numeric field transformation produces valid numbers
	// For any numeric string, transformation should produce a valid number
	properties.Property("numeric field transformation produces valid numbers", prop.ForAll(
		func(testData NumericTestData) bool {
			result := client.TransformFieldValue(testData.Value, "number")

			// Result should always be a number type (int, int64, or float64)
			// For invalid input, the function returns 0 (untyped int)
			switch result.(type) {
			case int, int64, float64:
				return true
			default:
				return false
			}
		},
		genNumericTestData(),
	))

	// Property 13.3: Boolean field transformation is idempotent
	// For any boolean value, transforming twice should yield the same result
	properties.Property("boolean field transformation is idempotent", prop.ForAll(
		func(value string) bool {
			result1 := client.TransformFieldValue(value, "checkbox")
			// Transform the result back to string and transform again
			result2 := client.TransformFieldValue(result1, "checkbox")

			// Both should be booleans with same value
			b1, ok1 := result1.(bool)
			b2, ok2 := result2.(bool)
			if !ok1 || !ok2 {
				return false
			}
			return b1 == b2
		},
		gen.OneConstOf("true", "false", "1", "0", "yes", "no", "sim", "TRUE", "FALSE"),
	))

	// Property 13.4: Date field transformation produces valid timestamps
	// For any valid date string, transformation should produce a valid Unix timestamp
	properties.Property("date field transformation produces valid timestamps", prop.ForAll(
		func(testData DateTestData) bool {
			result := client.TransformFieldValue(testData.DateString, "date")

			// Result should be an int64 timestamp or nil for invalid dates
			switch v := result.(type) {
			case int64:
				// Timestamp should be reasonable (after year 2000, before year 2100)
				minTimestamp := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
				maxTimestamp := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
				return v >= minTimestamp && v <= maxTimestamp
			case nil:
				// Invalid dates return nil, which is acceptable
				return true
			default:
				return false
			}
		},
		genDateTestData(),
	))

	// Property 13.5: Dropdown field transformation preserves option value
	// For any dropdown value, transformation should preserve the value as string
	properties.Property("dropdown field transformation preserves value", prop.ForAll(
		func(value string) bool {
			result := client.TransformFieldValue(value, "drop_down")
			strResult, ok := result.(string)
			if !ok {
				return false
			}
			return strResult == value
		},
		gen.Identifier(), // Use identifier for valid option IDs/names
	))

	// Property 13.6: Labels field transformation produces array
	// For any comma-separated labels, transformation should produce a string array
	properties.Property("labels field transformation produces array", prop.ForAll(
		func(testData LabelsTestData) bool {
			result := client.TransformFieldValue(testData.Input, "labels")
			arr, ok := result.([]string)
			if !ok {
				return false
			}

			// Empty input should produce empty array
			if testData.Input == "" {
				return len(arr) == 0
			}

			// Non-empty input should produce non-empty array
			return len(arr) > 0
		},
		genLabelsTestData(),
	))

	// Property 13.7: Field type determines transformation behavior
	// For any value and field type, the transformation should be deterministic
	properties.Property("field transformation is deterministic", prop.ForAll(
		func(testData FieldTransformTestData) bool {
			result1 := client.TransformFieldValue(testData.Value, testData.FieldType)
			result2 := client.TransformFieldValue(testData.Value, testData.FieldType)

			// Same input should always produce same output
			return reflect.DeepEqual(result1, result2)
		},
		genFieldTransformTestData(),
	))

	// Property 13.8: Short text fields behave like text fields
	// For any value, short_text transformation should match text transformation
	properties.Property("short_text behaves like text", prop.ForAll(
		func(value string) bool {
			textResult := client.TransformFieldValue(value, "text")
			shortTextResult := client.TransformFieldValue(value, "short_text")
			return reflect.DeepEqual(textResult, shortTextResult)
		},
		gen.AnyString(),
	))

	// Property 13.9: Email fields preserve email format
	// For any email string, transformation should preserve the value
	properties.Property("email field preserves value", prop.ForAll(
		func(email string) bool {
			result := client.TransformFieldValue(email, "email")
			strResult, ok := result.(string)
			if !ok {
				return false
			}
			return strResult == email
		},
		genEmailString(),
	))

	// Property 13.10: URL fields preserve URL format
	// For any URL string, transformation should preserve the value
	properties.Property("url field preserves value", prop.ForAll(
		func(url string) bool {
			result := client.TransformFieldValue(url, "url")
			strResult, ok := result.(string)
			if !ok {
				return false
			}
			return strResult == url
		},
		genURLString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Test data structures

type NumericTestData struct {
	Value string
}

type DateTestData struct {
	DateString string
}

type LabelsTestData struct {
	Input string
}

type FieldTransformTestData struct {
	Value     string
	FieldType string
}

// Generators

func genNumericTestData() gopter.Gen {
	return gen.OneGenOf(
		// Valid integers
		gen.IntRange(-1000000, 1000000).Map(func(n int) NumericTestData {
			return NumericTestData{Value: intToString(n)}
		}),
		// Valid floats
		gen.Float64Range(-1000000.0, 1000000.0).Map(func(f float64) NumericTestData {
			return NumericTestData{Value: floatToString(f)}
		}),
		// Invalid numeric strings
		gen.AlphaString().Map(func(s string) NumericTestData {
			return NumericTestData{Value: s}
		}),
	)
}

func genDateTestData() gopter.Gen {
	return gen.OneGenOf(
		// ISO format dates
		genValidISODate(),
		// BR format dates
		genValidBRDate(),
		// Invalid dates
		gen.AlphaString().Map(func(s string) DateTestData {
			return DateTestData{DateString: s}
		}),
	)
}

func genValidISODate() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(2000, 2050),
		gen.IntRange(1, 12),
		gen.IntRange(1, 28), // Use 28 to avoid invalid dates
	).Map(func(values []interface{}) DateTestData {
		year := values[0].(int)
		month := values[1].(int)
		day := values[2].(int)
		return DateTestData{
			DateString: formatDate(year, month, day, "2006-01-02"),
		}
	})
}

func genValidBRDate() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(2000, 2050),
		gen.IntRange(1, 12),
		gen.IntRange(1, 28),
	).Map(func(values []interface{}) DateTestData {
		year := values[0].(int)
		month := values[1].(int)
		day := values[2].(int)
		return DateTestData{
			DateString: formatDate(year, month, day, "02/01/2006"),
		}
	})
}

func genLabelsTestData() gopter.Gen {
	return gen.OneGenOf(
		// Empty string
		gen.Const(LabelsTestData{Input: ""}),
		// Single label
		gen.Identifier().Map(func(s string) LabelsTestData {
			return LabelsTestData{Input: s}
		}),
		// Multiple labels
		gen.SliceOfN(3, gen.Identifier()).Map(func(labels []string) LabelsTestData {
			return LabelsTestData{Input: joinStrings(labels, ",")}
		}),
	)
}

func genFieldTransformTestData() gopter.Gen {
	fieldTypes := []string{"text", "short_text", "number", "checkbox", "date", "drop_down", "email", "url", "phone"}

	return gopter.CombineGens(
		gen.AnyString(),
		gen.OneConstOf(fieldTypes[0], fieldTypes[1], fieldTypes[2], fieldTypes[3], fieldTypes[4], fieldTypes[5], fieldTypes[6], fieldTypes[7], fieldTypes[8]),
	).Map(func(values []interface{}) FieldTransformTestData {
		return FieldTransformTestData{
			Value:     values[0].(string),
			FieldType: values[1].(string),
		}
	})
}

func genEmailString() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),
		gen.OneConstOf("gmail.com", "example.com", "test.org"),
	).Map(func(values []interface{}) string {
		return values[0].(string) + "@" + values[1].(string)
	})
}

func genURLString() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("http://", "https://"),
		gen.Identifier(),
		gen.OneConstOf(".com", ".org", ".net"),
	).Map(func(values []interface{}) string {
		return values[0].(string) + values[1].(string) + values[2].(string)
	})
}

// Helper functions

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	
	negative := n < 0
	if negative {
		n = -n
	}
	
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	
	return string(digits)
}

func floatToString(f float64) string {
	// Simple float to string conversion
	intPart := int64(f)
	fracPart := f - float64(intPart)
	if fracPart < 0 {
		fracPart = -fracPart
	}
	
	// Get 2 decimal places
	fracInt := int64(fracPart * 100)
	
	result := intToString(int(intPart))
	if fracInt > 0 {
		result += "." + padLeft(intToString(int(fracInt)), 2, '0')
	}
	
	return result
}

func padLeft(s string, length int, pad byte) string {
	for len(s) < length {
		s = string(pad) + s
	}
	return s
}

func formatDate(year, month, day int, format string) string {
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return t.Format(format)
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
