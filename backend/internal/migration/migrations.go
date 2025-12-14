package migration

// getAllMigrations retorna todas as migrações disponíveis
func getAllMigrations() []Migration {
	return []Migration{
		{
			Version: 1,
			Name:    "create_metadata_tables",
			Up: `
				-- Workspaces do ClickUp
				CREATE TABLE workspaces (
					id VARCHAR(50) PRIMARY KEY,
					name VARCHAR(255) NOT NULL,
					created_at TIMESTAMP DEFAULT NOW(),
					updated_at TIMESTAMP DEFAULT NOW()
				);

				-- Spaces do ClickUp
				CREATE TABLE spaces (
					id VARCHAR(50) PRIMARY KEY,
					workspace_id VARCHAR(50) REFERENCES workspaces(id) ON DELETE CASCADE,
					name VARCHAR(255) NOT NULL,
					created_at TIMESTAMP DEFAULT NOW(),
					updated_at TIMESTAMP DEFAULT NOW()
				);

				-- Folders do ClickUp
				CREATE TABLE folders (
					id VARCHAR(50) PRIMARY KEY,
					space_id VARCHAR(50) REFERENCES spaces(id) ON DELETE CASCADE,
					name VARCHAR(255) NOT NULL,
					created_at TIMESTAMP DEFAULT NOW(),
					updated_at TIMESTAMP DEFAULT NOW()
				);

				-- Listas do ClickUp
				CREATE TABLE lists (
					id VARCHAR(50) PRIMARY KEY,
					folder_id VARCHAR(50) REFERENCES folders(id) ON DELETE CASCADE,
					name VARCHAR(255) NOT NULL,
					created_at TIMESTAMP DEFAULT NOW(),
					updated_at TIMESTAMP DEFAULT NOW()
				);

				-- Campos personalizados
				CREATE TABLE custom_fields (
					id VARCHAR(50) PRIMARY KEY,
					name VARCHAR(255) NOT NULL,
					type VARCHAR(50) NOT NULL,
					options JSONB,
					orderindex INTEGER,
					created_at TIMESTAMP DEFAULT NOW(),
					updated_at TIMESTAMP DEFAULT NOW()
				);
			`,
			Down: `
				DROP TABLE IF EXISTS custom_fields;
				DROP TABLE IF EXISTS lists;
				DROP TABLE IF EXISTS folders;
				DROP TABLE IF EXISTS spaces;
				DROP TABLE IF EXISTS workspaces;
			`,
		},
		{
			Version: 2,
			Name:    "create_queue_tables",
			Up: `
				-- Fila de processamento
				CREATE TABLE job_queue (
					id SERIAL PRIMARY KEY,
					user_id VARCHAR(100) NOT NULL,
					title VARCHAR(255) NOT NULL,
					status VARCHAR(50) DEFAULT 'pending',
					file_path VARCHAR(500),
					mapping JSONB NOT NULL,
					total_rows INTEGER DEFAULT 0,
					processed_rows INTEGER DEFAULT 0,
					success_count INTEGER DEFAULT 0,
					error_count INTEGER DEFAULT 0,
					error_details JSONB,
					created_at TIMESTAMP DEFAULT NOW(),
					updated_at TIMESTAMP DEFAULT NOW(),
					completed_at TIMESTAMP,
					CONSTRAINT chk_status CHECK (status IN ('pending', 'processing', 'completed', 'failed'))
				);

				-- Histórico de operações
				CREATE TABLE operation_history (
					id SERIAL PRIMARY KEY,
					user_id VARCHAR(100) NOT NULL,
					operation_type VARCHAR(50) NOT NULL,
					title VARCHAR(255) NOT NULL,
					status VARCHAR(50) NOT NULL,
					details JSONB,
					created_at TIMESTAMP DEFAULT NOW(),
					CONSTRAINT chk_operation_type CHECK (operation_type IN ('report_generation', 'field_update')),
					CONSTRAINT chk_operation_status CHECK (status IN ('pending', 'processing', 'completed', 'failed'))
				);
			`,
			Down: `
				DROP TABLE IF EXISTS operation_history;
				DROP TABLE IF EXISTS job_queue;
			`,
		},
		{
			Version: 3,
			Name:    "create_configuration_tables",
			Up: `
				-- Configurações do usuário
				CREATE TABLE user_config (
					user_id VARCHAR(100) PRIMARY KEY,
					clickup_token_encrypted TEXT,
					rate_limit_per_minute INTEGER DEFAULT 2000,
					created_at TIMESTAMP DEFAULT NOW(),
					updated_at TIMESTAMP DEFAULT NOW(),
					CONSTRAINT chk_rate_limit CHECK (rate_limit_per_minute >= 10 AND rate_limit_per_minute <= 10000)
				);
			`,
			Down: `
				DROP TABLE IF EXISTS user_config;
			`,
		},
		{
			Version: 4,
			Name:    "create_users_table",
			Up: `
				-- Tabela de usuários para autenticação básica
				CREATE TABLE users (
					id SERIAL PRIMARY KEY,
					username VARCHAR(100) UNIQUE NOT NULL,
					password_hash VARCHAR(255) NOT NULL,
					created_at TIMESTAMP DEFAULT NOW(),
					updated_at TIMESTAMP DEFAULT NOW()
				);

				-- Índice para busca por username
				CREATE INDEX idx_users_username ON users(username);
			`,
			Down: `
				DROP INDEX IF EXISTS idx_users_username;
				DROP TABLE IF EXISTS users;
			`,
		},
		{
			Version: 5,
			Name:    "create_performance_indexes",
			Up: `
				-- Índices para performance das tabelas de metadados
				CREATE INDEX idx_spaces_workspace_id ON spaces(workspace_id);
				CREATE INDEX idx_folders_space_id ON folders(space_id);
				CREATE INDEX idx_lists_folder_id ON lists(folder_id);
				CREATE INDEX idx_custom_fields_type ON custom_fields(type);
				CREATE INDEX idx_custom_fields_orderindex ON custom_fields(orderindex);

				-- Índices para performance das tabelas de fila
				CREATE INDEX idx_job_queue_status ON job_queue(status);
				CREATE INDEX idx_job_queue_user_id ON job_queue(user_id);
				CREATE INDEX idx_job_queue_created_at ON job_queue(created_at);
				CREATE INDEX idx_job_queue_user_status ON job_queue(user_id, status);

				-- Índices para performance do histórico
				CREATE INDEX idx_operation_history_user_id ON operation_history(user_id);
				CREATE INDEX idx_operation_history_created_at ON operation_history(created_at);
				CREATE INDEX idx_operation_history_operation_type ON operation_history(operation_type);
				CREATE INDEX idx_operation_history_status ON operation_history(status);

				-- Índices para configurações
				CREATE INDEX idx_user_config_updated_at ON user_config(updated_at);
			`,
			Down: `
				-- Remove índices de configurações
				DROP INDEX IF EXISTS idx_user_config_updated_at;

				-- Remove índices do histórico
				DROP INDEX IF EXISTS idx_operation_history_status;
				DROP INDEX IF EXISTS idx_operation_history_operation_type;
				DROP INDEX IF EXISTS idx_operation_history_created_at;
				DROP INDEX IF EXISTS idx_operation_history_user_id;

				-- Remove índices da fila
				DROP INDEX IF EXISTS idx_job_queue_user_status;
				DROP INDEX IF EXISTS idx_job_queue_created_at;
				DROP INDEX IF EXISTS idx_job_queue_user_id;
				DROP INDEX IF EXISTS idx_job_queue_status;

				-- Remove índices dos metadados
				DROP INDEX IF EXISTS idx_custom_fields_orderindex;
				DROP INDEX IF EXISTS idx_custom_fields_type;
				DROP INDEX IF EXISTS idx_lists_folder_id;
				DROP INDEX IF EXISTS idx_folders_space_id;
				DROP INDEX IF EXISTS idx_spaces_workspace_id;
			`,
		},
	}
}