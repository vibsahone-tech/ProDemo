package parser

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestParseProtocolForm_Validations(t *testing.T) {
	tests := []struct {
		name       string
		formData   url.Values
		wantErrs   []string
		checkCodes bool
	}{
		{
			name: "Valid baud rate and function codes",
			formData: url.Values{
				"name":                {"Test Protocol"},
				"slave_address":       {"1"},
				"baud_rate":           {"9600"},
				"stop_bits":           {"1"},
				"data_bits":           {"8"},
				"parity":              {"1"},
				"read_register_code":  {"0x03"},
				"write_register_code": {"0x06"},
			},
			wantErrs: nil,
		},
		{
			name: "Invalid baud rate",
			formData: url.Values{
				"name":                {"Test Protocol"},
				"slave_address":       {"1"},
				"baud_rate":           {"9999"},
				"stop_bits":           {"1"},
				"data_bits":           {"8"},
				"parity":              {"1"},
				"read_register_code":  {"0x03"},
				"write_register_code": {"0x06"},
			},
			wantErrs: []string{"baud_rate: invalid value 9999"},
		},
		{
			name: "Valid multi-select read code",
			formData: url.Values{
				"name":                {"Test Protocol"},
				"slave_address":       {"1"},
				"baud_rate":           {"9600"},
				"stop_bits":           {"1"},
				"data_bits":           {"8"},
				"parity":              {"1"},
				"read_register_code":  {"0x03, 0x04"},
				"write_register_code": {"0x06"},
			},
			wantErrs: nil,
		},
		{
			name: "Invalid read code",
			formData: url.Values{
				"name":                {"Test Protocol"},
				"slave_address":       {"1"},
				"baud_rate":           {"9600"},
				"stop_bits":           {"1"},
				"data_bits":           {"8"},
				"parity":              {"1"},
				"read_register_code":  {"0x99"},
				"write_register_code": {"0x06"},
			},
			wantErrs: []string{"read_register_code: invalid value \"0x99\""},
		},
		// ── Slave Address tests ───────────────────
		{
			name: "Slave address = 0 (out of range)",
			formData: url.Values{
				"name":                {"Test Protocol"},
				"slave_address":       {"0"},
				"baud_rate":           {"9600"},
				"stop_bits":           {"1"},
				"data_bits":           {"8"},
				"parity":              {"1"},
				"read_register_code":  {"0x03"},
				"write_register_code": {"0x06"},
			},
			wantErrs: []string{"slave_address: must be between 1 and 247"},
		},
		{
			name: "Slave address = -5 (negative, out of range)",
			formData: url.Values{
				"name":                {"Test Protocol"},
				"slave_address":       {"-5"},
				"baud_rate":           {"9600"},
				"stop_bits":           {"1"},
				"data_bits":           {"8"},
				"parity":              {"1"},
				"read_register_code":  {"0x03"},
				"write_register_code": {"0x06"},
			},
			wantErrs: []string{"slave_address: must be between 1 and 247"},
		},
		{
			name: "Slave address = 248 (above range)",
			formData: url.Values{
				"name":                {"Test Protocol"},
				"slave_address":       {"248"},
				"baud_rate":           {"9600"},
				"stop_bits":           {"1"},
				"data_bits":           {"8"},
				"parity":              {"1"},
				"read_register_code":  {"0x03"},
				"write_register_code": {"0x06"},
			},
			wantErrs: []string{"slave_address: must be between 1 and 247"},
		},
		{
			name: "Slave address = 247 (upper boundary, valid)",
			formData: url.Values{
				"name":                {"Test Protocol"},
				"slave_address":       {"247"},
				"baud_rate":           {"9600"},
				"stop_bits":           {"1"},
				"data_bits":           {"8"},
				"parity":              {"1"},
				"read_register_code":  {"0x03"},
				"write_register_code": {"0x06"},
			},
			wantErrs: nil,
		},
		// ── Stop Bits tests ───────────────────
		{
			name: "Valid stop_bits = 1.5",
			formData: url.Values{
				"name":                {"Test Protocol"},
				"slave_address":       {"1"},
				"baud_rate":           {"9600"},
				"stop_bits":           {"1.5"},
				"data_bits":           {"8"},
				"parity":              {"1"},
				"read_register_code":  {"0x03"},
				"write_register_code": {"0x06"},
			},
			wantErrs: nil,
		},
		{
			name: "Invalid stop_bits = 3",
			formData: url.Values{
				"name":                {"Test Protocol"},
				"slave_address":       {"1"},
				"baud_rate":           {"9600"},
				"stop_bits":           {"3"},
				"data_bits":           {"8"},
				"parity":              {"1"},
				"read_register_code":  {"0x03"},
				"write_register_code": {"0x06"},
			},
			wantErrs: []string{"stop_bits: invalid value 3"},
		},
		// ── Data Bits tests ───────────────────
		{
			name: "Valid data_bits = 5",
			formData: url.Values{
				"name":                {"Test Protocol"},
				"slave_address":       {"1"},
				"baud_rate":           {"9600"},
				"stop_bits":           {"1"},
				"data_bits":           {"5"},
				"parity":              {"1"},
				"read_register_code":  {"0x03"},
				"write_register_code": {"0x06"},
			},
			wantErrs: nil,
		},
		{
			name: "Invalid data_bits = 9",
			formData: url.Values{
				"name":                {"Test Protocol"},
				"slave_address":       {"1"},
				"baud_rate":           {"9600"},
				"stop_bits":           {"1"},
				"data_bits":           {"9"},
				"parity":              {"1"},
				"read_register_code":  {"0x03"},
				"write_register_code": {"0x06"},
			},
			wantErrs: []string{"data_bits: invalid value 9"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/test", strings.NewReader(tt.formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			_ = req.ParseForm()

			_, errs := ParseProtocolForm(req)

			if len(errs) != len(tt.wantErrs) {
				t.Errorf("expected %d errors, got %d: %v", len(tt.wantErrs), len(errs), errs)
				return
			}

			for i, err := range errs {
				if !strings.Contains(err, tt.wantErrs[i]) {
					t.Errorf("expected error to contain %q, got %q", tt.wantErrs[i], err)
				}
			}
		})
	}
}
