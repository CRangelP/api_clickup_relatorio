package repository

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/cleberrangel/clickup-excel-api/internal/model"
)

// TaskStorage gerencia persistência temporária de tasks em disco
type TaskStorage struct {
	mu         sync.Mutex
	file       *os.File
	filePath   string
	encoder    *json.Encoder
	taskCount  int
	folderName string
}

// NewTaskStorage cria um novo storage temporário
func NewTaskStorage() (*TaskStorage, error) {
	// Cria arquivo temporário
	tmpDir := os.TempDir()
	filePath := filepath.Join(tmpDir, fmt.Sprintf("clickup_tasks_%d.jsonl", os.Getpid()))

	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("criar arquivo temporário: %w", err)
	}

	log.Printf("[Storage] Arquivo temporário criado: %s", filePath)

	return &TaskStorage{
		file:     file,
		filePath: filePath,
		encoder:  json.NewEncoder(file),
	}, nil
}

// AppendTasks adiciona tasks ao storage (append, não carrega em memória)
func (s *TaskStorage) AppendTasks(tasks []model.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, task := range tasks {
		if err := s.encoder.Encode(task); err != nil {
			return fmt.Errorf("encode task: %w", err)
		}
		s.taskCount++

		// Captura folder_name da primeira task
		if s.folderName == "" && task.Folder.Name != "" {
			s.folderName = task.Folder.Name
		}
	}

	// Flush para garantir escrita em disco
	if err := s.file.Sync(); err != nil {
		return fmt.Errorf("sync file: %w", err)
	}

	return nil
}

// GetTaskCount retorna o número total de tasks armazenadas
func (s *TaskStorage) GetTaskCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.taskCount
}

// GetFolderName retorna o nome da pasta (capturado da primeira task)
func (s *TaskStorage) GetFolderName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.folderName
}

// TaskIterator permite iterar sobre tasks sem carregar tudo em memória
type TaskIterator struct {
	file    *os.File
	scanner *bufio.Scanner
	current model.Task
	err     error
}

// NewIterator cria um iterador para ler tasks em streaming
func (s *TaskStorage) NewIterator() (*TaskIterator, error) {
	// Fecha o arquivo de escrita
	if err := s.file.Close(); err != nil {
		return nil, fmt.Errorf("fechar arquivo de escrita: %w", err)
	}

	// Abre para leitura
	file, err := os.Open(s.filePath)
	if err != nil {
		return nil, fmt.Errorf("abrir arquivo para leitura: %w", err)
	}

	scanner := bufio.NewScanner(file)
	// Aumenta buffer para linhas grandes (tasks com muitos campos)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	return &TaskIterator{
		file:    file,
		scanner: scanner,
	}, nil
}

// Next avança para a próxima task
func (it *TaskIterator) Next() bool {
	if !it.scanner.Scan() {
		it.err = it.scanner.Err()
		return false
	}

	if err := json.Unmarshal(it.scanner.Bytes(), &it.current); err != nil {
		it.err = fmt.Errorf("unmarshal task: %w", err)
		return false
	}

	return true
}

// Task retorna a task atual
func (it *TaskIterator) Task() model.Task {
	return it.current
}

// Err retorna erro se houver
func (it *TaskIterator) Err() error {
	return it.err
}

// Close fecha o iterador
func (it *TaskIterator) Close() error {
	return it.file.Close()
}

// Close fecha e remove o arquivo temporário
func (s *TaskStorage) Close() error {
	// Tenta fechar o arquivo (pode já estar fechado)
	s.file.Close()

	// Remove o arquivo temporário
	if err := os.Remove(s.filePath); err != nil && !os.IsNotExist(err) {
		log.Printf("[Storage] Aviso: não foi possível remover arquivo temporário: %v", err)
		return err
	}

	log.Printf("[Storage] Arquivo temporário removido: %s", s.filePath)
	return nil
}

// GetFilePath retorna o caminho do arquivo (para debug)
func (s *TaskStorage) GetFilePath() string {
	return s.filePath
}

// ReadAllTasks lê todas as tasks (usar apenas para testes ou volumes pequenos)
func (s *TaskStorage) ReadAllTasks() ([]model.Task, error) {
	iter, err := s.NewIterator()
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var tasks []model.Task
	for iter.Next() {
		tasks = append(tasks, iter.Task())
	}

	if err := iter.Err(); err != nil && err != io.EOF {
		return nil, err
	}

	return tasks, nil
}
