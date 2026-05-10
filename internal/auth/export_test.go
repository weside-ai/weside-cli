package auth

// SetCacheWriterForTests swaps the auth-cache writer with a stub so tests don't
// touch the real ~/.weside/config.yaml. Call ResetCacheWriterForTests in t.Cleanup.
func SetCacheWriterForTests(fn func() error) {
	writeConfig = fn
}

// ResetCacheWriterForTests restores the production writeConfig implementation.
func ResetCacheWriterForTests() {
	writeConfig = defaultWriteConfig
}

// defaultWriteConfig captures the production writer at package init time so
// ResetCacheWriterForTests can restore it after a test swap.
var defaultWriteConfig = writeConfig
