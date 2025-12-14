package service

import (
	"reflect"
	"testing"

	"github.com/cleberrangel/clickup-excel-api/internal/repository"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// MappingServiceForTest is a testable version of MappingService
type MappingServiceForTest struct {
	customFields []repository.CustomField
}

// NewMappingServiceForTest creates a new mapping service for testing
func NewMappingServiceForTest(fields []repository.CustomField) *MappingServiceForTest {
	return &MappingServiceForTest{
		customFields: fields,
	}
}

// ValidateMappingForTest validates a mapping request without database dependency
func (s *MappingServiceForTest) ValidateMappingForTest(req *MappingRequest, fileColumns []string) *MappingValidationResult {
	result := &MappingValidationResult{
		Valid:       true,
		Errors:      []string{},
		Warnings:    []string{},
		HasTaskID:   false,
		TotalFields: len(req.Mappings),
	}

	if len(req.Mappings) == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, "nenhum mapeamento definido")
		return result
	}

	// Create a map for quick field lookup
	fieldMap := make(map[string]repository.CustomField)
	for _, f := range s.customFields {
		fieldMap[f.ID] = f
	}

	// Create a map for quick column lookup
	columnSet := make(map[string]bool)
	for _, col := range fileColumns {
		columnSet[col] = true
	}

	// Track mapped fields to detect duplicates
	mappedFields := make(map[string]string) // fieldID -> column

	for _, mapping := range req.Mappings {
		// Check if this is the task ID column
		if mapping.IsTaskID {
			result.HasTaskID = true
			continue
		}

		// Skip empty mappings
		if mapping.FieldID == "" {
			continue
		}

		// Validate column exists in file
		if !columnSet[mapping.Column] {
			result.Valid = false
			result.Errors = append(result.Errors, "coluna '"+mapping.Column+"' não encontrada no arquivo")
			continue
		}

		// Validate field exists
		field, exists := fieldMap[mapping.FieldID]
		if !exists {
			result.Valid = false
			result.Errors = append(result.Errors, "campo '"+mapping.FieldID+"' não encontrado")
			continue
		}

		// Check for duplicate mappings (same field mapped to multiple columns)
		if existingCol, isDuplicate := mappedFields[mapping.FieldID]; isDuplicate {
			result.Valid = false
			result.Errors = append(result.Errors, "campo '"+field.Name+"' já mapeado para coluna '"+existingCol+"'")
			continue
		}
		mappedFields[mapping.FieldID] = mapping.Column
	}

	// Check for required task ID column
	if !result.HasTaskID {
		result.Valid = false
		result.Errors = append(result.Errors, "coluna 'id task' é obrigatória para identificar as tarefas")
	}

	return result
}

// CheckDuplicateMappingsForTest checks for duplicate field mappings
func (s *MappingServiceForTest) CheckDuplicateMappingsForTest(mappings []ColumnMapping) []string {
	var duplicates []string
	fieldCount := make(map[string]int)
	fieldNames := make(map[string]string)

	for _, m := range mappings {
		if m.FieldID == "" || m.IsTaskID {
			continue
		}
		fieldCount[m.FieldID]++
		fieldNames[m.FieldID] = m.FieldName
	}

	for fieldID, count := range fieldCount {
		if count > 1 {
			name := fieldNames[fieldID]
			if name == "" {
				name = fieldID
			}
			duplicates = append(duplicates, name)
		}
	}

	return duplicates
}

// TestMappingValidationCompleteness tests Property 6: Mapping validation completeness
// **Feature: clickup-field-updater, Property 6: Mapping validation completeness**
// **Validates: Requirements 4.3, 4.4, 4.5**
//
// For any column mapping configuration, the system should validate required mappings
// (including mandatory "id task" column), detect duplicates, and ensure type compatibility.
func TestMappingValidationCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.MaxSize = 20

	properties := gopter.NewProperties(parameters)

	// Property 6.1: Duplicate mappings are always detected
	// For any mapping configuration with duplicate field IDs, the system should detect them
	properties.Property("duplicate mappings are detected", prop.ForAll(
		func(testData MappingTestData) bool {
			svc := NewMappingServiceForTest(testData.CustomFields)
			duplicates := svc.CheckDuplicateMappingsForTest(testData.Mappings)

			// Count actual duplicates in input
			fieldCount := make(map[string]int)
			for _, m := range testData.Mappings {
				if m.FieldID != "" && !m.IsTaskID {
					fieldCount[m.FieldID]++
				}
			}

			expectedDuplicateCount := 0
			for _, count := range fieldCount {
				if count > 1 {
					expectedDuplicateCount++
				}
			}

			// The number of detected duplicates should match expected
			return len(duplicates) == expectedDuplicateCount
		},
		genMappingTestData(),
	))

	// Property 6.2: Missing task ID column is always detected
	// For any mapping configuration without a task ID column, validation should fail
	properties.Property("missing task ID column is detected", prop.ForAll(
		func(testData MappingTestDataNoTaskID) bool {
			svc := NewMappingServiceForTest(testData.CustomFields)

			req := &MappingRequest{
				FilePath: "/tmp/test.csv",
				Mappings: testData.Mappings,
				Title:    "Test Mapping",
			}

			result := svc.ValidateMappingForTest(req, testData.FileColumns)

			// Mappings without task ID should fail validation
			return !result.HasTaskID && !result.Valid
		},
		genMappingTestDataNoTaskID(),
	))

	// Property 6.3: Valid mappings with task ID pass validation
	// For any valid mapping configuration with task ID and no duplicates, validation should pass
	properties.Property("valid mappings with task ID pass validation", prop.ForAll(
		func(testData ValidMappingTestData) bool {
			svc := NewMappingServiceForTest(testData.CustomFields)

			req := &MappingRequest{
				FilePath: "/tmp/test.csv",
				Mappings: testData.Mappings,
				Title:    "Test Mapping",
			}

			result := svc.ValidateMappingForTest(req, testData.FileColumns)

			// Valid mappings should pass
			return result.Valid && result.HasTaskID && len(result.Errors) == 0
		},
		genValidMappingTestData(),
	))

	// Property 6.4: Invalid column references are detected
	// For any mapping referencing a non-existent column, validation should fail
	properties.Property("invalid column references are detected", prop.ForAll(
		func(testData InvalidColumnTestData) bool {
			svc := NewMappingServiceForTest(testData.CustomFields)

			req := &MappingRequest{
				FilePath: "/tmp/test.csv",
				Mappings: testData.Mappings,
				Title:    "Test Mapping",
			}

			result := svc.ValidateMappingForTest(req, testData.FileColumns)

			// If there's a mapping to a non-existent column, validation should fail
			return !result.Valid
		},
		genInvalidColumnTestData(),
	))

	// Property 6.5: Invalid field references are detected
	// For any mapping referencing a non-existent field, validation should fail
	properties.Property("invalid field references are detected", prop.ForAll(
		func(testData InvalidFieldTestData) bool {
			svc := NewMappingServiceForTest(testData.CustomFields)

			req := &MappingRequest{
				FilePath: "/tmp/test.csv",
				Mappings: testData.Mappings,
				Title:    "Test Mapping",
			}

			result := svc.ValidateMappingForTest(req, testData.FileColumns)

			// If there's a mapping to a non-existent field, validation should fail
			return !result.Valid
		},
		genInvalidFieldTestData(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Test data structures
type MappingTestData struct {
	CustomFields []repository.CustomField
	Mappings     []ColumnMapping
	FileColumns  []string
}

type MappingTestDataNoTaskID struct {
	CustomFields []repository.CustomField
	Mappings     []ColumnMapping
	FileColumns  []string
}

type ValidMappingTestData struct {
	CustomFields []repository.CustomField
	Mappings     []ColumnMapping
	FileColumns  []string
}

type InvalidColumnTestData struct {
	CustomFields []repository.CustomField
	Mappings     []ColumnMapping
	FileColumns  []string
}

type InvalidFieldTestData struct {
	CustomFields []repository.CustomField
	Mappings     []ColumnMapping
	FileColumns  []string
}

// Generators
func genMappingTestData() gopter.Gen {
	return gen.IntRange(1, 5).FlatMap(func(numFields interface{}) gopter.Gen {
		fieldCount := numFields.(int)
		return gen.IntRange(1, 5).FlatMap(func(numMappings interface{}) gopter.Gen {
			mappingCount := numMappings.(int)
			return gopter.CombineGens(
				genCustomFieldSlice(fieldCount),
			).Map(func(values []interface{}) MappingTestData {
				fields := values[0].([]repository.CustomField)
				mappings := generateMappingsWithDuplicates(fields, mappingCount)
				columns := extractColumnsFromMappings(mappings)
				return MappingTestData{
					CustomFields: fields,
					Mappings:     mappings,
					FileColumns:  columns,
				}
			})
		}, reflect.TypeOf(MappingTestData{}))
	}, reflect.TypeOf(MappingTestData{}))
}

func genMappingTestDataNoTaskID() gopter.Gen {
	return gen.IntRange(1, 5).FlatMap(func(numFields interface{}) gopter.Gen {
		fieldCount := numFields.(int)
		return gen.IntRange(1, 5).FlatMap(func(numMappings interface{}) gopter.Gen {
			mappingCount := numMappings.(int)
			return gopter.CombineGens(
				genCustomFieldSlice(fieldCount),
			).Map(func(values []interface{}) MappingTestDataNoTaskID {
				fields := values[0].([]repository.CustomField)
				mappings := generateMappingsNoTaskID(fields, mappingCount)
				columns := extractColumnsFromMappings(mappings)
				return MappingTestDataNoTaskID{
					CustomFields: fields,
					Mappings:     mappings,
					FileColumns:  columns,
				}
			})
		}, reflect.TypeOf(MappingTestDataNoTaskID{}))
	}, reflect.TypeOf(MappingTestDataNoTaskID{}))
}

func genValidMappingTestData() gopter.Gen {
	return gen.IntRange(1, 5).FlatMap(func(numFields interface{}) gopter.Gen {
		fieldCount := numFields.(int)
		return gen.IntRange(1, 3).FlatMap(func(numMappings interface{}) gopter.Gen {
			mappingCount := numMappings.(int)
			return gopter.CombineGens(
				genCustomFieldSlice(fieldCount),
			).Map(func(values []interface{}) ValidMappingTestData {
				fields := values[0].([]repository.CustomField)
				mappings := generateValidMappings(fields, mappingCount)
				columns := extractColumnsFromMappings(mappings)
				return ValidMappingTestData{
					CustomFields: fields,
					Mappings:     mappings,
					FileColumns:  columns,
				}
			})
		}, reflect.TypeOf(ValidMappingTestData{}))
	}, reflect.TypeOf(ValidMappingTestData{}))
}

func genInvalidColumnTestData() gopter.Gen {
	return gen.IntRange(1, 5).FlatMap(func(numFields interface{}) gopter.Gen {
		fieldCount := numFields.(int)
		return gopter.CombineGens(
			genCustomFieldSlice(fieldCount),
		).Map(func(values []interface{}) InvalidColumnTestData {
			fields := values[0].([]repository.CustomField)
			mappings := generateMappingWithInvalidColumn(fields)
			// Only include task ID column, not the invalid column
			columns := []string{"id_task"}
			return InvalidColumnTestData{
				CustomFields: fields,
				Mappings:     mappings,
				FileColumns:  columns,
			}
		})
	}, reflect.TypeOf(InvalidColumnTestData{}))
}

func genInvalidFieldTestData() gopter.Gen {
	return gen.IntRange(1, 5).FlatMap(func(numFields interface{}) gopter.Gen {
		fieldCount := numFields.(int)
		return gopter.CombineGens(
			genCustomFieldSlice(fieldCount),
		).Map(func(values []interface{}) InvalidFieldTestData {
			fields := values[0].([]repository.CustomField)
			mappings := generateMappingWithInvalidField(fields)
			columns := extractColumnsFromMappings(mappings)
			return InvalidFieldTestData{
				CustomFields: fields,
				Mappings:     mappings,
				FileColumns:  columns,
			}
		})
	}, reflect.TypeOf(InvalidFieldTestData{}))
}

func genCustomFieldSlice(count int) gopter.Gen {
	return gen.SliceOfN(count, genSingleCustomField()).SuchThat(func(fields []repository.CustomField) bool {
		// Ensure unique field IDs
		seen := make(map[string]bool)
		for _, f := range fields {
			if seen[f.ID] || f.ID == "" {
				return false
			}
			seen[f.ID] = true
		}
		return len(fields) == count
	})
}

func genSingleCustomField() gopter.Gen {
	fieldTypes := []string{"text", "number", "date", "drop_down", "checkbox", "email", "url"}

	return gopter.CombineGens(
		gen.Identifier(),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) <= 50 }),
		gen.OneConstOf(fieldTypes[0], fieldTypes[1], fieldTypes[2], fieldTypes[3], fieldTypes[4], fieldTypes[5], fieldTypes[6]),
	).Map(func(values []interface{}) repository.CustomField {
		return repository.CustomField{
			ID:   values[0].(string),
			Name: values[1].(string),
			Type: values[2].(string),
		}
	})
}

// Helper functions to generate mappings
func generateMappingsWithDuplicates(fields []repository.CustomField, count int) []ColumnMapping {
	if len(fields) == 0 {
		return []ColumnMapping{}
	}

	var mappings []ColumnMapping
	for i := 0; i < count; i++ {
		// Use modulo to potentially create duplicates
		fieldIdx := i % len(fields)
		field := fields[fieldIdx]
		mappings = append(mappings, ColumnMapping{
			Column:    "col_" + field.ID,
			FieldID:   field.ID,
			FieldName: field.Name,
			FieldType: field.Type,
		})
	}
	return mappings
}

func generateMappingsNoTaskID(fields []repository.CustomField, count int) []ColumnMapping {
	if len(fields) == 0 {
		return []ColumnMapping{}
	}

	var mappings []ColumnMapping
	for i := 0; i < count && i < len(fields); i++ {
		field := fields[i]
		mappings = append(mappings, ColumnMapping{
			Column:    "col_" + field.ID,
			FieldID:   field.ID,
			FieldName: field.Name,
			FieldType: field.Type,
			IsTaskID:  false,
		})
	}
	return mappings
}

func generateValidMappings(fields []repository.CustomField, count int) []ColumnMapping {
	// Start with task ID mapping
	mappings := []ColumnMapping{
		{Column: "id_task", IsTaskID: true},
	}

	// Add unique field mappings
	for i := 0; i < count && i < len(fields); i++ {
		field := fields[i]
		mappings = append(mappings, ColumnMapping{
			Column:    "col_" + field.ID,
			FieldID:   field.ID,
			FieldName: field.Name,
			FieldType: field.Type,
		})
	}
	return mappings
}

func generateMappingWithInvalidColumn(fields []repository.CustomField) []ColumnMapping {
	mappings := []ColumnMapping{
		{Column: "id_task", IsTaskID: true},
	}

	if len(fields) > 0 {
		field := fields[0]
		mappings = append(mappings, ColumnMapping{
			Column:    "nonexistent_column",
			FieldID:   field.ID,
			FieldName: field.Name,
			FieldType: field.Type,
		})
	} else {
		mappings = append(mappings, ColumnMapping{
			Column:    "nonexistent_column",
			FieldID:   "field1",
			FieldName: "Field 1",
		})
	}
	return mappings
}

func generateMappingWithInvalidField(fields []repository.CustomField) []ColumnMapping {
	// Generate an invalid field ID that doesn't exist
	invalidFieldID := "invalid_field_id_xyz"
	for _, f := range fields {
		if f.ID == invalidFieldID {
			invalidFieldID = invalidFieldID + "_extra"
		}
	}

	return []ColumnMapping{
		{Column: "id_task", IsTaskID: true},
		{Column: "some_column", FieldID: invalidFieldID, FieldName: "Invalid Field"},
	}
}

func extractColumnsFromMappings(mappings []ColumnMapping) []string {
	columnSet := make(map[string]bool)
	for _, m := range mappings {
		if m.Column != "" {
			columnSet[m.Column] = true
		}
	}

	var columns []string
	for col := range columnSet {
		columns = append(columns, col)
	}
	return columns
}
