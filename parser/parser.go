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
		reg, fmtErrs := buildRegister(record)

		// Merge format-parse errors with field-level validation errors.
		validErrs := validateRegister(rowNum, reg, seenNames)
		allErrs := append(fmtErrs, validErrs...)
		if len(allErrs) > 0 {
			parseErrs = append(parseErrs, ParseError{Row: rowNum, Errors: allErrs})
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
// It returns the Register and any format-level errors from complex field parsing.
// Field-level semantic validation is handled separately by validateRegister.
func buildRegister(row map[string]string) (model.Register, []string) {
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

	var fmtErrs []string

	enumMap, errs := parseEnumMap(row["EnumMap"])
	reg.EnumMap = enumMap
	fmtErrs = append(fmtErrs, errs...)

	bitMap, errs := parseBitMap(row["BitMap"])
	reg.BitMap = bitMap
	fmtErrs = append(fmtErrs, errs...)

	c, errs := parseConstraints(row["Constraints"])
	if !c.IsZero() {
		reg.Constraints = c
	}
	fmtErrs = append(fmtErrs, errs...)

	e, errs := parseExecution(row["Execution"])
	if !e.IsZero() {
		reg.Execution = e
	}
	fmtErrs = append(fmtErrs, errs...)

	return reg, fmtErrs
}

// ── Complex field parsers ───────────────────────────────────────────────────

// parseEnumMap parses "value=label;value=label;..." into []EnumMapping.
// Returns both the parsed result and any format errors encountered.
func parseEnumMap(raw string) ([]model.EnumMapping, []string) {
	const correctFormat = `correct format: "<int>=<label>;<int>=<label>;..." e.g. "0=Off;1=On;2=Standby"`
	if raw == "" {
		return nil, nil
	}
	items := strings.Split(raw, ";")
	result := make([]model.EnumMapping, 0, len(items))
	var errs []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key, label, ok := strings.Cut(item, "=")
		if !ok {
			errs = append(errs, fmt.Sprintf(`EnumMap: item %q is missing "=" separator; %s`, item, correctFormat))
			continue
		}
		key = strings.TrimSpace(key)
		label = strings.TrimSpace(label)
		v, err := strconv.Atoi(key)
		if err != nil {
			errs = append(errs, fmt.Sprintf(`EnumMap: value %q in item %q must be an integer; %s`, key, item, correctFormat))
			continue
		}
		if label == "" {
			errs = append(errs, fmt.Sprintf(`EnumMap: label is empty for value %d; %s`, v, correctFormat))
			continue
		}
		result = append(result, model.EnumMapping{Value: v, Label: label})
	}
	return result, errs
}

// parseBitMap parses "bit=label:severity|..." into []BitMapping.
// Returns both the parsed result and any format errors encountered.
func parseBitMap(raw string) ([]model.BitMapping, []string) {
	const correctFormat = `correct format: "<bit>=<label>:<severity>|..." e.g. "0=Alarm:critical|1=Warning:warning|2=Info:info" (severity: info/warning/critical, optional)`
	if raw == "" {
		return nil, nil
	}
	items := strings.Split(raw, "|")
	result := make([]model.BitMapping, 0, len(items))
	var errs []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		bitStr, rest, ok := strings.Cut(item, "=")
		if !ok {
			errs = append(errs, fmt.Sprintf(`BitMap: item %q is missing "=" separator; %s`, item, correctFormat))
			continue
		}
		bitStr = strings.TrimSpace(bitStr)
		bit, err := strconv.Atoi(bitStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf(`BitMap: bit position %q in item %q must be an integer; %s`, bitStr, item, correctFormat))
			continue
		}
		m := model.BitMapping{Bit: bit}
		label, severity, hasSeverity := strings.Cut(rest, ":")
		m.Label = strings.TrimSpace(label)
		if m.Label == "" {
			errs = append(errs, fmt.Sprintf(`BitMap: label is empty for bit %d; %s`, bit, correctFormat))
			continue
		}
		if hasSeverity {
			sev := strings.ToLower(strings.TrimSpace(severity))
			switch sev {
			case "info":
				m.Severity = model.BitSeverityInfo
			case "warning":
				m.Severity = model.BitSeverityWarning
			case "critical":
				m.Severity = model.BitSeverityCritical
			default:
				errs = append(errs, fmt.Sprintf(`BitMap: unknown severity %q for bit %d (valid: info/warning/critical); %s`, sev, bit, correctFormat))
				continue
			}
		}
		result = append(result, m)
	}
	return result, errs
}

// parseConstraints parses "min=N;max=N;step=N" into Constraints.
// Returns both the parsed result and any format errors encountered.
func parseConstraints(raw string) (model.Constraints, []string) {
	const correctFormat = `correct format: "min=<number>;max=<number>;step=<number>" e.g. "min=0;max=100;step=1"`
	if raw == "" {
		return model.Constraints{}, nil
	}
	var c model.Constraints
	var errs []string
	for _, item := range strings.Split(raw, ";") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key, valStr, ok := strings.Cut(item, "=")
		if !ok {
			errs = append(errs, fmt.Sprintf(`Constraints: item %q is missing "=" separator; %s`, item, correctFormat))
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		valStr = strings.TrimSpace(valStr)
		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			errs = append(errs, fmt.Sprintf(`Constraints: value %q for key %q must be a number; %s`, valStr, key, correctFormat))
			continue
		}
		switch key {
		case "min":
			c.Min = val
		case "max":
			c.Max = val
		case "step":
			c.Step = val
		default:
			errs = append(errs, fmt.Sprintf(`Constraints: unknown key %q (valid keys: min/max/step); %s`, key, correctFormat))
		}
	}
	return c, errs
}

// parseExecution parses "triggerValue:autoReset" (e.g. "1:true") into ExecutionConfig.
// Returns both the parsed result and any format errors encountered.
func parseExecution(raw string) (model.ExecutionConfig, []string) {
	const correctFormat = `correct format: "<int>:<bool>" e.g. "1:true" or "1:false"`
	if raw == "" {
		return model.ExecutionConfig{}, nil
	}
	valStr, resetStr, ok := strings.Cut(raw, ":")
	if !ok {
		return model.ExecutionConfig{}, []string{
			fmt.Sprintf(`Execution: %q is missing ":" separator; %s`, raw, correctFormat),
		}
	}
	valStr = strings.TrimSpace(valStr)
	resetStr = strings.TrimSpace(resetStr)
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return model.ExecutionConfig{}, []string{
			fmt.Sprintf(`Execution: trigger value %q must be an integer; %s`, valStr, correctFormat),
		}
	}
	if resetStr != "true" && resetStr != "false" {
		return model.ExecutionConfig{}, []string{
			fmt.Sprintf(`Execution: autoReset %q must be "true" or "false"; %s`, resetStr, correctFormat),
		}
	}
	return model.ExecutionConfig{
		TriggerValue: val,
		AutoReset:    resetStr == "true",
	}, nil
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
