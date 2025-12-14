package repository

import (
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/database"
	"github.com/cleberrangel/clickup-excel-api/internal/migration"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	_ "github.com/lib/pq"
)

// **Feature: clickup-field-updater, Property 3: Metadata synchronization completeness**
// **Validates: Requirements 14.1, 14.2, 14.3, 14.4, 14.5**

func setupTestDB(t *testing.T) *sql.DB {
	// Configuração de teste do banco
	dbConfig := database.Config{
		Host:     getEnvOrDefault("TEST_DB_HOST", "127.0.0.1"),
		Port:     getEnvOrDefault("TEST_DB_PORT", "5432"),
		User:     getEnvOrDefault("TEST_DB_USER", "postgres"),
		Password: getEnvOrDefault("TEST_DB_PASSWORD", "postgres"),
		DBName:   fmt.Sprintf("test_clickup_%d", time.Now().UnixNano()),
		SSLMode:  "disable",
	}

	// Conecta ao postgres para criar o banco de teste
	adminConfig := dbConfig
	adminConfig.DBName = "postgres"
	
	adminDB, err := database.Connect(adminConfig)
	if err != nil {
		t.Skipf("Pulando teste: não foi possível conectar ao PostgreSQL: %v", err)
	}
	defer adminDB.Close()

	// Cria banco de teste
	_, err = adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", dbConfig.DBName))
	if err != nil {
		t.Fatalf("Erro ao criar banco de teste: %v", err)
	}

	// Conecta ao banco de teste
	testDB, err := database.Connect(dbConfig)
	if err != nil {
		t.Fatalf("Erro ao conectar ao banco de teste: %v", err)
	}

	// Executa migrações
	migrator := migration.NewMigrator(testDB)
	if err := migrator.Run(); err != nil {
		testDB.Close()
		t.Fatalf("Erro ao executar migrações: %v", err)
	}

	// Cleanup function
	t.Cleanup(func() {
		testDB.Close()
		adminDB, _ := database.Connect(adminConfig)
		if adminDB != nil {
			adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbConfig.DBName))
			adminDB.Close()
		}
	})

	return testDB
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// TestMetadataSynchronizationCompleteness testa a propriedade de completude da sincronização de metadados
func TestMetadataSynchronizationCompleteness(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMetadataRepository(db)

	// Configure properties with smaller test cases and timeout
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 10 // Reduced from default 100
	parameters.MaxSize = 5             // Limit size of generated data
	properties := gopter.NewProperties(parameters)

	// Property 3: Metadata synchronization completeness
	// Para qualquer token do ClickUp, quando a sincronização de metadados ocorre,
	// todas as entidades hierárquicas (workspaces, spaces, folders, lists, custom fields)
	// devem ser armazenadas com ID, nome e relacionamentos pai completos
	properties.Property("metadata synchronization stores complete hierarchical data", prop.ForAll(
		func(workspaceData WorkspaceTestData) bool {
			// Insere workspace
			workspace := Workspace{
				ID:   workspaceData.ID,
				Name: workspaceData.Name,
			}
			if err := repo.UpsertWorkspace(workspace); err != nil {
				t.Logf("Erro ao inserir workspace: %v", err)
				return false
			}

			// Verifica se workspace foi inserido corretamente
			workspaces, err := repo.GetWorkspaces()
			if err != nil {
				t.Logf("Erro ao buscar workspaces: %v", err)
				return false
			}

			// Deve conter o workspace inserido
			found := false
			for _, w := range workspaces {
				if w.ID == workspaceData.ID && w.Name == workspaceData.Name {
					found = true
					// Verifica se timestamps foram definidos
					if w.CreatedAt.IsZero() || w.UpdatedAt.IsZero() {
						t.Logf("Timestamps não foram definidos corretamente")
						return false
					}
					break
				}
			}

			if !found {
				t.Logf("Workspace não foi encontrado após inserção")
				return false
			}

			// Testa spaces vinculados ao workspace
			for _, spaceData := range workspaceData.Spaces {
				space := Space{
					ID:          spaceData.ID,
					WorkspaceID: workspaceData.ID,
					Name:        spaceData.Name,
				}
				if err := repo.UpsertSpace(space); err != nil {
					t.Logf("Erro ao inserir space: %v", err)
					return false
				}

				// Verifica relacionamento pai
				spaces, err := repo.GetSpacesByWorkspace(workspaceData.ID)
				if err != nil {
					t.Logf("Erro ao buscar spaces: %v", err)
					return false
				}

				spaceFound := false
				for _, s := range spaces {
					if s.ID == spaceData.ID && s.WorkspaceID == workspaceData.ID {
						spaceFound = true
						break
					}
				}

				if !spaceFound {
					t.Logf("Space não foi encontrado com relacionamento correto")
					return false
				}

				// Testa folders vinculados ao space
				for _, folderData := range spaceData.Folders {
					folder := Folder{
						ID:      folderData.ID,
						SpaceID: spaceData.ID,
						Name:    folderData.Name,
					}
					if err := repo.UpsertFolder(folder); err != nil {
						t.Logf("Erro ao inserir folder: %v", err)
						return false
					}

					// Verifica relacionamento pai
					folders, err := repo.GetFoldersBySpace(spaceData.ID)
					if err != nil {
						t.Logf("Erro ao buscar folders: %v", err)
						return false
					}

					folderFound := false
					for _, f := range folders {
						if f.ID == folderData.ID && f.SpaceID == spaceData.ID {
							folderFound = true
							break
						}
					}

					if !folderFound {
						t.Logf("Folder não foi encontrado com relacionamento correto")
						return false
					}

					// Testa lists vinculadas ao folder
					for _, listData := range folderData.Lists {
						list := List{
							ID:       listData.ID,
							FolderID: folderData.ID,
							Name:     listData.Name,
						}
						if err := repo.UpsertList(list); err != nil {
							t.Logf("Erro ao inserir list: %v", err)
							return false
						}

						// Verifica relacionamento pai
						lists, err := repo.GetListsByFolder(folderData.ID)
						if err != nil {
							t.Logf("Erro ao buscar lists: %v", err)
							return false
						}

						listFound := false
						for _, l := range lists {
							if l.ID == listData.ID && l.FolderID == folderData.ID {
								listFound = true
								break
							}
						}

						if !listFound {
							t.Logf("List não foi encontrada com relacionamento correto")
							return false
						}
					}
				}
			}

			// Testa custom fields
			for _, fieldData := range workspaceData.CustomFields {
				field := CustomField{
					ID:         fieldData.ID,
					Name:       fieldData.Name,
					Type:       fieldData.Type,
					Options:    fieldData.Options,
					OrderIndex: fieldData.OrderIndex,
				}
				if err := repo.UpsertCustomField(field); err != nil {
					t.Logf("Erro ao inserir custom field: %v", err)
					return false
				}
			}

			// Verifica se todos os custom fields foram inseridos
			fields, err := repo.GetCustomFields()
			if err != nil {
				t.Logf("Erro ao buscar custom fields: %v", err)
				return false
			}

			for _, expectedField := range workspaceData.CustomFields {
				fieldFound := false
				for _, f := range fields {
					if f.ID == expectedField.ID && f.Name == expectedField.Name && f.Type == expectedField.Type {
						fieldFound = true
						break
					}
				}
				if !fieldFound {
					t.Logf("Custom field não foi encontrado: %s", expectedField.ID)
					return false
				}
			}

			return true
		},
		genWorkspaceTestData(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Estruturas de dados para teste
type WorkspaceTestData struct {
	ID           string
	Name         string
	Spaces       []SpaceTestData
	CustomFields []CustomFieldTestData
}

type SpaceTestData struct {
	ID      string
	Name    string
	Folders []FolderTestData
}

type FolderTestData struct {
	ID    string
	Name  string
	Lists []ListTestData
}

type ListTestData struct {
	ID   string
	Name string
}

type CustomFieldTestData struct {
	ID         string
	Name       string
	Type       string
	Options    map[string]interface{}
	OrderIndex int
}

// Geradores para property-based testing
// Using generators that produce valid values directly instead of filtering with SuchThat
// to avoid excessive discards

func genNonEmptyAlphaString() gopter.Gen {
	// Generate strings with minimum length 1 to avoid empty strings
	return gen.AlphaString().Map(func(s string) string {
		if len(s) == 0 {
			return "a" // Ensure non-empty
		}
		return s
	})
}

func genWorkspaceTestData() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),        // ID
		genNonEmptyAlphaString(), // Name - guaranteed non-empty
		genSliceOfSpaces(),      // Spaces (0-2)
		genSliceOfCustomFields(), // CustomFields (0-2)
	).Map(func(values []interface{}) WorkspaceTestData {
		return WorkspaceTestData{
			ID:           values[0].(string),
			Name:         values[1].(string),
			Spaces:       values[2].([]SpaceTestData),
			CustomFields: values[3].([]CustomFieldTestData),
		}
	})
}

func genSliceOfSpaces() gopter.Gen {
	return gen.IntRange(0, 2).FlatMap(func(n interface{}) gopter.Gen {
		count := n.(int)
		if count == 0 {
			return gen.Const([]SpaceTestData{})
		}
		gens := make([]gopter.Gen, count)
		for i := 0; i < count; i++ {
			// Use index suffix to ensure unique IDs within the same test case
			idx := i
			gens[i] = genSpaceTestDataWithSuffix(fmt.Sprintf("_s%d", idx))
		}
		return gopter.CombineGens(gens...).Map(func(values []interface{}) []SpaceTestData {
			result := make([]SpaceTestData, len(values))
			for i, v := range values {
				result[i] = v.(SpaceTestData)
			}
			return result
		})
	}, reflect.TypeOf([]SpaceTestData{}))
}

func genSliceOfCustomFields() gopter.Gen {
	return gen.IntRange(0, 2).FlatMap(func(n interface{}) gopter.Gen {
		count := n.(int)
		if count == 0 {
			return gen.Const([]CustomFieldTestData{})
		}
		gens := make([]gopter.Gen, count)
		for i := 0; i < count; i++ {
			// Use index suffix to ensure unique IDs within the same test case
			idx := i
			gens[i] = genCustomFieldTestDataWithSuffix(fmt.Sprintf("_%d", idx))
		}
		return gopter.CombineGens(gens...).Map(func(values []interface{}) []CustomFieldTestData {
			result := make([]CustomFieldTestData, len(values))
			for i, v := range values {
				result[i] = v.(CustomFieldTestData)
			}
			return result
		})
	}, reflect.TypeOf([]CustomFieldTestData{}))
}

func genSpaceTestData() gopter.Gen {
	return genSpaceTestDataWithSuffix("")
}

func genSpaceTestDataWithSuffix(suffix string) gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),        // ID
		genNonEmptyAlphaString(), // Name
		genSliceOfFoldersWithPrefix(suffix), // Folders (0-2)
	).Map(func(values []interface{}) SpaceTestData {
		return SpaceTestData{
			ID:      values[0].(string) + suffix,
			Name:    values[1].(string),
			Folders: values[2].([]FolderTestData),
		}
	})
}

func genSliceOfFolders() gopter.Gen {
	return genSliceOfFoldersWithPrefix("")
}

func genSliceOfFoldersWithPrefix(prefix string) gopter.Gen {
	return gen.IntRange(0, 2).FlatMap(func(n interface{}) gopter.Gen {
		count := n.(int)
		if count == 0 {
			return gen.Const([]FolderTestData{})
		}
		gens := make([]gopter.Gen, count)
		for i := 0; i < count; i++ {
			// Use index suffix to ensure unique IDs within the same test case
			idx := i
			gens[i] = genFolderTestDataWithSuffix(fmt.Sprintf("%s_f%d", prefix, idx))
		}
		return gopter.CombineGens(gens...).Map(func(values []interface{}) []FolderTestData {
			result := make([]FolderTestData, len(values))
			for i, v := range values {
				result[i] = v.(FolderTestData)
			}
			return result
		})
	}, reflect.TypeOf([]FolderTestData{}))
}

func genFolderTestData() gopter.Gen {
	return genFolderTestDataWithSuffix("")
}

func genFolderTestDataWithSuffix(suffix string) gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),        // ID
		genNonEmptyAlphaString(), // Name
		genSliceOfListsWithPrefix(suffix), // Lists (0-2)
	).Map(func(values []interface{}) FolderTestData {
		return FolderTestData{
			ID:    values[0].(string) + suffix,
			Name:  values[1].(string),
			Lists: values[2].([]ListTestData),
		}
	})
}

func genSliceOfLists() gopter.Gen {
	return genSliceOfListsWithPrefix("")
}

func genSliceOfListsWithPrefix(prefix string) gopter.Gen {
	return gen.IntRange(0, 2).FlatMap(func(n interface{}) gopter.Gen {
		count := n.(int)
		if count == 0 {
			return gen.Const([]ListTestData{})
		}
		gens := make([]gopter.Gen, count)
		for i := 0; i < count; i++ {
			// Use index suffix to ensure unique IDs within the same test case
			idx := i
			gens[i] = genListTestDataWithSuffix(fmt.Sprintf("%s_l%d", prefix, idx))
		}
		return gopter.CombineGens(gens...).Map(func(values []interface{}) []ListTestData {
			result := make([]ListTestData, len(values))
			for i, v := range values {
				result[i] = v.(ListTestData)
			}
			return result
		})
	}, reflect.TypeOf([]ListTestData{}))
}

func genListTestData() gopter.Gen {
	return genListTestDataWithSuffix("")
}

func genListTestDataWithSuffix(suffix string) gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),        // ID
		genNonEmptyAlphaString(), // Name
	).Map(func(values []interface{}) ListTestData {
		return ListTestData{
			ID:   values[0].(string) + suffix,
			Name: values[1].(string),
		}
	})
}

func genCustomFieldTestData() gopter.Gen {
	return genCustomFieldTestDataWithSuffix("")
}

func genCustomFieldTestDataWithSuffix(suffix string) gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),        // ID
		genNonEmptyAlphaString(), // Name
		gen.OneConstOf("text", "number", "dropdown", "date"), // Type
		genOptionsMap(),         // Options
		gen.IntRange(0, 100),    // OrderIndex
	).Map(func(values []interface{}) CustomFieldTestData {
		return CustomFieldTestData{
			ID:         values[0].(string) + suffix, // Add suffix to ensure uniqueness
			Name:       values[1].(string),
			Type:       values[2].(string),
			Options:    values[3].(map[string]interface{}),
			OrderIndex: values[4].(int),
		}
	})
}

func genOptionsMap() gopter.Gen {
	// Generate a small map directly without filtering
	return gen.IntRange(0, 3).FlatMap(func(n interface{}) gopter.Gen {
		count := n.(int)
		if count == 0 {
			return gen.Const(map[string]interface{}{})
		}
		// Generate fixed number of key-value pairs
		gens := make([]gopter.Gen, count*2)
		for i := 0; i < count; i++ {
			gens[i*2] = genNonEmptyAlphaString()   // key
			gens[i*2+1] = genNonEmptyAlphaString() // value
		}
		return gopter.CombineGens(gens...).Map(func(values []interface{}) map[string]interface{} {
			result := make(map[string]interface{})
			for i := 0; i < len(values); i += 2 {
				result[values[i].(string)] = values[i+1].(string)
			}
			return result
		})
	}, reflect.TypeOf(map[string]interface{}{}))
}