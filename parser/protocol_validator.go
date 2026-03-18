package parser

import (
	"csv-upload-parser/model"
	"fmt"
	"net/http"
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

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		errs = append(errs, "name: mandatory field is missing")
	} else if utf8.RuneCountInString(name) > maxStrLen {
		errs = append(errs, fmt.Sprintf("name: exceeds %d characters", maxStrLen))
	}

	slaveAddress := formInt(r, "slave_address", true, &errs)
	if (slaveAddress < 1 || slaveAddress > 247) && !hasError(errs, "slave_address") {
		errs = append(errs, "slave_address: must be between 1 and 247")
	}

	baudRate := formInt(r, "baud_rate", true, &errs)
	// Validate baudRate against allowed list from UI.
	validateInList("baud_rate", baudRate, []int{1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200}, &errs)

	stopBits := formFloat(r, "stop_bits", true, &errs)
	validateInListFloat("stop_bits", stopBits, []float64{1, 1.5, 2}, &errs)

	dataBits := formInt(r, "data_bits", true, &errs)
	validateInList("data_bits", dataBits, []int{5, 6, 7, 8}, &errs)

	parity := formInt(r, "parity", true, &errs)
	readRegCode := normalizeHexCode(r.FormValue("read_register_code"))
	if readRegCode == "" {
		errs = append(errs, "read_register_code: mandatory field is missing")
	} else {
		validateInListStr("read_register_code", readRegCode, []string{"0x02", "0x03", "0x04", "0x05", "0x03, 0x04"}, &errs)
	}

	writeRegCode := normalizeHexCode(r.FormValue("write_register_code"))
	if writeRegCode == "" {
		errs = append(errs, "write_register_code: mandatory field is missing")
	} else {
		validateInListStr("write_register_code", writeRegCode, []string{"0x06", "0x10"}, &errs)
	}

	writeMultiRegCode := strings.TrimSpace(r.FormValue("write_multiple_register_code"))
	commErrCode := strings.TrimSpace(r.FormValue("comm_err_response_code"))

	// Validate parity enum.
	validateEnum("parity", parity, int(model.ParityMax), true, &errs)

	// Parse error_codes: key-value pairs sent as "code=label" separated by ";".
	// e.g. "1=Illegal Function;2=Illegal Data Address;3=Illegal Data Value"
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
// It also returns a list of validation errors for any malformed items,
// telling the caller what went wrong and showing the expected format.
func parseErrorCodes(raw string) ([]model.ErrorCode, []string) {
	const correctFormat = `correct format: "<int>=<label>;<int>=<label>;..." e.g. "1=Illegal Function;2=Illegal Data Address;3=Illegal Data Value"`
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	items := strings.Split(raw, ";")
	result := make([]model.ErrorCode, 0, len(items))
	var errs []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		codeStr, label, ok := strings.Cut(item, "=")
		if !ok {
			errs = append(errs, fmt.Sprintf(
				`error_codes: item %q is missing "=" separator; %s`,
				item, correctFormat,
			))
			continue
		}
		codeStr = strings.TrimSpace(codeStr)
		label = strings.TrimSpace(label)
		code, err := strconv.Atoi(codeStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf(
				`error_codes: code %q in item %q must be an integer; %s`,
				codeStr, item, correctFormat,
			))
			continue
		}
		if label == "" {
			errs = append(errs, fmt.Sprintf(
				`error_codes: label is empty for code %d; %s`,
				code, correctFormat,
			))
			continue
		}
		result = append(result, model.ErrorCode{
			Code:  code,
			Label: label,
		})
	}
	return result, errs
}

// formInt reads an integer from a form field. If mandatory and missing/invalid, appends an error.
func formInt(r *http.Request, field string, mandatory bool, errs *[]string) int {
	raw := strings.TrimSpace(r.FormValue(field))
	if raw == "" {
		if mandatory {
			*errs = append(*errs, fmt.Sprintf("%s: mandatory field is missing", field))
		}
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: must be an integer", field))
		return 0
	}
	return v
}

// formFloat reads a float from a form field. If mandatory and missing/invalid, appends an error.
func formFloat(r *http.Request, field string, mandatory bool, errs *[]string) float64 {
	raw := strings.TrimSpace(r.FormValue(field))
	if raw == "" {
		if mandatory {
			*errs = append(*errs, fmt.Sprintf("%s: mandatory field is missing", field))
		}
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: must be a number", field))
		return 0
	}
	return v
}

// hasError returns true if field has an error already.
func hasError(errs []string, field string) bool {
	prefix := field + ":"
	for _, e := range errs {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}

// validateInList checks if an integer value is within a permitted set.
func validateInList(field string, val int, allowed []int, errs *[]string) {
	if hasError(*errs, field) {
		return
	}
	for _, a := range allowed {
		if val == a {
			return
		}
	}
	*errs = append(*errs, fmt.Sprintf("%s: invalid value %d", field, val))
}

// validateInListFloat checks if a float64 value is within a permitted set.
func validateInListFloat(field string, val float64, allowed []float64, errs *[]string) {
	if hasError(*errs, field) {
		return
	}
	for _, a := range allowed {
		if val == a {
			return
		}
	}
	*errs = append(*errs, fmt.Sprintf("%s: invalid value %g", field, val))
}

// validateInListStr checks if a string value is within a permitted set.
func validateInListStr(field string, val string, allowed []string, errs *[]string) {
	if hasError(*errs, field) {
		return
	}
	for _, a := range allowed {
		if val == a {
			return
		}
	}
	*errs = append(*errs, fmt.Sprintf("%s: invalid value %q", field, val))
}

// normalizeHexCode pads decimal strings with 0x and a zero-prefix if necessary.
// e.g., "3" -> "0x03", "03" -> "0x03", "0x03" -> "0x03".
func normalizeHexCode(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "0x") {
		return s
	}
	// Try parsing as integer (decimal or hex WITHOUT 0x prefix)
	if v, err := strconv.Atoi(s); err == nil {
		return fmt.Sprintf("0x%02x", v)
	}
	return s
}
