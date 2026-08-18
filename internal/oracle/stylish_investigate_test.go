package oracle

import (
	"fmt"
	"testing"
)

func TestStylishInvestigate(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	r, err := Run(
		"/tmp/eslint-inspect/lib/cli-engine/formatters/stylish.js",
		"/tmp/eslint-inspect/tests/lib/cli-engine/formatters/stylish.js",
		"/tmp/sty-oracle-wd",
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	fmt.Println("STYLISH JS >>>", r.JSOut)
	fmt.Println("STYLISH BUILD >>>", r.GoBuildOK, r.GoBuildEr)
	fmt.Println("STYLISH GO RUN >>>", r.GoRunOut)
	fmt.Println("STYLISH SUMMARY >>>", r.Summary())
}
