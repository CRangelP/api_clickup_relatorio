package service

import (
	"bytes"
	"fmt"

	"github.com/cleberrangel/clickup-excel-api/internal/model"
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
