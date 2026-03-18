package parser

import (
	"fmt"
	"unicode/utf8"

	"csv-upload-parser/model"
)

const maxStrLen = 256

// ── Main validation logic ───────────────────────────────────────────────────

// validateRegister checks all field-level rules for a parsed register row.
// rowNum is 1-based (data row, not including header).
// seenNames tracks register names already encountered to enforce uniqueness.
// Returns a slice of human-readable error messages; empty means the row is valid.
func validateRegister(_ int, reg model.Register, seenNames map[string]bool) []string {
	var errs []string

	// ── 1. Name ───────────────────
	if reg.Name == "" {
		errs = append(errs, "Name: mandatory field is missing")
	} else {
		if utf8.RuneCountInString(reg.Name) > maxStrLen {
			errs = append(errs, fmt.Sprintf("Name: exceeds %d characters", maxStrLen))
		}
		if seenNames[reg.Name] {
			errs = append(errs, fmt.Sprintf("Name: %q is not unique", reg.Name))
		}
		seenNames[reg.Name] = true
	}

	// ── 2. Label ───────────────────
	if reg.Label == "" {
		errs = append(errs, "Label: mandatory field is missing")
	} else if utf8.RuneCountInString(reg.Label) > maxStrLen {
		errs = append(errs, fmt.Sprintf("Label: exceeds %d characters", maxStrLen))
	}

	// ── 3. SeqNo / 6. AddressStart / 7. Length ───────────────────
	if reg.SeqNo <= 0 {
		errs = append(errs, "SeqNo: must be positive")
	}
	if reg.AddressStart < 0 || reg.AddressStart > 65535 {
		errs = append(errs, "AddressStart: must be between 0 and 65535")
	}
	if reg.Length <= 0 {
		errs = append(errs, "Length: must be positive")
	}

	// ── Mandatory Enums ───────────────────
	validateEnum("Category", int(reg.Category), int(model.RegCategoryMax), true, &errs)
	validateEnum("RegisterType", int(reg.RegisterType), int(model.RegisterTypeMax), true, &errs)
	validateEnum("AccessType", int(reg.AccessType), int(model.RegAccessTypeMax), true, &errs)
	validateEnum("DataType", int(reg.DataType), int(model.RegDataTypeMax), true, &errs)
	validateEnum("SubDataType", int(reg.SubDataType), int(model.RegSubTypeMax), true, &errs)

	// ── Optional Enums ───────────────────
	validateEnum("ByteOrder", int(reg.ByteOrder), int(model.ByteOrderMax), false, &errs)
	if validateEnum("WordOrder", int(reg.WordOrder), int(model.WordOrderMax), false, &errs) {
		if reg.WordOrder != 0 && reg.Length != 2 {
			errs = append(errs, fmt.Sprintf("WordOrder: only for Length=2, but Length is %d", reg.Length))
		}
	}

	// ── 32-bit DataType Length check ───────────────────
	if model.Is32BitDataType(reg.DataType) && reg.Length != 2 {
		errs = append(errs, fmt.Sprintf("DataType: %d is 32-bit, but Length is %d (must be 2)", reg.DataType, reg.Length))
	}

	// ── Scale ───────────────────
	if reg.Scale < 0 {
		errs = append(errs, "Scale: must be non-negative")
	}

	// ── 14. Unit ───────────────────
	if reg.Unit != "" && utf8.RuneCountInString(reg.Unit) > maxStrLen {
		errs = append(errs, fmt.Sprintf("Unit: exceeds %d characters", maxStrLen))
	}

	// ── 15. EnumMap (only for EnumType=2) ───────────────────
	if reg.SubDataType == model.RegSubTypeEnumType {
		if len(reg.EnumMap) == 0 {
			errs = append(errs, "EnumMap: required for EnumType (2)")
		}
	} else if len(reg.EnumMap) > 0 {
		errs = append(errs, fmt.Sprintf("EnumMap: only for EnumType (2), but SubDataType is %d", reg.SubDataType))
	}

	// ── 16. BitMap (only for Bitfield=3) ───────────────────
	if reg.SubDataType == model.RegSubTypeBitfield {
		if len(reg.BitMap) == 0 {
			errs = append(errs, "BitMap: required for Bitfield (3)")
		}
		for i, bm := range reg.BitMap {
			validateEnum(fmt.Sprintf("BitMap[%d].Severity", i), int(bm.Severity), int(model.BitSeverityMax), false, &errs)
		}
	} else if len(reg.BitMap) > 0 {
		errs = append(errs, fmt.Sprintf("BitMap: only for Bitfield (3), but SubDataType is %d", reg.SubDataType))
	}

	// ── 17. Constraints ───────────────────
	if !reg.Constraints.IsZero() {
		if reg.AccessType != model.RegAccessTypeReadWrite {
			errs = append(errs, "Constraints: only for ReadWrite (3)")
		}
		if !model.IsNumericDataType(reg.DataType) {
			errs = append(errs, fmt.Sprintf("Constraints: only for numeric DataTypes (1–7), but DataType is %d", reg.DataType))
		}
		if reg.Constraints.Min >= reg.Constraints.Max {
			errs = append(errs, "Constraints: Min must be less than Max")
		}
		if reg.Constraints.Step <= 0 {
			errs = append(errs, "Constraints: Step must be positive")
		}
	}

	// ── 18. Execution ───────────────────
	if !reg.Execution.IsZero() {
		if reg.AccessType == model.RegAccessTypeRead {
			errs = append(errs, "Execution: not applicable for ReadOnly registers")
		}
	}

	return errs
}

// ── Helper functions ────────────────────────────────────────────────────────

// validateEnum checks if a value is within [1, max-1].
// If mandatory=false, it allows 0 (not set).
// Returns true if the value is valid.
func validateEnum(name string, val int, max int, mandatory bool, errs *[]string) bool {
	if !mandatory && val == 0 {
		return true
	}
	if val <= 0 || val >= max {
		*errs = append(*errs, fmt.Sprintf("%s: invalid value %d (valid: 1–%d)", name, val, max-1))
		return false
	}
	return true
}
