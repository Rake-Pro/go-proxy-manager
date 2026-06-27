package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// runHashpw prints a bcrypt hash for a password, for use as the local/break-glass
// admin password hash (GPM_LOCAL_ADMIN_PASSWORD_HASH) or an access-list basic-auth
// user. Reads the password from the first argument, or from stdin if omitted.
func runHashpw(args []string) {
	var pw string
	if len(args) > 0 && args[0] != "" {
		pw = args[0]
	} else {
		fmt.Fprint(os.Stderr, "password: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		pw = strings.TrimRight(line, "\r\n")
	}
	if pw == "" {
		fmt.Fprintln(os.Stderr, "error: empty password")
		os.Exit(2)
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(h))
}
