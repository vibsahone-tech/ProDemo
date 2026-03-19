package parser

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"csv-upload-parser/model"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ParseError describes validation failures for a single CSV data row.
type ParseError struct {
	Row    int      `json:"row"`
	Errors []string `json:"errors"`
}

// ── Compiled Regexes (once at startup) ─────────────────────────────────────

var (
	// "0=Off" or "1=On Standby" — label cannot contain = or ;
	enumItemRegex = regexp.MustCompile(`^(\d+)=([^;=]+)$`)

	// "0=Alarm:critical" or "1=Info" — severity is optional
	bitMapItemRegex = regexp.MustCompile(`^(\d+)=([^|=:]+)(?::(info|warning|critical))?$`)

	// "min=10" or "max=-5.5" or "step=0.1"
	constraintItemRegex = regexp.MustCompile(`^(?i)(min|max|step)=(-?\d+(?:\.\d+)?)$`)

	// "1:true" or "-5:false"
	executionRegex = regexp.MustCompile(`^(-?\d+):(true|false)$`)
)

// ── ParseCSV ────────────────────────────────────────────────────────────────

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

	seenNames := make(map[string]bool)
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

		if maxRows > 0 && rowNum > maxRows {
			return nil, nil, nil, fmt.Errorf("CSV exceeds maximum allowed data rows (%d)", maxRows)
		}

		record := buildRowMap(headers, row)
		reg, fmtErrs := buildRegister(record)

		validErrs := validateRegister(rowNum, reg, seenNames)
		allErrs := append(fmtErrs, validErrs...)
		if len(allErrs) > 0 {
			parseErrs = append(parseErrs, ParseError{Row: rowNum, Errors: allErrs})
			continue
		}

		registers = append(registers, reg)

		groupName := record["GroupName"]
		if groupName != "" {
			normalizedGroupName := strings.ToLower(groupName)
			groupSeqNo := atoi(record["GroupSeqNo"])
			if g, ok := groupMap[normalizedGroupName]; ok {
				if g.SeqNo != groupSeqNo {
					parseErrs = append(parseErrs, ParseError{
						Row: rowNum,
						Errors: []string{fmt.Sprintf(
							"GroupName: consistency error; group %q already exists with SeqNo %d, but this row has %d",
							groupName, g.SeqNo, groupSeqNo,
						)},
					})
					continue
				}
			} else {
				groupMap[normalizedGroupName] = &model.RegisterGroup{
					Name:  groupName,
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

// ── buildRegister ───────────────────────────────────────────────────────────

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
		WordOrder:    model.WordOrder(atoi(row["WordOrder"])),
		ByteOrder:    model.ByteOrder(atoi(row["ByteOrder"])),
	}

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
func parseEnumMap(raw string) ([]model.EnumMapping, []string) {
	const correctFormat = `correct format: "<int>=<label>;<int>=<label>;..." e.g. "0=Off;1=On;2=Standby"`
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	items := strings.Split(raw, ";")
	result := make([]model.EnumMapping, 0, len(items))
	seen := make(map[int]bool)
	var errs []string

	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		matches := enumItemRegex.FindStringSubmatch(item)
		if matches == nil {
			errs = append(errs, fmt.Sprintf(`EnumMap: invalid item %q; %s`, item, correctFormat))
			continue
		}
		v, _ := strconv.Atoi(matches[1])
		label := strings.TrimSpace(matches[2])

		if seen[v] {
			errs = append(errs, fmt.Sprintf(`EnumMap: duplicate value %d found; %s`, v, correctFormat))
			continue
		}
		seen[v] = true

		result = append(result, model.EnumMapping{Value: v, Label: label})
	}
	return result, errs
}

// parseBitMap parses "bit=label:severity|..." into []BitMapping.
func parseBitMap(raw string) ([]model.BitMapping, []string) {
	const correctFormat = `correct format: "<bit>=<label>:<severity>|..." e.g. "0=Alarm:critical|1=Warning:warning|2=Info:info" (severity: info/warning/critical, optional)`
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	items := strings.Split(raw, "|")
	result := make([]model.BitMapping, 0, len(items))
	seen := make(map[int]bool)
	var errs []string

	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		matches := bitMapItemRegex.FindStringSubmatch(item)
		if matches == nil {
			errs = append(errs, fmt.Sprintf(`BitMap: invalid item %q; %s`, item, correctFormat))
			continue
		}
		bit, _ := strconv.Atoi(matches[1])
		label := strings.TrimSpace(matches[2])
		severityRaw := strings.ToLower(strings.TrimSpace(matches[3]))

		if seen[bit] {
			errs = append(errs, fmt.Sprintf(`BitMap: duplicate bit position %d found; %s`, bit, correctFormat))
			continue
		}
		seen[bit] = true

		m := model.BitMapping{
			Bit:   bit,
			Label: label,
		}
		switch severityRaw {
		case "info":
			m.Severity = model.BitSeverityInfo
		case "warning":
			m.Severity = model.BitSeverityWarning
		case "critical":
			m.Severity = model.BitSeverityCritical
		}

		result = append(result, m)
	}
	return result, errs
}

// parseConstraints parses "min=N;max=N;step=N" into Constraints.
func parseConstraints(raw string) (model.Constraints, []string) {
	const correctFormat = `correct format: "min=<number>;max=<number>;step=<number>" e.g. "min=0;max=100;step=1"`
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.Constraints{}, nil
	}

	var c model.Constraints
	seen := make(map[string]bool)
	var errs []string

	for _, item := range strings.Split(raw, ";") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		matches := constraintItemRegex.FindStringSubmatch(item)
		if matches == nil {
			errs = append(errs, fmt.Sprintf(`Constraints: invalid item %q; %s`, item, correctFormat))
			continue
		}
		key := strings.ToLower(matches[1])
		val, _ := strconv.ParseFloat(matches[2], 64)

		if seen[key] {
			errs = append(errs, fmt.Sprintf(`Constraints: duplicate key %q found; %s`, key, correctFormat))
			continue
		}
		seen[key] = true

		switch key {
		case "min":
			c.Min = val
		case "max":
			c.Max = val
		case "step":
			c.Step = val
		}
	}
	return c, errs
}

// parseExecution parses "triggerValue:autoReset" (e.g. "1:true") into ExecutionConfig.
func parseExecution(raw string) (model.ExecutionConfig, []string) {
	const correctFormat = `correct format: "<int>:<bool>" e.g. "1:true" or "1:false"`
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.ExecutionConfig{}, nil
	}

	matches := executionRegex.FindStringSubmatch(raw)
	if matches == nil {
		return model.ExecutionConfig{}, []string{
			fmt.Sprintf(`Execution: invalid format %q; %s`, raw, correctFormat),
		}
	}

	val, _ := strconv.Atoi(matches[1])
	autoReset := matches[2] == "true"

	return model.ExecutionConfig{
		TriggerValue: val,
		AutoReset:    autoReset,
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
func buildRowMap(headers []string, row []string) map[string]string {
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
