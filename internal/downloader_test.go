package internal

import (
	"fmt"
	"strings"
	"testing"
)

func TestStringsCut(t *testing.T) {
	var s = "aaa.bbb.ccc.ddd.eee"
	fir, after, exists := strings.Cut(s, ".")
	fmt.Println(fir, after, exists)
	fmt.Println(strings.SplitAfterN(s, ".", 1))
	fmt.Println(strings.SplitN(s, ".", 2))
	fmt.Println(strings.SplitAfterN(s, ".", 2))
	fmt.Println(strings.SplitAfterN(s, ".", 3))

}
