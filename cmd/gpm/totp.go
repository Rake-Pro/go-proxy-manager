package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
)

// runTOTPSecret prints a fresh base32 TOTP secret and its otpauth:// enrolment
// URI, for GPM_LOCAL_ADMIN_TOTP_SECRET (or its _FILE form). The secret is
// printed to stdout so it can be redirected into a secret file; everything else
// goes to stderr.
//
// No QR code is rendered: the URI is the enrolment payload, and any QR encoder
// turns it into the square an authenticator app scans.
func runTOTPSecret(args []string) {
	fs := flag.NewFlagSet("totp-secret", flag.ExitOnError)
	account := fs.String("account", envOr("GPM_LOCAL_ADMIN_USER", "admin"),
		"account name shown in the authenticator app (the local admin username)")
	issuer := fs.String("issuer", "gpm",
		"issuer shown in the authenticator app (use settings.appName to match the UI)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	secret, err := auth.NewTOTPSecret()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println(secret)
	fmt.Fprintln(os.Stderr, "otpauth URI (paste into any QR encoder, or enter the secret by hand):")
	fmt.Fprintln(os.Stderr, auth.TOTPURI(*issuer, *account, secret))
	fmt.Fprintln(os.Stderr, "Set it as GPM_LOCAL_ADMIN_TOTP_SECRET (or GPM_LOCAL_ADMIN_TOTP_SECRET_FILE) and restart gpm.")
}
