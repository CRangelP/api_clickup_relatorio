package repository

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"sync/atomic"

	"github.com/cleberrangel/clickup-excel-api/internal/model"
)

// DataDir é o diretório para arquivos temporários (pode ser montado como volume)
// Usa variável de ambiente DATA_DIR ou fallback para /tmp
var DataDir = getDataDir()

func getDataDir() string {
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		// Garante que o diretório existe
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("[Storage] Aviso: não foi possível criar DATA_DIR %s: %v, usando /tmp", dir, err)
			return os.TempDir()
		}
		log.Printf("[Storage] Usando DATA_DIR: %s", dir)
		return dir
	}
	return os.TempDir()
}

// requestCounter é um contador atômico para gerar IDs únicos por requisição
var requestCounter int32

// nextRequestID retorna o próximo ID de requisição (thread-safe)
func nextRequestID() int32 {
	return atomic.AddInt32(&requestCounter, 1)
}

// TaskStorage gerencia persistência temporária de tasks em disco
type TaskStorage struct {
	mu         sync.Mutex
	file       *os.File
	filePath   string
	encoder    *json.Encoder
	taskCount  int
	folderName string
	requestID  int32
}

// NewTaskStorage cria um novo storage temporário com ID único
func NewTaskStorage() (*TaskStorage, error) {
	// Gera ID único para esta requisição
	reqID := nextRequestID()

	// Cria arquivo temporário com ID único no DataDir
	file, err := os.CreateTemp(DataDir, fmt.Sprintf("clickup_tasks_%d_*.jsonl", reqID))
	if err != nil {
		return nil, fmt.Errorf("criar arquivo temporário: %w", err)
	}

	filePath := file.Name()
	log.Printf("[Storage] Requisição #%d - Arquivo temporário criado: %s", reqID, filePath)

	return &TaskStorage{
		file:      file,
		filePath:  filePath,
		encoder:   json.NewEncoder(file),
		requestID: reqID,
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

	log.Printf("[Storage] Requisição #%d - Arquivo temporário removido: %s", s.requestID, s.filePath)
	return nil
}

// GetRequestID retorna o ID da requisição
func (s *TaskStorage) GetRequestID() int32 {
	return s.requestID
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
