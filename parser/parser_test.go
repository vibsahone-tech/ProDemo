package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCSVValidationSuite(t *testing.T) {
	// Path to the validation tests directory relative to the parser package
	testDir := "../validation_tests"

	files, err := os.ReadDir(testDir)
	if err != nil {
		t.Fatalf("failed to read validation_tests directory: %v", err)
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".csv") {
			continue
		}

		t.Run(file.Name(), func(t *testing.T) {
			filePath := filepath.Join(testDir, file.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("failed to read test file %s: %v", file.Name(), err)
			}

			// All tests in this suite use the default MaxRows limit from config (2000)
			// or a reasonable default for testing like 100.
			_, _, parseErrs, err := ParseCSV(data, 100)
			if err != nil {
				// Fatal errors (like CSV format errors) are only expected if specifically testing that.
				// For most validation tests, we expect the parser to continue and return ParseError.
				t.Errorf("%s: ParseCSV returned fatal error: %v", file.Name(), err)
				return
			}

			isFailTest := strings.Contains(file.Name(), "_fail_")
			isValidTest := strings.Contains(file.Name(), "_valid_")

			if isFailTest && len(parseErrs) == 0 {
				t.Errorf("%s: expected validation errors but got none", file.Name())
			}

			if isValidTest && len(parseErrs) > 0 {
				t.Errorf("%s: expected valid CSV but got errors: %v", file.Name(), parseErrs)
			}
		})
	}
}
