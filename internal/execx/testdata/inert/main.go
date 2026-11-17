// Command inert is built by a test of internal/execx to prove the shell guard does nothing in a production
// binary: it prints whether a login shell start would pass the guard and whether the login-shell PATH is read.
package main

import (
	"fmt"
	"strings"

	"github.com/rudian/codepass/internal/execx"
	"github.com/rudian/codepass/internal/shellguard"
)

func main() {
	ok := shellguard.Check("/bin/zsh", []string{"-l", "-i"}, []string{"HOME=/Users/someone"}) == nil
	execx.Configure(execx.Options{})
	fmt.Println(ok, strings.Contains(execx.EnvPATH(), "/fake/shell/bin"))
}
