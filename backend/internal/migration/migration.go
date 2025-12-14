package migration

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	_ "github.com/lib/pq"
)

// Migration representa uma migração de banco de dados
type Migration struct {
	Version int
	Name    string
	Up      string
	Down    string
}

// Migrator gerencia as migrações do banco de dados
type Migrator struct {
	db         *sql.DB
	migrations []Migration
}

// NewMigrator cria um novo migrator
func NewMigrator(db *sql.DB) *Migrator {
	return &Migrator{
		db:         db,
		migrations: getAllMigrations(),
	}
}

// Run executa todas as migrações pendentes
func (m *Migrator) Run() error {
	log := logger.Global()
	
	// Cria tabela de migrações se não existir
	if err := m.createMigrationsTable(); err != nil {
		return fmt.Errorf("erro ao criar tabela de migrações: %w", err)
	}

	// Obtém versão atual
	currentVersion, err := m.getCurrentVersion()
	if err != nil {
		return fmt.Errorf("erro ao obter versão atual: %w", err)
	}

	log.Info().Int("current_version", currentVersion).Msg("Versão atual do banco de dados")

	// Ordena migrações por versão
	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].Version < m.migrations[j].Version
	})

	// Executa migrações pendentes
	for _, migration := range m.migrations {
		if migration.Version > currentVersion {
			log.Info().
				Int("version", migration.Version).
				Str("name", migration.Name).
				Msg("Executando migração")

			if err := m.runMigration(migration); err != nil {
				return fmt.Errorf("erro ao executar migração %d (%s): %w", 
					migration.Version, migration.Name, err)
			}

			log.Info().
				Int("version", migration.Version).
				Str("name", migration.Name).
				Msg("Migração executada com sucesso")
		}
	}

	return nil
}

// createMigrationsTable cria a tabela de controle de migrações
func (m *Migrator) createMigrationsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT NOW()
		)
	`
	_, err := m.db.Exec(query)
	return err
}

// getCurrentVersion obtém a versão atual do banco
func (m *Migrator) getCurrentVersion() (int, error) {
	var version int
	query := "SELECT COALESCE(MAX(version), 0) FROM schema_migrations"
	err := m.db.QueryRow(query).Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

// runMigration executa uma migração específica
func (m *Migrator) runMigration(migration Migration) error {
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Executa a migração
	if _, err := tx.Exec(migration.Up); err != nil {
		return err
	}

	// Registra a migração como aplicada
	if _, err := tx.Exec(
		"INSERT INTO schema_migrations (version, applied_at) VALUES ($1, $2)",
		migration.Version, time.Now(),
	); err != nil {
		return err
	}

	return tx.Commit()
}