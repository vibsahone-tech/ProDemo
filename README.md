# ProDemo: CSV Upload & Modbus Parser

ProDemo is a Go-based microservice designed to handle the ingestion, validation, and storage of Modbus register configurations via CSV uploads. It ensures data integrity through strict validation and atomic MongoDB transactions.

## Architecture

- **`main.go`**: Entry point that loads configuration and starts the HTTP server.
- **`server/`**: Contains the HTTP server logic, route handlers, and MongoDB transaction management.
- **`parser/`**: Core logic for parsing multipart form-data (Protocol) and CSV content (Registers).
- **`model/`**: Shared data structures for Registers, Protocols, and related enums.
- **`config/`**: JSON configuration management.

## Getting Started

### Prerequisites

- [Go](https://go.dev/dl/) 1.22 or later.
- [MongoDB](https://www.mongodb.com/try/download/community) instance running locally (default: `mongodb://localhost:27017`).

### Running the Server

1. Install dependencies:
   ```bash
   go mod tidy
   ```
2. Start the server:
   ```bash
   go run main.go
   ```
   The server defaults to port `8080`.

## API Documentation

### POST `/upload-csv`

Uploads a protocol definition (via form-data) and a register list (via CSV file).

#### Form-Data Fields

| Field | Type | Description |
| :--- | :--- | :--- |
| `name` | String | Protocol name (mandatory) |
| `slave_address` | Int | Device address (1-247) |
| `baud_rate` | Int | 1200, 2400, ..., 115200 |
| `stop_bits` | Float | 1, 1.5, 2 |
| `data_bits` | Int | 5-8 |
| `parity` | Int | 1 (Even), 2 (Odd), 3 (No) |
| `read_register_code` | Hex | e.g., `0x03` |
| `write_register_code` | Hex | e.g., `0x06` |
| `error_codes` | String | `code=label;code=label;...` |
| `file` | File | The CSV file containing registers |

#### CSV Format

The CSV must include headers:
`RegisterName`, `Label`, `Category`, `RegisterType`, `AddressStart`, `Length`, `AccessType`, `DataType`, `SubDataType`, `GroupName`, `GroupSeqNo`.

Optional fields: `Scale`, `Unit`, `WordOrder`, `ByteOrder`, `EnumMap`, `BitMap`, `Constraints`, `Execution`.

## Testing

Run the automated validation test suite:
```bash
go test ./parser/...
```
This suite validates the parser against 37+ edge-case CSVs located in `validation_tests/`.
