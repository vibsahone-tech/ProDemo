package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ── Document structs ────────────────────────────────────────────────────────

// Register is the primary Modbus register definition stored in MongoDB.
type Register struct {
	ID           bson.ObjectID      `bson:"_id,omitempty"`         // The document ID.
	Name         string             `bson:"name"`                  // Unique key for the register.
	Label        string             `bson:"label"`                 // Human-readable display name.
	SeqNo        int                `bson:"seq_no"`                // Sequence number used for ordering within register list.
	Category     RegisterCategory   `bson:"category"`              // Functional classification.
	RegisterType RegisterType       `bson:"register_type"`         // Modbus register type.
	AddressStart int                `bson:"address_start"`         // Starting Modbus address of the register.
	Length       int                `bson:"length"`                // Number of 16-bit Modbus registers occupied. Length = 2 required for 32-bit data types.
	AccessType   RegisterAccessType `bson:"access_type"`           // Access permission.
	DataType     RegisterDataType   `bson:"data_type"`             // Data interpretation type.
	SubDataType  RegisterSubType    `bson:"sub_data_type"`         // Data interpretation sub type.
	ByteOrder    ByteOrder          `bson:"byte_order,omitempty"`  // Byte order.
	WordOrder    WordOrder          `bson:"word_order,omitempty"`  // Word order for 32-bit values ONLY (Length = 2). Applicable when Length = 2.
	Scale        float64            `bson:"scale,omitempty"`       // Multiplier applied to raw register value before exposing to application.
	Unit         string             `bson:"unit,omitempty"`        // Measurement unit.
	EnumMap      []EnumMapping      `bson:"enum_map,omitempty"`    // Enum mapping definitions. Required only when DataType = Enum.
	BitMap       []BitMapping       `bson:"bit_map,omitempty"`     // Bit-level mapping definitions. Required only when DataType = Bitfield.
	Constraints  Constraints        `bson:"constraints,omitempty"` // Validation constraints. // Applicable ONLY for ReadWrite numeric setting registers.
	Execution    ExecutionConfig    `bson:"execution,omitempty"`   // Command execution configuration.
}

// RegisterGroup represents the register groups for the documents.
type RegisterGroup struct {
	Name        string          `bson:"name"`                   // The name of register group.
	SeqNo       int             `bson:"seq_no"`                 // The Sequence number.
	RegisterIds []bson.ObjectID `bson:"register_ids,omitempty"` // The associated register._id values.
}

// ── Supporting structs ──────────────────────────────────────────────────────

// EnumMapping maps a raw register integer value to a human-readable label.
type EnumMapping struct {
	Value int    `bson:"value"` // Raw register value.
	Label string `bson:"label"` // Human-readable meaning of the value.
}

// BitMapping maps a single bit position to a label and severity.
type BitMapping struct {
	Bit      int         `bson:"bit"`      // Bit position (0-based index within register).
	Label    string      `bson:"label"`    // Human-readable label describing the bit meaning.
	Severity BitSeverity `bson:"severity"` // Severity classification.
}

// Constraints holds validation bounds for ReadWrite numeric registers.
type Constraints struct {
	Min  float64 `bson:"min,omitempty"`  // Minimum allowed value.
	Max  float64 `bson:"max,omitempty"`  // Maximum allowed value.
	Step float64 `bson:"step,omitempty"` // Step increment value for valid inputs.
}

// ExecutionConfig holds command-execution settings for Command-category registers.
type ExecutionConfig struct {
	TriggerValue int  `bson:"trigger_value"` // Value written to register to trigger execution.
	AutoReset    bool `bson:"auto_reset"`    // Whether the register auto-resets after execution.
}

// ── Enum types & constants ───────────────────────────────────────────────────

// RegisterAccessType defines the allowed operation on a register (read, write, or both).
type RegisterAccessType int

// RegisterDataType defines how the raw register value should be interpreted.
type RegisterDataType int

// BitSeverity defines the severity level for bits in a bitfield register.
type BitSeverity int

// RegisterCategory groups registers logically (e.g., general info, identification, faults).
type RegisterCategory int

// RegisterSubType defines special interpretation of register values like enum or bitfield.
type RegisterSubType int

// RegisterType defines the Modbus register type.
type RegisterType int

// WordOrder defines the order of 16-bit words for multi-register values.
type WordOrder int

// ByteOrder defines the byte order used inside a register.
type ByteOrder int

// RegisterCategory constants.
const (
	RegCategoryGeneral        RegisterCategory = iota + 1 // 1. General
	RegCategoryIdentification                             // 2. Identification
	RegCategoryFault                                      // 3. Fault
	RegCategoryMax                                        // Sentinel value
)

// RegisterType constants.
const (
	RegisterTypeHolding  RegisterType = iota + 1 // 1. Holding
	RegisterTypeInput                            // 2. Input
	RegisterTypeCoil                             // 3. Coil
	RegisterTypeDiscrete                         // 4. Discrete
	RegisterTypeMax                              // Sentinel value
)

// RegisterAccessType constants.
const (
	RegAccessTypeRead      RegisterAccessType = iota + 1 // 1- Read
	RegAccessTypeWrite                                   // 2- Write
	RegAccessTypeReadWrite                               // 3- ReadWrite
	RegAccessTypeMax                                     // Sentinel value
)

// RegisterDataType constants.
const (
	RegDataTypeUInt8   RegisterDataType = iota + 1 // 1. UInt8
	RegDataTypeInt8                                // 2. Int8
	RegDataTypeUInt16                              // 3. UInt16
	RegDataTypeInt16                               // 4. Int16
	RegDataTypeUInt32                              // 5. UInt32
	RegDataTypeInt32                               // 6. Int32
	RegDataTypeFloat32                             // 7. Float32
	RegDataTypeASCII                               // 8. ASCII
	RegDataTypeMax                                 // Sentinel value
)

// RegisterSubType constants.
const (
	RegSubTypeNormal   RegisterSubType = iota + 1 // 1. Normal
	RegSubTypeEnumType                            // 2. EnumType
	RegSubTypeBitfield                            // 3. Bitfield
	RegSubTypeMax                                 // Sentinel value
)

// BitSeverity constants.
const (
	BitSeverityInfo     BitSeverity = iota + 1 // 1. Info
	BitSeverityWarning                         // 2. Warning
	BitSeverityCritical                        // 3. Critical
	BitSeverityMax                             // Sentinel value
)

// WordOrder constants.
const (
	WordOrderHighLow WordOrder = iota + 1 // 1. HighLow
	WordOrderLowHigh                      // 2. LowHigh
	WordOrderMax                          // Sentinel value
)

// ByteOrder constants.
const (
	ByteOrderBigEndian    ByteOrder = iota + 1 // 1. BigEndian
	ByteOrderLittleEndian                      // 2. LittleEndian
	ByteOrderMax                               // Sentinel value
)

// ── Helper predicates ───────────────────────────────────────────────────────

func (c Constraints) IsZero() bool {
	return c.Min == 0 && c.Max == 0 && c.Step == 0
}

func (e ExecutionConfig) IsZero() bool {
	return e.TriggerValue == 0 && !e.AutoReset
}

// IsNumericDataType returns true for all numeric (non-ASCII) data types.
// Used to decide whether Constraints are applicable.
func IsNumericDataType(dt RegisterDataType) bool {
	return dt >= RegDataTypeUInt8 && dt < RegDataTypeASCII
}

// Is32BitDataType returns true for all 32-bit data types.
// Used to enforce Length=2 requirement.
func Is32BitDataType(dt RegisterDataType) bool {
	return dt == RegDataTypeUInt32 || dt == RegDataTypeInt32 || dt == RegDataTypeFloat32
}

// ── Protocol & Communication structs ────────────────────────────────────────

type Parity int

// Parity defines the parity mode used in RS485 serial communication configuration.
const (
	ParityEven Parity = iota + 1 // 1- Even
	ParityOdd                    // 2- Odd
	ParityNo                     // 3- No
	ParityMax                    // Sentinel value
)

// Represents the audit info for documents.
type AuditInfo struct {
	CreatedBy bson.ObjectID `bson:"created_by"` // user._id that created the document.
	CreatedAt time.Time     `bson:"created_at"` // TS of document creation.
	UpdatedBy bson.ObjectID `bson:"updated_by"` // user._id that updated the document.
	UpdatedAt time.Time     `bson:"updated_at"` // TS of document updation.
}

// Represents Protocol. Collection: protocol
type Protocol struct {
	ID                        bson.ObjectID   `bson:"_id,omitempty"`                // The document ID.
	Name                      string          `bson:"name"`                         // The name of protocol.
	SlaveAddress              int             `bson:"slave_address"`                // Device slave address.
	Communication             Communication   `bson:"communication"`                // Serial communication configuration.
	RegisterGroups            []RegisterGroup `bson:"register_groups"`              // Register groups.
	ReadRegisterCode          string          `bson:"read_register_code"`           // Function code for reading registers.
	WriteRegisterCode         string          `bson:"write_register_code"`          // Function code for writing single register.
	WriteMultipleRegisterCode string          `bson:"write_multiple_register_code"` // Function code for writing multiple registers
	CommErrResponseCode       string          `bson:"comm_err_response_code"`       // Error response code.
	ErrorCodes                []ErrorCode     `bson:"error_codes"`                  // List of error codes.
	AuditInfo                 AuditInfo       `bson:"audit_info"`                   // Audit info.
}

// Represents the communication for the documents.
type Communication struct {
	BaudRate int     `bson:"baud_rate"` // Communication speed.
	StopBits float64 `bson:"stop_bits"` // Number of stop bits (1, 1.5, or 2).
	DataBits int    `bson:"data_bits"` // Number of data bits.
	Parity   Parity `bson:"parity"`    // Parity mode.
}

// Represents the error code for the documents.
type ErrorCode struct {
	Code  int    `bson:"code"`  // Exception Code.
	Label string `bson:"label"` // Human-readable description of the error.
}
