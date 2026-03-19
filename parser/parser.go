package parser

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"csv-upload-parser/model"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const maxStrLen = 256

// ParseError describes validation failures for a single CSV data row.
type ParseError struct {
	Row    int      `json:"row"`
	Errors []string `json:"errors"`
}

// ParseCSV

// ParseCSV parses and validates a CSV byte slice.
//
// maxRows limits the number of data rows (excluding the header). Pass 0 to
// disable the limit.
// regexCfg provides the raw regex strings to compile during parsing.
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
	fmt.Printf("ParseCSV: Received %d bytes of CSV data", len(data))
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	headers, err := reader.Read()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read CSV header: %w", err)
	}
	// 1. Headers - trim whitespace and strip BOM
	for i, v := range headers {
		v = strings.TrimSpace(v)
		v = strings.ReplaceAll(v, "\ufeff", "")
		headers[i] = v
	}

	// 2. Headers - mandatory columns check (RegisterName, DataType, AddressStart)
	headerMap := make(map[string]bool, len(headers))
	for _, h := range headers {
		headerMap[strings.ToLower(h)] = true
	}
	var missing []string
	required := []string{
		"RegisterName", "Label", "Category", "RegisterType",
		"AddressStart", "Length", "AccessType", "DataType", "SubDataType",
		"GroupName", "GroupSeqNo",
	}
	for _, req := range required {
		if !headerMap[strings.ToLower(req)] {
			missing = append(missing, req)
		}
	}
	if len(missing) > 0 {
		return nil, nil, nil, fmt.Errorf("CSV header validation failed: missing required columns: %s", strings.Join(missing, ", "))
	}

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

		// 3. Row - trim cells and map to headers (case-insensitive keys)
		record := make(map[string]string, len(headers))
		for i, h := range headers {
			key := strings.ToLower(h)
			if i < len(row) {
				record[key] = strings.TrimSpace(row[i])
			} else {
				record[key] = ""
			}
		}

		reg, allErrs := buildRegister(record, seenNames)
		if len(allErrs) > 0 {
			parseErrs = append(parseErrs, ParseError{Row: rowNum, Errors: allErrs})
			continue
		}

		// 4. Groups and Sequence Assignment
		groupName := record["groupname"]
		if groupName != "" {
			normalizedGroupName := strings.ToLower(groupName)

			// Initialize group if new
			var groupSeqNo int
			if s := record["groupseqno"]; s != "" {
				if v, err := strconv.Atoi(s); err != nil {
					parseErrs = append(parseErrs, ParseError{
						Row:    rowNum,
						Errors: []string{fmt.Sprintf("GroupSeqNo: invalid integer format %q", s)},
					})
					continue
				} else if v <= 0 {
					parseErrs = append(parseErrs, ParseError{
						Row:    rowNum,
						Errors: []string{fmt.Sprintf("GroupSeqNo: must be positive (got %d)", v)},
					})
					continue
				} else {
					groupSeqNo = v
				}
			}

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

			// AUTO-GENERATE SEQNO: Use the current count of registers in this group + 1
			reg.SeqNo = len(groupMap[normalizedGroupName].RegisterIds) + 1

			// Link register to group
			groupMap[normalizedGroupName].RegisterIds = append(groupMap[normalizedGroupName].RegisterIds, reg.ID)
		}

		registers = append(registers, reg)
	}

	groups := make([]model.RegisterGroup, 0, len(groupMap))
	for _, g := range groupMap {
		groups = append(groups, *g)
	}

	return groups, registers, parseErrs, nil
}

// buildRegister constructs and validates a Register from a CSV row map.
// It returns the Register and a slice of all formatting/validation errors.
// Each validation step ensures the data conforms to the Modbus protocol
// and internal system requirements.
func buildRegister(row map[string]string, seenNames map[string]bool) (model.Register, []string) {
	var errs []string

	// 1. Name - trim, mandatory, length check, uniqueness check
	name := row["registername"]
	if name == "" {
		errs = append(errs, "RegisterName: mandatory field is missing")
	} else {
		if utf8.RuneCountInString(name) > maxStrLen {
			errs = append(errs, fmt.Sprintf("RegisterName: exceeds %d characters", maxStrLen))
		}
		if seenNames[name] {
			errs = append(errs, fmt.Sprintf("RegisterName: %q is not unique", name))
		}
		seenNames[name] = true
	}

	// 2. Label - mandatory, length check
	label := row["label"]
	if label == "" {
		errs = append(errs, "Label: mandatory field is missing")
	} else if utf8.RuneCountInString(label) > maxStrLen {
		errs = append(errs, fmt.Sprintf("Label: exceeds %d characters", maxStrLen))
	}

	// 3. Category - mandatory, enum range check
	var category int
	if s := row["category"]; s != "" {
		if v, err := strconv.Atoi(s); err != nil {
			errs = append(errs, fmt.Sprintf("Category: invalid integer format %q", s))
		} else if v <= 0 || v >= int(model.RegCategoryMax) {
			errs = append(errs, fmt.Sprintf("Category: invalid value %d (valid: 1–%d)", v, int(model.RegCategoryMax)-1))
		} else {
			category = v
		}
	} else {
		errs = append(errs, "Category: mandatory field is missing")
	}

	// 5. RegisterType - mandatory, enum range check
	var regType int
	if s := row["registertype"]; s != "" {
		if v, err := strconv.Atoi(s); err != nil {
			errs = append(errs, fmt.Sprintf("RegisterType: invalid integer format %q", s))
		} else if v <= 0 || v >= int(model.RegisterTypeMax) {
			errs = append(errs, fmt.Sprintf("RegisterType: invalid value %d (valid: 1–%d)", v, int(model.RegisterTypeMax)-1))
		} else {
			regType = v
		}
	} else {
		errs = append(errs, "RegisterType: mandatory field is missing")
	}

	// 6. AddressStart - mandatory, 16-bit range check
	var addrStart int
	if s := row["addressstart"]; s != "" {
		if v, err := strconv.Atoi(s); err != nil {
			errs = append(errs, fmt.Sprintf("AddressStart: invalid integer format %q", s))
		} else if v < 0 || v > 65535 {
			// Modbus registers are identified by 16-bit addresses.
			errs = append(errs, "AddressStart: must be between 0 and 65535")
		} else {
			addrStart = v
		}
	} else {
		errs = append(errs, "AddressStart: mandatory field is missing")
	}

	// 7. Length - mandatory, positive integer
	var length int
	if s := row["length"]; s != "" {
		if v, err := strconv.Atoi(s); err != nil {
			errs = append(errs, fmt.Sprintf("Length: invalid integer format %q", s))
		} else if v <= 0 {
			errs = append(errs, "Length: must be positive")
		} else {
			length = v
		}
	} else {
		errs = append(errs, "Length: mandatory field is missing")
	}

	// 8. AccessType - mandatory, enum range check
	var accessType int
	if s := row["accesstype"]; s != "" {
		if v, err := strconv.Atoi(s); err != nil {
			errs = append(errs, fmt.Sprintf("AccessType: invalid integer format %q", s))
		} else if v <= 0 || v >= int(model.RegAccessTypeMax) {
			errs = append(errs, fmt.Sprintf("AccessType: invalid value %d (valid: 1–%d)", v, int(model.RegAccessTypeMax)-1))
		} else {
			accessType = v
		}
	} else {
		errs = append(errs, "AccessType: mandatory field is missing")
	}

	// 9. DataType - mandatory, enum range check
	var dataType int
	if s := row["datatype"]; s != "" {
		if v, err := strconv.Atoi(s); err != nil {
			errs = append(errs, fmt.Sprintf("DataType: invalid integer format %q", s))
		} else if v <= 0 || v >= int(model.RegDataTypeMax) {
			errs = append(errs, fmt.Sprintf("DataType: invalid value %d (valid: 1–%d)", v, int(model.RegDataTypeMax)-1))
		} else {
			dataType = v
		}
	} else {
		errs = append(errs, "DataType: mandatory field is missing")
	}

	// 10. SubDataType - mandatory, enum range check
	var subDataType int
	if s := row["subdatatype"]; s != "" {
		if v, err := strconv.Atoi(s); err != nil {
			errs = append(errs, fmt.Sprintf("SubDataType: invalid integer format %q", s))
		} else if v <= 0 || v >= int(model.RegSubTypeMax) {
			errs = append(errs, fmt.Sprintf("SubDataType: invalid value %d (valid: 1–%d)", v, int(model.RegSubTypeMax)-1))
		} else {
			subDataType = v
		}
	} else {
		errs = append(errs, "SubDataType: mandatory field is missing")
	}

	// 11. Scale - optional, non-negative float
	var scale float64
	if s := row["scale"]; s != "" {
		if v, err := strconv.ParseFloat(s, 64); err != nil {
			errs = append(errs, fmt.Sprintf("Scale: invalid number format %q", s))
		} else if v < 0 {
			errs = append(errs, "Scale: must be non-negative")
		} else {
			scale = v
		}
	}

	// 12. WordOrder - optional, enum range check, only for Length=2
	var wordOrder int
	if s := row["wordorder"]; s != "" {
		if v, err := strconv.Atoi(s); err != nil {
			errs = append(errs, fmt.Sprintf("WordOrder: invalid integer format %q", s))
		} else if v < 0 || v >= int(model.WordOrderMax) {
			errs = append(errs, fmt.Sprintf("WordOrder: invalid value %d (valid: 1–%d)", v, int(model.WordOrderMax)-1))
		} else if v > 0 && length != 2 {
			errs = append(errs, fmt.Sprintf("WordOrder: only for Length=2, but Length is %d", length))
		} else {
			wordOrder = v
		}
	}

	// 13. ByteOrder - optional, enum range check
	var byteOrder int
	if s := row["byteorder"]; s != "" {
		if v, err := strconv.Atoi(s); err != nil {
			errs = append(errs, fmt.Sprintf("ByteOrder: invalid integer format %q", s))
		} else if v < 0 || v >= int(model.ByteOrderMax) {
			errs = append(errs, fmt.Sprintf("ByteOrder: invalid value %d (valid: 1–%d)", v, int(model.ByteOrderMax)-1))
		} else {
			byteOrder = v
		}
	}

	// 14. Logic - 32-bit DataType Length check
	// 32-bit Modbus values (e.g. Float32, Int32) MUST occupy exactly two 16-bit registers.
	if model.Is32BitDataType(model.RegisterDataType(dataType)) && length != 2 {
		errs = append(errs, fmt.Sprintf("DataType: %d is 32-bit, but Length is %d (must be 2)", dataType, length))
	}

	// 15. Unit - optional, length check
	unit := row["unit"]
	if unit != "" && utf8.RuneCountInString(unit) > maxStrLen {
		errs = append(errs, fmt.Sprintf("Unit: exceeds %d characters", maxStrLen))
	}

	// 16. EnumMapping - mandatory for EnumType, includes format and uniqueness check
	enumMap, emErrs := parseEnumMap(row["enummap"])
	errs = append(errs, emErrs...)
	if subDataType == int(model.RegSubTypeEnumType) {
		if len(enumMap) == 0 {
			errs = append(errs, "EnumMap: required for EnumType (2)")
		}
	} else if len(enumMap) > 0 {
		errs = append(errs, fmt.Sprintf("EnumMap: only for EnumType (2), but SubDataType is %d", subDataType))
	}

	// 17. BitMapping - mandatory for Bitfield, includes format and uniqueness check
	bitMap, bmErrs := parseBitMap(row["bitmap"])
	errs = append(errs, bmErrs...)
	if subDataType == int(model.RegSubTypeBitfield) {
		if len(bitMap) == 0 {
			errs = append(errs, "BitMap: required for Bitfield (3)")
		}
	} else if len(bitMap) > 0 {
		errs = append(errs, fmt.Sprintf("BitMap: only for Bitfield (3), but SubDataType is %d", subDataType))
	}

	// 18. Constraints - optional, only for ReadWrite & Numeric, includes range and logic check
	constraints, cErrs := parseConstraints(row["constraints"])
	errs = append(errs, cErrs...)
	if !constraints.IsZero() {
		if accessType != int(model.RegAccessTypeReadWrite) {
			errs = append(errs, "Constraints: only for ReadWrite (3)")
		}
		if !model.IsNumericDataType(model.RegisterDataType(dataType)) {
			errs = append(errs, fmt.Sprintf("Constraints: only for numeric DataTypes (1–7), but DataType is %d", dataType))
		}
		if constraints.Min >= constraints.Max {
			errs = append(errs, "Constraints: Min must be less than Max")
		}
		if constraints.Step <= 0 {
			errs = append(errs, "Constraints: Step must be positive")
		}
	}

	// 19. Execution - optional, only for non-ReadOnly registers
	execution, eErrs := parseExecution(row["execution"])
	errs = append(errs, eErrs...)
	if !execution.IsZero() {
		if accessType == int(model.RegAccessTypeRead) {
			errs = append(errs, "Execution: not applicable for ReadOnly registers")
		}
	}

	reg := model.Register{
		ID:           bson.NewObjectID(),
		Name:         name,
		Label:        label,
		Category:     model.RegisterCategory(category),
		RegisterType: model.RegisterType(regType),
		AddressStart: addrStart,
		Length:       length,
		AccessType:   model.RegisterAccessType(accessType),
		DataType:     model.RegisterDataType(dataType),
		SubDataType:  model.RegisterSubType(subDataType),
		Scale:        scale,
		Unit:         unit,
		WordOrder:    model.WordOrder(wordOrder),
		ByteOrder:    model.ByteOrder(byteOrder),
		EnumMap:      enumMap,
		BitMap:       bitMap,
		Constraints:  constraints,
		Execution:    execution,
	}

	return reg, errs
}

// Complex field parsers

// parseEnumMap parses "value=label;value=label;..." into []EnumMapping.
func parseEnumMap(raw string) ([]model.EnumMapping, []string) {
	const correctFormat = `correct format: "<int>=<label>;<int>=<label>;..." e.g. "0=Off;1=On;2=Standby"`
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	enumItemRegex := regexp.MustCompile(`^(\d+)=([^;=]+)$`)

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
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	re := regexp.MustCompile(`^(\d+)=([^|=:]+)(?::(info|warning|critical))?$`)
	var result []model.BitMapping
	var errs []string
	seen := make(map[int]bool)

	for _, item := range strings.Split(raw, "|") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		matches := re.FindStringSubmatch(item)
		if matches == nil {
			errs = append(errs, "BitMap: invalid item "+item)
			continue
		}

		bit, _ := strconv.Atoi(matches[1])
		label := strings.TrimSpace(matches[2])
		severityRaw := strings.ToLower(strings.TrimSpace(matches[3]))

		if seen[bit] {
			errs = append(errs, fmt.Sprintf("BitMap: duplicate bit position %d", bit))
			continue
		}
		seen[bit] = true

		m := model.BitMapping{Bit: bit, Label: label}
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
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.Constraints{}, nil
	}

	re := regexp.MustCompile(`^(?i)(min|max|step)=(-?\d+(?:\.\d+)?)$`)
	var c model.Constraints
	var errs []string
	seen := make(map[string]bool)

	for _, item := range strings.Split(raw, ";") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		matches := re.FindStringSubmatch(item)
		if matches == nil {
			errs = append(errs, "Constraints: invalid item "+item)
			continue
		}
		key := strings.ToLower(matches[1])
		val, _ := strconv.ParseFloat(matches[2], 64)

		if seen[key] {
			errs = append(errs, fmt.Sprintf("Constraints: duplicate key %q", key))
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
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.ExecutionConfig{}, nil
	}

	re := regexp.MustCompile(`^(-?\d+):(true|false)$`)
	matches := re.FindStringSubmatch(raw)
	if matches == nil {
		return model.ExecutionConfig{}, []string{"Execution: invalid format " + raw}
	}

	val, _ := strconv.Atoi(matches[1])
	autoReset := matches[2] == "true"

	return model.ExecutionConfig{
		TriggerValue: val,
		AutoReset:    autoReset,
	}, nil
}
