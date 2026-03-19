package parser

import (
	"csv-upload-parser/model"
	"fmt"
	"net/http"
	"regexp" // Added regexp import
	"strconv"
	"strings"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ProtocolParseError holds validation errors for the protocol form-data.
type ProtocolParseError struct {
	Errors []string `json:"errors"`
}

// ParseProtocolForm extracts and validates protocol fields from multipart form-data.
// Returns the constructed Protocol and any validation errors.
func ParseProtocolForm(r *http.Request) (model.Protocol, []string) {
	var errs []string

	// 1. Name -trim, not empty ,length check
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		errs = append(errs, "name: mandatory field is missing")
	} else if utf8.RuneCountInString(name) > maxStrLen {
		errs = append(errs, fmt.Sprintf("name: exceeds %d characters", maxStrLen))
	}

	// 2. Slave Address - trim, mandatory, integer, range check (1-247)
	var slaveAddress int
	rawSlave := strings.TrimSpace(r.FormValue("slave_address"))
	if rawSlave == "" {
		errs = append(errs, "slave_address: mandatory field is missing")
	} else if v, err := strconv.Atoi(rawSlave); err != nil {
		errs = append(errs, "slave_address: must be an integer")
	} else if v < 1 || v > 247 {
		errs = append(errs, "slave_address: must be between 1 and 247")
	} else {
		slaveAddress = v
	}

	// 3. Baud Rate - trim, mandatory, integer, in-list check
	var baudRate int
	rawBaud := strings.TrimSpace(r.FormValue("baud_rate"))
	if rawBaud == "" {
		errs = append(errs, "baud_rate: mandatory field is missing")
	} else if v, err := strconv.Atoi(rawBaud); err != nil {
		errs = append(errs, "baud_rate: must be an integer")
	} else {
		allowed := false
		for _, b := range []int{1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200} {
			if v == b {
				allowed = true
				break
			}
		}
		if !allowed {
			errs = append(errs, fmt.Sprintf("baud_rate: invalid value %d", v))
		} else {
			baudRate = v
		}
	}

	// 4. Stop Bits - trim, mandatory, float, in-list check
	var stopBits float64
	rawStop := strings.TrimSpace(r.FormValue("stop_bits"))
	if rawStop == "" {
		errs = append(errs, "stop_bits: mandatory field is missing")
	} else if v, err := strconv.ParseFloat(rawStop, 64); err != nil {
		errs = append(errs, "stop_bits: must be a number")
	} else if v != 1.0 && v != 1.5 && v != 2.0 {
		errs = append(errs, fmt.Sprintf("stop_bits: invalid value %v", v))
	} else {
		stopBits = v
	}

	// 5. Data Bits - trim, mandatory, integer, range check (5-8)
	var dataBits int
	rawData := strings.TrimSpace(r.FormValue("data_bits"))
	if rawData == "" {
		errs = append(errs, "data_bits: mandatory field is missing")
	} else if v, err := strconv.Atoi(rawData); err != nil {
		errs = append(errs, "data_bits: must be an integer")
	} else if v < 5 || v > 8 {
		errs = append(errs, fmt.Sprintf("data_bits: invalid value %d", v))
	} else {
		dataBits = v
	}

	// 6. Parity - trim, mandatory, integer, enum range check
	var parity int
	rawParity := strings.TrimSpace(r.FormValue("parity"))
	if rawParity == "" {
		errs = append(errs, "parity: mandatory field is missing")
	} else if v, err := strconv.Atoi(rawParity); err != nil {
		errs = append(errs, "parity: must be an integer")
	} else if v <= 0 || v >= int(model.ParityMax) {
		errs = append(errs, fmt.Sprintf("parity: invalid value %d (valid: 1–%d)", v, int(model.ParityMax)-1))
	} else {
		parity = v
	}

	// 7. Register Codes - trim, mandatory, in-list check for each code
	readRegCode := strings.TrimSpace(r.FormValue("read_register_code"))
	if readRegCode == "" {
		errs = append(errs, "read_register_code: mandatory field is missing")
	} else {
		allowed := false
		lowerRead := strings.ToLower(readRegCode)
		for _, c := range []string{"0x02", "0x03", "0x04", "0x05", "0x03, 0x04"} {
			if lowerRead == c {
				allowed = true
				break
			}
		}
		if !allowed {
			errs = append(errs, "read_register_code: invalid value "+readRegCode)
		}
	}

	writeRegCode := strings.TrimSpace(r.FormValue("write_register_code"))
	if writeRegCode == "" {
		errs = append(errs, "write_register_code: mandatory field is missing")
	} else if strings.ToLower(writeRegCode) != "0x06" {
		errs = append(errs, "write_register_code: invalid value "+writeRegCode)
	}

	writeMultiRegCode := strings.TrimSpace(r.FormValue("write_multiple_register_code"))
	if writeMultiRegCode == "" {
		errs = append(errs, "write_multiple_register_code: mandatory field is missing")
	} else if strings.ToLower(writeMultiRegCode) != "0x10" {
		errs = append(errs, "write_multiple_register_code: invalid value "+writeMultiRegCode)
	}

	commErrCode := strings.TrimSpace(r.FormValue("comm_err_response_code"))
	if commErrCode == "" {
		errs = append(errs, "comm_err_response_code: mandatory field is missing")
	} else if strings.ToLower(commErrCode) != "0x83" {
		errs = append(errs, "comm_err_response_code: invalid value "+commErrCode)
	}

	// 8. Error Codes - regex-based parsing of code=label pairs
	errorCodes, errCodeErrs := parseErrorCodes(r.FormValue("error_codes"))
	errs = append(errs, errCodeErrs...)

	proto := model.Protocol{
		ID:           bson.NewObjectID(),
		Name:         name,
		SlaveAddress: slaveAddress,
		Communication: model.Communication{
			BaudRate: baudRate,
			StopBits: stopBits,
			DataBits: dataBits,
			Parity:   model.Parity(parity),
		},
		ReadRegisterCode:          readRegCode,
		WriteRegisterCode:         writeRegCode,
		WriteMultipleRegisterCode: writeMultiRegCode,
		CommErrResponseCode:       commErrCode,
		ErrorCodes:                errorCodes,
	}

	return proto, errs
}

// parseErrorCodes parses "code=label;code=label;..." into []ErrorCode.
func parseErrorCodes(raw string) ([]model.ErrorCode, []string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	// The $ ensures we check the ENTIRE item, catching missing separators.
	re := regexp.MustCompile(`^(\d+)=([^;=]+)$`)

	var result []model.ErrorCode
	var errs []string
	seen := make(map[int]bool)

	for _, item := range strings.Split(raw, ";") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		matches := re.FindStringSubmatch(item)
		if matches == nil {
			errs = append(errs, "error_codes: invalid item "+item)
			continue
		}

		code, _ := strconv.Atoi(matches[1])
		label := strings.TrimSpace(matches[2])

		if seen[code] {
			errs = append(errs, fmt.Sprintf("error_codes: duplicate code %d", code))
		} else if label == "" {
			errs = append(errs, fmt.Sprintf("error_codes: empty label for %d", code))
		} else {
			seen[code] = true
			result = append(result, model.ErrorCode{Code: code, Label: label})
		}
	}
	return result, errs
}
