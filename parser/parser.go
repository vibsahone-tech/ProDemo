package parser

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"csv-upload-parser/model"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ParseError describes validation failures for a single CSV data row.
type ParseError struct {
	Row    int      `json:"row"`    // 1-based data row number (excluding header)
	Errors []string `json:"errors"` // list of error messages for this row
}

// ParseCSV parses and validates a CSV byte slice.
//
// maxRows limits the number of data rows (excluding the header). Pass 0 to
// disable the limit.
//
// It processes every row regardless of validation failures and collects all
// errors. Invalid rows are excluded from the returned slices — the caller
// receives only the rows that passed validation.
//
// Returns:
//   - groups     — unique RegisterGroups extracted from valid rows
//   - registers  — valid Register documents ready for DB insertion
//   - parseErrs  — per-row validation errors (empty if everything is valid)
//   - err        — a fatal IO / CSV format error that aborted parsing entirely
func ParseCSV(data []byte, maxRows int) ([]model.RegisterGroup, []model.Register, []ParseError, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	headers, err := reader.Read()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read CSV header: %w", err)
	}
	normalizeHeaders(headers)

	groupMap := map[string]*model.RegisterGroup{}
	var registers []model.Register
	var parseErrs []ParseError

	seenNames := make(map[string]bool) // for uniqueness check across all rows
	rowNum := 0

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to read CSV row %d: %w", rowNum+1, err)
		}
		rowNum++

		// Enforce max data rows.
		if maxRows > 0 && rowNum > maxRows {
			return nil, nil, nil, fmt.Errorf("CSV exceeds maximum allowed data rows (%d)", maxRows)
		}

		record := buildRowMap(headers, row)
		reg := buildRegister(record)

		if errs := validateRegister(rowNum, reg, seenNames); len(errs) > 0 {
			parseErrs = append(parseErrs, ParseError{Row: rowNum, Errors: errs})
			continue // skip invalid rows
		}

		registers = append(registers, reg)

		groupName := record["GroupName"]
		if groupName != "" {
			normalizedGroupName := strings.ToLower(groupName)
			groupSeqNo := atoi(record["GroupSeqNo"])
			if g, ok := groupMap[normalizedGroupName]; ok {
				if g.SeqNo != groupSeqNo {
					parseErrs = append(parseErrs, ParseError{
						Row:    rowNum,
						Errors: []string{fmt.Sprintf("GroupName: consistency error; group %q (case-insensitive) already exists with SeqNo %d, but this row has %d", groupName, g.SeqNo, groupSeqNo)},
					})
					continue
				}
			} else {
				groupMap[normalizedGroupName] = &model.RegisterGroup{
					Name:  groupName, // Keep original case for the first occurrence
					SeqNo: groupSeqNo,
				}
			}
			groupMap[normalizedGroupName].RegisterIds = append(groupMap[normalizedGroupName].RegisterIds, reg.ID)
		}
	}

	groups := make([]model.RegisterGroup, 0, len(groupMap))
	for _, g := range groupMap {
		groups = append(groups, *g)
	}

	return groups, registers, parseErrs, nil
}

// buildRegister constructs a Register from a CSV row map.
// No validation is done here; that is handled by validateRegister.
func buildRegister(row map[string]string) model.Register {
	reg := model.Register{
		ID:           bson.NewObjectID(),
		Name:         row["RegisterName"],
		Label:        row["Label"],
		SeqNo:        atoi(row["GroupSeqNo"]),
		Category:     model.RegisterCategory(atoi(row["Category"])),
		RegisterType: model.RegisterType(atoi(row["RegisterType"])),
		AddressStart: atoi(row["AddressStart"]),
		Length:       atoi(row["Length"]),
		AccessType:   model.RegisterAccessType(atoi(row["AccessType"])),
		DataType:     model.RegisterDataType(atoi(row["DataType"])),
		SubDataType:  model.RegisterSubType(atoi(row["SubDataType"])),
		Scale:        atof(row["Scale"]),
		Unit:         row["Unit"],
	}

	reg.ByteOrder = model.ByteOrder(atoi(row["ByteOrder"]))
	reg.WordOrder = model.WordOrder(atoi(row["WordOrder"]))

	reg.EnumMap = parseEnumMap(row["EnumMap"])
	reg.BitMap = parseBitMap(row["BitMap"])

	if c := parseConstraints(row["Constraints"]); !c.IsZero() {
		reg.Constraints = c
	}

	if e := parseExecution(row["Execution"]); !e.IsZero() {
		reg.Execution = e
	}

	return reg
}

// ── Complex field parsers ───────────────────────────────────────────────────

// parseEnumMap parses "value=label;value=label;..." into []EnumMapping.
func parseEnumMap(raw string) []model.EnumMapping {
	if raw == "" {
		return nil
	}
	items := strings.Split(raw, ";")
	result := make([]model.EnumMapping, 0, len(items))
	for _, item := range items {
		key, label, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		v, err := strconv.Atoi(strings.TrimSpace(key))
		if err != nil {
			continue
		}
		result = append(result, model.EnumMapping{
			Value: v,
			Label: strings.TrimSpace(label),
		})
	}
	return result
}

// parseBitMap parses "bit=label:severity|..." into []BitMapping.
func parseBitMap(raw string) []model.BitMapping {
	if raw == "" {
		return nil
	}
	items := strings.Split(raw, "|")
	result := make([]model.BitMapping, 0, len(items))
	for _, item := range items {
		bitStr, rest, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		bit, err := strconv.Atoi(strings.TrimSpace(bitStr))
		if err != nil {
			continue
		}
		m := model.BitMapping{Bit: bit}
		label, severity, hasSeverity := strings.Cut(rest, ":")
		m.Label = strings.TrimSpace(label)
		if hasSeverity {
			switch strings.ToLower(strings.TrimSpace(severity)) {
			case "info":
				m.Severity = model.BitSeverityInfo
			case "warning":
				m.Severity = model.BitSeverityWarning
			case "critical":
				m.Severity = model.BitSeverityCritical
			}
		}
		result = append(result, m)
	}
	return result
}

// parseConstraints parses "min=N;max=N;step=N" into Constraints.
func parseConstraints(raw string) model.Constraints {
	if raw == "" {
		return model.Constraints{}
	}
	var c model.Constraints
	for _, item := range strings.Split(raw, ";") {
		key, valStr, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		val, err := strconv.ParseFloat(strings.TrimSpace(valStr), 64)
		if err != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "min":
			c.Min = val
		case "max":
			c.Max = val
		case "step":
			c.Step = val
		}
	}
	return c
}

// parseExecution parses "triggerValue:autoReset" (e.g. "1:true") into ExecutionConfig.
func parseExecution(raw string) model.ExecutionConfig {
	if raw == "" {
		return model.ExecutionConfig{}
	}
	valStr, resetStr, ok := strings.Cut(raw, ":")
	if !ok {
		return model.ExecutionConfig{}
	}
	val, err := strconv.Atoi(strings.TrimSpace(valStr))
	if err != nil {
		return model.ExecutionConfig{}
	}
	return model.ExecutionConfig{
		TriggerValue: val,
		AutoReset:    strings.TrimSpace(resetStr) == "true",
	}
}

// ── Header / row helpers ────────────────────────────────────────────────────

// normalizeHeaders trims whitespace and strips the BOM in-place.
func normalizeHeaders(h []string) {
	for i, v := range h {
		v = strings.TrimSpace(v)
		v = strings.ReplaceAll(v, "\ufeff", "")
		h[i] = v
	}
}

// buildRowMap zips headers and row values into a map. Missing cells default to "".
func buildRowMap(headers, row []string) map[string]string {
	record := make(map[string]string, len(headers))
	for i, h := range headers {
		if i < len(row) {
			record[h] = strings.TrimSpace(row[i])
		} else {
			record[h] = ""
		}
	}
	return record
}

// ── Primitive parsers ───────────────────────────────────────────────────────

func atoi(s string) int {
	if s == "" {
		return 0
	}
	v, _ := strconv.Atoi(s)
	return v
}

func atof(s string) float64 {
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
