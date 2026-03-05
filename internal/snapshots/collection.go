package snapshots

const internalCollectionTmpDir = ".tmp"

// IsInternalCollectionDir reports whether a collection directory name is
// reserved for internal snapshot/orphan staging.
func IsInternalCollectionDir(name string) bool {
	return name == internalCollectionTmpDir
}
