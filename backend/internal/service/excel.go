package service

import (
	"bytes"
	"fmt"
	"log"
	"os"

	"github.com/cleberrangel/clickup-excel-api/internal/model"
	"github.com/cleberrangel/clickup-excel-api/internal/repository"
	"github.com/xuri/excelize/v2"
)

const sheetName = "Relatório"

// ExcelGenerator gera arquivos Excel
type ExcelGenerator struct {
	extractor *Extractor
}

// NewExcelGenerator cria um novo gerador de Excel
func NewExcelGenerator() *ExcelGenerator {
	return &ExcelGenerator{
		extractor: NewExtractor(),
	}
}

// Generate gera um arquivo Excel a partir das tarefas e campos solicitados
func (g *ExcelGenerator) Generate(tasks []model.Task, fields []string) (*bytes.Buffer, error) {
	f := excelize.NewFile()
	defer f.Close()

	// Renomeia a sheet padrão
	defaultSheet := f.GetSheetName(0)
	if err := f.SetSheetName(defaultSheet, sheetName); err != nil {
		return nil, fmt.Errorf("renomear sheet: %w", err)
	}

	// Resolve headers dinamicamente
	headers := g.resolveHeaders(fields, tasks)

	// Escreve cabeçalhos
	if err := g.writeHeaders(f, headers); err != nil {
		return nil, fmt.Errorf("escrever headers: %w", err)
	}

	// Escreve dados
	if err := g.writeData(f, tasks, fields); err != nil {
		return nil, fmt.Errorf("escrever dados: %w", err)
	}

	// Ajusta largura das colunas
	if err := g.autoFitColumns(f, len(headers)); err != nil {
		return nil, fmt.Errorf("ajustar colunas: %w", err)
	}

	// Escreve para buffer
	buf := new(bytes.Buffer)
	if err := f.Write(buf); err != nil {
		return nil, fmt.Errorf("escrever buffer: %w", err)
	}

	return buf, nil
}

// resolveHeaders resolve os nomes das colunas
func (g *ExcelGenerator) resolveHeaders(fields []string, tasks []model.Task) []string {
	headers := make([]string, len(fields))

	// Usa a primeira tarefa para resolver campos personalizados
	var sampleTask model.Task
	if len(tasks) > 0 {
		sampleTask = tasks[0]
	}

	for i, field := range fields {
		headers[i] = g.extractor.ResolveHeader(field, sampleTask)
	}

	return headers
}

// writeHeaders escreve os cabeçalhos no Excel
func (g *ExcelGenerator) writeHeaders(f *excelize.File, headers []string) error {
	// Estilo do cabeçalho
	style, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold:  true,
			Size:  11,
			Color: "FFFFFF",
		},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"4472C4"},
			Pattern: 1,
		},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
		},
	})
	if err != nil {
		return err
	}

	for col, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		if err := f.SetCellValue(sheetName, cell, header); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheetName, cell, cell, style); err != nil {
			return err
		}
	}

	return nil
}

// writeData escreve os dados das tarefas no Excel
func (g *ExcelGenerator) writeData(f *excelize.File, tasks []model.Task, fields []string) error {
	// Estilo alternado para linhas
	styleOdd, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"F2F2F2"},
			Pattern: 1,
		},
		Border: []excelize.Border{
			{Type: "left", Color: "D9D9D9", Style: 1},
			{Type: "top", Color: "D9D9D9", Style: 1},
			{Type: "bottom", Color: "D9D9D9", Style: 1},
			{Type: "right", Color: "D9D9D9", Style: 1},
		},
	})

	styleEven, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"FFFFFF"},
			Pattern: 1,
		},
		Border: []excelize.Border{
			{Type: "left", Color: "D9D9D9", Style: 1},
			{Type: "top", Color: "D9D9D9", Style: 1},
			{Type: "bottom", Color: "D9D9D9", Style: 1},
			{Type: "right", Color: "D9D9D9", Style: 1},
		},
	})

	for row, task := range tasks {
		excelRow := row + 2 // Linha 1 é header

		style := styleEven
		if row%2 == 1 {
			style = styleOdd
		}

		for col, field := range fields {
			cell, _ := excelize.CoordinatesToCellName(col+1, excelRow)
			value := g.extractor.ExtractValue(field, task)

			if err := f.SetCellValue(sheetName, cell, value); err != nil {
				return err
			}
			if err := f.SetCellStyle(sheetName, cell, cell, style); err != nil {
				return err
			}
		}
	}

	return nil
}

// autoFitColumns ajusta a largura das colunas
func (g *ExcelGenerator) autoFitColumns(f *excelize.File, numCols int) error {
	for col := 1; col <= numCols; col++ {
		colName, _ := excelize.ColumnNumberToName(col)
		// Largura mínima de 15, máxima de 50
		if err := f.SetColWidth(sheetName, colName, colName, 20); err != nil {
			return err
		}
	}
	return nil
}

// GenerateFromStorage gera Excel lendo tasks do storage via streaming (baixo consumo de memória)
func (g *ExcelGenerator) GenerateFromStorage(storage *repository.TaskStorage, fields []string) (string, error) {
	f := excelize.NewFile()
	defer f.Close()

	// Renomeia a sheet padrão
	defaultSheet := f.GetSheetName(0)
	if err := f.SetSheetName(defaultSheet, sheetName); err != nil {
		return "", fmt.Errorf("renomear sheet: %w", err)
	}

	// Cria iterador para ler tasks em streaming
	iter, err := storage.NewIterator()
	if err != nil {
		return "", fmt.Errorf("criar iterador: %w", err)
	}
	defer iter.Close()

	// Lê primeira task para resolver headers
	var firstTask model.Task
	hasFirst := false
	if iter.Next() {
		firstTask = iter.Task()
		hasFirst = true
	}

	// Resolve headers
	headers := g.resolveHeadersFromTask(fields, firstTask)

	// Escreve cabeçalhos
	if err := g.writeHeaders(f, headers); err != nil {
		return "", fmt.Errorf("escrever headers: %w", err)
	}

	// Cria estilos
	styleOdd, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"F2F2F2"},
			Pattern: 1,
		},
		Border: []excelize.Border{
			{Type: "left", Color: "D9D9D9", Style: 1},
			{Type: "top", Color: "D9D9D9", Style: 1},
			{Type: "bottom", Color: "D9D9D9", Style: 1},
			{Type: "right", Color: "D9D9D9", Style: 1},
		},
	})

	styleEven, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"FFFFFF"},
			Pattern: 1,
		},
		Border: []excelize.Border{
			{Type: "left", Color: "D9D9D9", Style: 1},
			{Type: "top", Color: "D9D9D9", Style: 1},
			{Type: "bottom", Color: "D9D9D9", Style: 1},
			{Type: "right", Color: "D9D9D9", Style: 1},
		},
	})

	// Escreve primeira task se existir
	row := 0
	if hasFirst {
		if err := g.writeTaskRow(f, firstTask, fields, row, styleEven, styleOdd); err != nil {
			return "", err
		}
		row++
	}

	// Escreve restante das tasks em streaming
	for iter.Next() {
		task := iter.Task()
		if err := g.writeTaskRow(f, task, fields, row, styleEven, styleOdd); err != nil {
			return "", err
		}
		row++

		// Log de progresso a cada 1000 tasks
		if row%1000 == 0 {
			log.Printf("[Excel] Processadas %d tarefas...", row)
		}
	}

	if err := iter.Err(); err != nil {
		return "", fmt.Errorf("erro ao iterar tasks: %w", err)
	}

	log.Printf("[Excel] Total de %d tarefas escritas no Excel", row)

	// Ajusta largura das colunas
	if err := g.autoFitColumns(f, len(headers)); err != nil {
		return "", fmt.Errorf("ajustar colunas: %w", err)
	}

	// Salva em arquivo temporário
	tmpFile, err := os.CreateTemp("", "report_*.xlsx")
	if err != nil {
		return "", fmt.Errorf("criar arquivo temp: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	if err := f.SaveAs(tmpPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("salvar excel: %w", err)
	}

	log.Printf("[Excel] Arquivo salvo em: %s", tmpPath)
	return tmpPath, nil
}

// writeTaskRow escreve uma linha de task no Excel
func (g *ExcelGenerator) writeTaskRow(f *excelize.File, task model.Task, fields []string, row int, styleEven, styleOdd int) error {
	excelRow := row + 2 // Linha 1 é header

	style := styleEven
	if row%2 == 1 {
		style = styleOdd
	}

	for col, field := range fields {
		cell, _ := excelize.CoordinatesToCellName(col+1, excelRow)
		value := g.extractor.ExtractValue(field, task)

		if err := f.SetCellValue(sheetName, cell, value); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheetName, cell, cell, style); err != nil {
			return err
		}
	}

	return nil
}

// resolveHeadersFromTask resolve headers a partir de uma task
func (g *ExcelGenerator) resolveHeadersFromTask(fields []string, task model.Task) []string {
	headers := make([]string, len(fields))
	for i, field := range fields {
		headers[i] = g.extractor.ResolveHeader(field, task)
	}
	return headers
}
