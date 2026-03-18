const fs = require('fs');
const path = require('path');

const HEADER = "GroupName,GroupSeqNo,RegisterName,Label,Category,RegisterType,AddressStart,Length,AccessType,DataType,SubDataType,ByteOrder,WordOrder,Scale,Unit,EnumMap,BitMap,Constraints,Execution\n";

const OUTPUT_DIR = 'validation_tests';
if (!fs.existsSync(OUTPUT_DIR)) fs.mkdirSync(OUTPUT_DIR);

function createCSV(filename, rows) {
    const filePath = path.join(OUTPUT_DIR, filename);
    const content = HEADER + rows.map(r => r.join(',')).join('\n');
    fs.writeFileSync(filePath, content);
    console.log(`Created: ${filePath}`);
}

// 1. Valid Baseline
createCSV('01_valid_baseline.csv', [
    ["Main", 1, "reg1", "Valid Reg", 1, 1, 40001, 1, 1, 3, 1, 1, "", 1.0, "V", "", "", "", ""],
    ["Main", 1, "reg32", "Valid 32bit", 1, 1, 40002, 2, 1, 5, 1, 1, 1, 1.0, "V", "", "", "", ""]
]);

// 2. Fail: 32-bit Logic (DataType 5/6/7 must have Length 2)
createCSV('02_fail_32bit_length.csv', [
    ["Main", 1, "bad_32bit", "Should be length 2", 1, 1, 40001, 1, 1, 5, 1, 1, "", 1.0, "V", "", "", "", ""]
]);

// 3. Fail: Case-Insensitive Group Inconsistency (Same name different case, different SeqNo)
createCSV('03_fail_group_inconsistency.csv', [
    ["Inverter", 1, "reg1", "Reg 1", 1, 1, 40001, 1, 1, 3, 1, 1, "", 1.0, "V", "", "", "", ""],
    ["inverter", 2, "reg2", "Reg 2", 1, 1, 40002, 1, 1, 3, 1, 1, "", 1.0, "V", "", "", "", ""]
]);

// 4. Fail: Name Uniqueness
createCSV('04_fail_duplicate_name.csv', [
    ["Main", 1, "duplicate", "Reg A", 1, 1, 40001, 1, 1, 3, 1, 1, "", 1.0, "V", "", "", "", ""],
    ["Main", 1, "duplicate", "Reg B", 1, 1, 40002, 1, 1, 3, 1, 1, "", 1.0, "V", "", "", "", ""]
]);

// 5. Fail: WordOrder Restriction (Length 1 cannot have WordOrder)
createCSV('05_fail_wordorder_length1.csv', [
    ["Main", 1, "reg1", "Length 1 with WordOrder", 1, 1, 40001, 1, 1, 3, 1, 1, 1, 1.0, "V", "", "", "", ""]
]);

// 6. Fail: Constraints Restriction (Only for ReadWrite=3)
createCSV('06_fail_constraints_readonly.csv', [
    ["Main", 1, "reg1", "ReadOnly with Constraints", 1, 1, 40001, 1, 1, 3, 1, 1, "", 1.0, "V", "", "", "min=0;max=10", ""]
]);

// 7. Fail: Execution Restriction (Only for Category Command=4)
createCSV('07_fail_execution_general.csv', [
    ["Main", 1, "reg1", "General with Execution", 1, 1, 40001, 1, 1, 3, 1, 1, "", 1.0, "V", "", "", "", "1:true"]
]);

// 8. Fail: SubDataType Enum Map Missing
createCSV('08_fail_enum_map_missing.csv', [
    ["Main", 1, "reg1", "Enum SubType missing Map", 1, 1, 40001, 1, 1, 3, 2, 1, "", 1.0, "V", "", "", "", ""]
]);

// 9. Fail: SubDataType Bit Map Missing
createCSV('09_fail_bit_map_missing.csv', [
    ["Main", 1, "reg1", "Bitfield SubType missing Map", 1, 1, 40001, 1, 1, 3, 3, 1, "", 1.0, "V", "", "", "", ""]
]);

// 10. Fail: DataType Max Enum Check
createCSV('10_fail_invalid_datatype.csv', [
    ["Main", 1, "reg1", "Invalid DataType 99", 1, 1, 40001, 1, 1, 99, 1, 1, "", 1.0, "V", "", "", "", ""]
]);

console.log("\nAll validation test files generated in './validation_tests' directory.");
console.log("You can test them by uploading each to the server.");
