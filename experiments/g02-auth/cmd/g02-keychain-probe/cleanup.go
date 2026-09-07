//go:build darwin && cgo && g02runtime

package main

// finishOwned centralizes the cleanup ordering so the private recovery inventory
// cannot be removed while an exact launchd label's absence remains unproven.
func finishOwned(servicesGone bool, lockKeychain func(), deleteKeychain, removeDirectory func() bool) bool {
	if !servicesGone {
		lockKeychain()
		return false
	}
	if !deleteKeychain() {
		lockKeychain()
		return false
	}
	return removeDirectory()
}
