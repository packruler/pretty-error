package htmltemplates_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/packruler/pretty-error/htmltemplates"
)

func TestEncode(t *testing.T) {
	status := 400
	for status < 404 {
		t.Run(fmt.Sprintf("Status: %d", status), func(t *testing.T) {
			output, err := htmltemplates.GetErrorBody(int16(status))
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			isBad := !strings.Contains(string(output), fmt.Sprint(status))

			if isBad {
				t.Errorf("expected status: %d got: %s", status, output)
			}
		})
		status++
	}
}
