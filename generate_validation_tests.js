const fs = require('fs');
const path = require('path');

// CSV column order must match the server header exactly.
const HEADER = "GroupName,GroupSeqNo,RegisterName,Label,Category,RegisterType,AddressStart,Length,AccessType,DataType,SubDataType,ByteOrder,WordOrder,Scale,Unit,EnumMap,BitMap,Constraints,Execution\n";

// ── Enum value reference (matches model/register.go) ─────────────────────────
// Category:     1=General, 2=Identification, 3=Fault            (max sentinel: 4)
// RegisterType: 1=Holding, 2=Input, 3=Coil, 4=Discrete          (max sentinel: 5)
// AccessType:   1=Read, 2=Write, 3=ReadWrite                     (max sentinel: 4)
// DataType:     1=UInt8, 2=Int8, 3=UInt16, 4=Int16, 5=UInt32,
//               6=Int32, 7=Float32, 8=ASCII                      (max sentinel: 9)
// SubDataType:  1=Normal, 2=EnumType, 3=Bitfield                 (max sentinel: 4)
// ByteOrder:    1=BigEndian, 2=LittleEndian                      (max sentinel: 3)
// WordOrder:    1=HighLow, 2=LowHigh  (0 = not set)             (max sentinel: 3)
// BitSeverity:  info, warning, critical
// ─────────────────────────────────────────────────────────────────────────────

const OUTPUT_DIR = 'validation_tests';
if (!fs.existsSync(OUTPUT_DIR)) fs.mkdirSync(OUTPUT_DIR);

function row(group, seqno, name, label, cat, regtype, addr, len, access, dtype, subdtype, byteord, wordord, scale, unit, enumMap, bitMap, constraints, execution) {
    return [group, seqno, name, label, cat, regtype, addr, len, access, dtype, subdtype, byteord, wordord, scale, unit, enumMap, bitMap, constraints, execution];
}

function createCSV(filename, rows) {
    const filePath = path.join(OUTPUT_DIR, filename);
    // Wrap cells containing commas or semicolons in quotes so the CSV is valid.
    const content = HEADER + rows.map(r =>
        r.map(cell => String(cell).includes(',') ? `"${cell}"` : cell).join(',')
    ).join('\n');
    fs.writeFileSync(filePath, content);
    console.log(`Created: ${filePath}`);
}

// ── SECTION 1: Valid baselines ───────────────────────────────────────────────

// 01 — valid 16-bit and 32-bit registers in one group
createCSV('01_valid_baseline.csv', [
    row("Main", 1, "voltage", "Voltage",          1, 1, 40001, 1, 1, 3, 1, 1, 0,  1.0, "V",  "",                "", "",             ""),
    row("Main", 1, "power",   "Power 32bit",      1, 1, 40002, 2, 1, 5, 1, 1, 1,  1.0, "W",  "",                "", "",             ""),
    row("Main", 1, "status",  "Status Enum",      1, 1, 40004, 1, 1, 3, 2, 1, 0,  1.0, "",   "0=Off;1=On;2=Err","", "",             ""),
    row("Main", 1, "fault",   "Fault Bitfield",   1, 1, 40005, 1, 1, 3, 3, 1, 0,  1.0, "",   "",                "0=OVP:critical|1=OTP:warning|2=OK:info", "", ""),
    row("Main", 1, "setpt",   "Setpoint RW",      1, 1, 40006, 1, 3, 3, 1, 1, 0,  1.0, "V",  "",                "", "min=0;max=100;step=1", ""),
    row("Cmd",  2, "reset",   "Reset Command",    1, 1, 40007, 1, 2, 3, 1, 1, 0,  1.0, "",   "",                "", "",             "1:true"),
]);

// ── SECTION 2: Mandatory field validation ────────────────────────────────────

// 02 — missing RegisterName and Label
createCSV('02_fail_missing_name_label.csv', [
    row("Main", 1, "", "", 1, 1, 40001, 1, 1, 3, 1, 1, 0, 1.0, "V", "", "", "", ""),
]);

// 03 — SeqNo = 0 (must be positive)
createCSV('03_fail_seqno_zero.csv', [
    row("Main", 0, "reg1", "Reg 1", 1, 1, 40001, 1, 1, 3, 1, 1, 0, 1.0, "V", "", "", "", ""),
]);

// 04 — AddressStart out of range (>65535)
createCSV('04_fail_address_out_of_range.csv', [
    row("Main", 1, "reg1", "Reg 1", 1, 1, 99999, 1, 1, 3, 1, 1, 0, 1.0, "V", "", "", "", ""),
]);

// 05 — Length = 0 (must be positive)
createCSV('05_fail_length_zero.csv', [
    row("Main", 1, "reg1", "Reg 1", 1, 1, 40001, 0, 1, 3, 1, 1, 0, 1.0, "V", "", "", "", ""),
]);

// ── SECTION 3: Enum validation ───────────────────────────────────────────────

// 06 — DataType out of valid range (99 is invalid, valid 1–8)
createCSV('06_fail_invalid_datatype.csv', [
    row("Main", 1, "reg1", "Invalid DataType 99", 1, 1, 40001, 1, 1, 99, 1, 1, 0, 1.0, "V", "", "", "", ""),
]);

// 07 — Category sentinel value (3 is Fault=valid, 4 is the sentinel=invalid)
createCSV('07_fail_category_sentinel.csv', [
    row("Main", 1, "reg1", "Category = max sentinel", 4, 1, 40001, 1, 1, 3, 1, 1, 0, 1.0, "V", "", "", "", ""),
]);

// 08 — AccessType 0 (mandatory, 0 means not set → invalid)
createCSV('08_fail_access_type_zero.csv', [
    row("Main", 1, "reg1", "AccessType = 0", 1, 1, 40001, 1, 0, 3, 1, 1, 0, 1.0, "V", "", "", "", ""),
]);

// ── SECTION 4: 32-bit DataType + Length check ────────────────────────────────

// 09 — 32-bit DataType (UInt32=5) must have Length=2 but here it has Length=1
createCSV('09_fail_32bit_length_1.csv', [
    row("Main", 1, "reg1", "UInt32 with Length=1", 1, 1, 40001, 1, 1, 5, 1, 1, 0, 1.0, "W", "", "", "", ""),
]);

// ── SECTION 5: WordOrder restriction ─────────────────────────────────────────

// 10 — WordOrder can only be set when Length=2; here Length=1
createCSV('10_fail_wordorder_length1.csv', [
    row("Main", 1, "reg1", "Length 1 with WordOrder", 1, 1, 40001, 1, 1, 3, 1, 1, 1, 1.0, "V", "", "", "", ""),
]);

// ── SECTION 6: Name uniqueness ───────────────────────────────────────────────

// 11 — duplicate RegisterName within same upload
createCSV('11_fail_duplicate_name.csv', [
    row("Main", 1, "duplicate", "Reg A", 1, 1, 40001, 1, 1, 3, 1, 1, 0, 1.0, "V", "", "", "", ""),
    row("Main", 1, "duplicate", "Reg B", 1, 1, 40002, 1, 1, 3, 1, 1, 0, 1.0, "V", "", "", "", ""),
]);

// ── SECTION 7: Group consistency ─────────────────────────────────────────────

// 12 — same group name (case-insensitive) but different SeqNo
createCSV('12_fail_group_inconsistency.csv', [
    row("Inverter", 1, "reg1", "Reg 1", 1, 1, 40001, 1, 1, 3, 1, 1, 0, 1.0, "V", "", "", "", ""),
    row("inverter", 2, "reg2", "Reg 2", 1, 1, 40002, 1, 1, 3, 1, 1, 0, 1.0, "V", "", "", "", ""),
]);

// ── SECTION 8: EnumMap format validation (NEW) ───────────────────────────────

// 13 — SubDataType=EnumType(2) but EnumMap is empty → required error
createCSV('13_fail_enum_map_missing.csv', [
    row("Main", 1, "reg1", "Enum SubType missing Map", 1, 1, 40001, 1, 1, 3, 2, 1, 0, 1.0, "V", "", "", "", ""),
]);

// 14 — EnumMap item missing '=' separator → format error
createCSV('14_fail_enum_map_no_separator.csv', [
    row("Main", 1, "reg1", "Bad EnumMap", 1, 1, 40001, 1, 1, 3, 2, 1, 0, 1.0, "V", "0 Off;1=On", "", "", ""),
]);

// 15 — EnumMap value is not an integer → format error
createCSV('15_fail_enum_map_nonint_value.csv', [
    row("Main", 1, "reg1", "Bad EnumMap value", 1, 1, 40001, 1, 1, 3, 2, 1, 0, 1.0, "V", "X=Off;1=On", "", "", ""),
]);

// 16 — EnumMap label is empty → format error
createCSV('16_fail_enum_map_empty_label.csv', [
    row("Main", 1, "reg1", "Empty label in EnumMap", 1, 1, 40001, 1, 1, 3, 2, 1, 0, 1.0, "V", "0=;1=On", "", "", ""),
]);

// 17 — EnumMap provided but SubDataType is Normal(1) → not allowed
createCSV('17_fail_enum_map_wrong_subtype.csv', [
    row("Main", 1, "reg1", "EnumMap on Normal SubType", 1, 1, 40001, 1, 1, 3, 1, 1, 0, 1.0, "V", "0=Off;1=On", "", "", ""),
]);

// ── SECTION 9: BitMap format validation (NEW) ────────────────────────────────

// 18 — SubDataType=Bitfield(3) but BitMap is empty → required error
createCSV('18_fail_bit_map_missing.csv', [
    row("Main", 1, "reg1", "Bitfield missing BitMap", 1, 1, 40001, 1, 1, 3, 3, 1, 0, 1.0, "V", "", "", "", ""),
]);

// 19 — BitMap item missing '=' separator → format error
createCSV('19_fail_bit_map_no_separator.csv', [
    row("Main", 1, "reg1", "Bad BitMap no sep", 1, 1, 40001, 1, 1, 3, 3, 1, 0, 1.0, "V", "", "0 Alarm:critical|1=OK:info", "", ""),
]);

// 20 — BitMap bit position is not an integer → format error
createCSV('20_fail_bit_map_nonint_bit.csv', [
    row("Main", 1, "reg1", "Bad BitMap bit", 1, 1, 40001, 1, 1, 3, 3, 1, 0, 1.0, "V", "", "X=Alarm:critical|1=OK:info", "", ""),
]);

// 21 — BitMap label is empty → format error
createCSV('21_fail_bit_map_empty_label.csv', [
    row("Main", 1, "reg1", "Empty label in BitMap", 1, 1, 40001, 1, 1, 3, 3, 1, 0, 1.0, "V", "", "0=:critical|1=OK:info", "", ""),
]);

// 22 — BitMap severity is an unknown string → format error
createCSV('22_fail_bit_map_bad_severity.csv', [
    row("Main", 1, "reg1", "Bad severity in BitMap", 1, 1, 40001, 1, 1, 3, 3, 1, 0, 1.0, "V", "", "0=Alarm:high|1=OK:info", "", ""),
]);

// 23 — BitMap provided but SubDataType is Normal(1) → not allowed
createCSV('23_fail_bit_map_wrong_subtype.csv', [
    row("Main", 1, "reg1", "BitMap on Normal SubType", 1, 1, 40001, 1, 1, 3, 1, 1, 0, 1.0, "V", "", "0=Alarm:critical", "", ""),
]);

// ── SECTION 10: Constraints format validation (NEW) ──────────────────────────

// 24 — valid Constraints on ReadWrite numeric register
createCSV('24_valid_constraints.csv', [
    row("Main", 1, "setpt", "Setpoint", 1, 1, 40001, 1, 3, 3, 1, 1, 0, 1.0, "V", "", "", "min=0;max=100;step=1", ""),
]);

// 25 — Constraints on ReadOnly register → not allowed
createCSV('25_fail_constraints_readonly.csv', [
    row("Main", 1, "reg1", "ReadOnly with Constraints", 1, 1, 40001, 1, 1, 3, 1, 1, 0, 1.0, "V", "", "", "min=0;max=10;step=1", ""),
]);

// 26 — Constraints on non-numeric DataType (ASCII=8) → not allowed
createCSV('26_fail_constraints_non_numeric.csv', [
    row("Main", 1, "reg1", "ASCII with Constraints", 1, 1, 40001, 1, 3, 8, 1, 1, 0, 1.0, "V", "", "", "min=0;max=10;step=1", ""),
]);

// 27 — Constraints Min >= Max → invalid
createCSV('27_fail_constraints_min_gte_max.csv', [
    row("Main", 1, "reg1", "Bad min/max", 1, 1, 40001, 1, 3, 3, 1, 1, 0, 1.0, "V", "", "", "min=100;max=0;step=1", ""),
]);

// 28 — Constraints Step <= 0 → invalid
createCSV('28_fail_constraints_step_zero.csv', [
    row("Main", 1, "reg1", "Zero step", 1, 1, 40001, 1, 3, 3, 1, 1, 0, 1.0, "V", "", "", "min=0;max=100;step=0", ""),
]);

// 29 — Constraints item missing '=' → format error
createCSV('29_fail_constraints_no_separator.csv', [
    row("Main", 1, "reg1", "Bad Constraints format", 1, 1, 40001, 1, 3, 3, 1, 1, 0, 1.0, "V", "", "", "min 0;max=100;step=1", ""),
]);

// 30 — Constraints value is not a number → format error
createCSV('30_fail_constraints_nonnum_value.csv', [
    row("Main", 1, "reg1", "Non-number Constraints", 1, 1, 40001, 1, 3, 3, 1, 1, 0, 1.0, "V", "", "", "min=abc;max=100;step=1", ""),
]);

// 31 — Constraints unknown key → format error
createCSV('31_fail_constraints_unknown_key.csv', [
    row("Main", 1, "reg1", "Unknown Constraints key", 1, 1, 40001, 1, 3, 3, 1, 1, 0, 1.0, "V", "", "", "minimum=0;max=100;step=1", ""),
]);

// ── SECTION 11: Execution format validation (NEW) ────────────────────────────

// 32 — valid Execution on a Write register
createCSV('32_valid_execution.csv', [
    row("Cmd", 1, "reset", "Reset", 1, 1, 40001, 1, 2, 3, 1, 1, 0, 1.0, "", "", "", "", "1:true"),
]);

// 33 — Execution on ReadOnly register → not allowed
createCSV('33_fail_execution_readonly.csv', [
    row("Main", 1, "reg1", "ReadOnly with Execution", 1, 1, 40001, 1, 1, 3, 1, 1, 0, 1.0, "V", "", "", "", "1:true"),
]);

// 34 — Execution missing ':' separator → format error
createCSV('34_fail_execution_no_separator.csv', [
    row("Cmd", 1, "reset", "Bad Execution", 1, 1, 40001, 1, 2, 3, 1, 1, 0, 1.0, "", "", "", "", "1true"),
]);

// 35 — Execution trigger value is not an integer → format error
createCSV('35_fail_execution_nonint_trigger.csv', [
    row("Cmd", 1, "reset", "Bad trigger", 1, 1, 40001, 1, 2, 3, 1, 1, 0, 1.0, "", "", "", "", "X:true"),
]);

// 36 — Execution autoReset is not true/false → format error
createCSV('36_fail_execution_bad_autoreset.csv', [
    row("Cmd", 1, "reset", "Bad autoReset", 1, 1, 40001, 1, 2, 3, 1, 1, 0, 1.0, "", "", "", "", "1:yes"),
]);

// ── SECTION 12: Scale validation ─────────────────────────────────────────────

// 37 — Scale is negative → invalid
createCSV('37_fail_scale_negative.csv', [
    row("Main", 1, "reg1", "Negative Scale", 1, 1, 40001, 1, 1, 3, 1, 1, 0, -1.0, "V", "", "", "", ""),
]);

// ── Summary ───────────────────────────────────────────────────────────────────
console.log("\nAll validation test CSVs generated in './" + OUTPUT_DIR + "':\n");
console.log("  VALID BASELINES  : 01, 24, 32");
console.log("  MANDATORY FIELDS : 02–05");
console.log("  ENUM VALIDATION  : 06–08");
console.log("  32-BIT / LENGTH  : 09");
console.log("  WORDORDER        : 10");
console.log("  NAME UNIQUENESS  : 11");
console.log("  GROUP CONSIST.   : 12");
console.log("  ENUMMAP FORMAT   : 13–17");
console.log("  BITMAP FORMAT    : 18–23");
console.log("  CONSTRAINTS FMT  : 25–31");
console.log("  EXECUTION FORMAT : 33–36");
console.log("  SCALE            : 37");
console.log("\nUpload each file to the server to verify the expected error is returned.\n");
