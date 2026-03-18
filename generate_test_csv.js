const fs = require('fs');

/**
 * Generate a test CSV file for the Modbus Register Parser.
 * Usage: node generate_test_csv.js <num_rows> [output_file]
 */

const numRows = process.argv[2] ? parseInt(process.argv[2]) : 2000;
const outputFile = process.argv[3] || 'test_registers.csv';

const header = "GroupName,GroupSeqNo,RegisterName,Label,Category,RegisterType,AddressStart,Length,AccessType,DataType,SubDataType,ByteOrder,WordOrder,Scale,Unit,EnumMap,BitMap,Constraints,Execution\n";

console.log(`Generating ${numRows} rows...`);

// Overwrite the file with the header to ensure a clean start
fs.writeFileSync(outputFile, header);

let content = "";

// Mapping groups to fixed sequence numbers to ensure consistency
const groupDefinitions = [
    { name: "MainStatus", seq: 1 },
    { name: "DetailedTelemetry", seq: 2 },
    { name: "FaultLog", seq: 3 },
    { name: "Settings", seq: 4 },
    { name: "Commands", seq: 5 }
];

for (let i = 1; i <= numRows; i++) {
    const group = groupDefinitions[(i - 1) % groupDefinitions.length];
    
    // Test case-insensitive GroupName consistency: mix uppercase and lowercase
    const groupName = (i % 2 === 0) ? group.name.toUpperCase() : group.name.toLowerCase();
    const groupSeqNo = group.seq;
    
    const registerName = `reg_id_${i}`;
    const label = `Test Register ${i}`;
    
    const category = (i % 3) + 1; // 1-3 (General, Identification, Fault)
    const registerType = (i % 4) + 1;
    const addressStart = 40000 + i;
    
    // New Rule Enforcement: 32-bit types (5,6,7) MUST have Length=2.
    // DataTypes: 1:UInt8, 2:Int8, 3:UInt16, 4:Int16, 5:UInt32, 6:Int32, 7:Float32, 8:ASCII
    const dataType = (i % 8) + 1;
    const is32Bit = (dataType === 5 || dataType === 6 || dataType === 7);
    const length = is32Bit ? 2 : 1; 
    
    const accessType = (i % 3) + 1;
    const subDataType = (i % 3) + 1;    // 1=Normal, 2=EnumType, 3=Bitfield
    
    // Rule: WordOrder only for Length=2.
    const byteOrder = (i % 2 === 0) ? 1 : 2;                         // 1=BigEndian, 2=LittleEndian
    const wordOrder = (length === 2) ? ((i % 2 === 0) ? 1 : 2) : ""; // 1=HighLow, 2=LowHigh — blank if not applicable
    
    const scale = 0.1;
    const unit = "kW";
    
    let enumMap = "";
    if (subDataType === 2) {
        enumMap = "0=Off;1=On;2=Standby";
    }
    
    let bitMap = "";
    if (subDataType === 3) {
        bitMap = "0=Err:critical|1=Warn:warning";
    }
    
    let constraints = "";
    // Rule: Constraints only for ReadWrite (3) and numeric types (1-7)
    if (accessType === 3 && dataType < 8) {
        constraints = "min=0;max=1000;step=1";
    }
    
    let execution = "";
    // Rule: Execution is only allowed for non-ReadOnly registers (2=Write, 3=ReadWrite)
    if (i % 5 === 0 && accessType !== 1) {
        execution = "1:true";
    }

    const row = [
        groupName,
        groupSeqNo,
        registerName,
        label,
        category,
        registerType,
        addressStart,
        length,
        accessType,
        dataType,
        subDataType,
        byteOrder,
        wordOrder,
        scale,
        unit,
        // Wrap fields that might contain commas in quotes
        enumMap.includes(';') ? `"${enumMap}"` : enumMap,
        bitMap.includes('|') ? `"${bitMap}"` : bitMap,
        constraints.includes(';') ? `"${constraints}"` : constraints,
        execution
    ].join(",");

    content += row + "\n";

    if (i % 10000 === 0) {
        fs.appendFileSync(outputFile, content);
        content = "";
    }
}

if (content) {
    fs.appendFileSync(outputFile, content);
}

console.log(`Successfully generated ${outputFile}`);
