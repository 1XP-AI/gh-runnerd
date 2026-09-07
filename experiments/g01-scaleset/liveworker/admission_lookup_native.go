//go:build cgo && !osusergo && !android

package liveworker

// Pin the supported os/user implementation. Alternate pure-Go lookups may
// synthesize the current account from HOME/USER when the account is absent.
const nativeAccountLookup = true
