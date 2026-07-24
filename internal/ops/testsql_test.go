package ops

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// testsql.go keeps its own copy of tasks.go's taskSelect (the API's query lives in an
// unexported const in another package). A copy that drifts is worse than no check at all: the
// self-test would report "OK" for a query the app no longer runs. This compares the two
// verbatim, ignoring whitespace, and fails loudly the moment they diverge.
func TestTaskSelectCopyMatchesAPI(t *testing.T) {
	src, err := os.ReadFile("../api/tasks.go")
	if err != nil {
		t.Fatalf("reading tasks.go: %v", err)
	}
	re := regexp.MustCompile("(?s)const taskSelect = `(.*?)`")
	m := re.FindSubmatch(src)
	if m == nil {
		t.Fatal("could not find taskSelect in internal/api/tasks.go — was it renamed?")
	}
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	want, got := norm(string(m[1])), norm(taskSelectSQL)
	if want != got {
		t.Errorf("taskSelectSQL in testsql.go has drifted from taskSelect in api/tasks.go.\n"+
			"api:     %s\ntestsql: %s", want, got)
	}
}
